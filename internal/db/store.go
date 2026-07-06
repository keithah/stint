package db

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql/driver"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/keithah/stint/internal/auth"
	"github.com/keithah/stint/internal/services"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var ErrDuplicateHeartbeat = errors.New("duplicate heartbeat")
var ErrInvalidOAuthScope = errors.New("invalid OAuth scope")
var ErrInvalidResourceName = errors.New("invalid resource name")
var ErrInvalidDataDumpType = errors.New("invalid data dump type")
var ErrCustomRulesAborted = errors.New("custom rules application aborted")
var ErrDuplicatePublicUsername = errors.New("public username already exists")

type Store struct {
	Pool *pgxpool.Pool
}

const customRuleBatchWriteLimit = 1000
const wakaTimeImportChunkSize = 5000

const userColumns = `id, github_id, github_username, coalesce(email, ''), coalesce(full_name, ''),
	coalesce(avatar_url, ''), timezone, timeout_minutes, writes_only, is_hireable, has_public_profile, coalesce(country, ''), heartbeat_retention_days,
	coalesce(public_username, ''), coalesce(public_display_name, ''), public_github_link_enabled, public_show_total_time, public_show_projects,
	public_project_visibility, public_show_languages, public_show_editors, public_show_machines, public_show_operating_systems,
	public_show_categories, public_show_ai, public_show_summaries, coalesce(public_profile, '{}'::jsonb)`

const userColumnsU = `u.id, u.github_id, u.github_username, coalesce(u.email, ''), coalesce(u.full_name, ''),
	coalesce(u.avatar_url, ''), u.timezone, u.timeout_minutes, u.writes_only, u.is_hireable, u.has_public_profile, coalesce(u.country, ''), u.heartbeat_retention_days,
	coalesce(u.public_username, ''), coalesce(u.public_display_name, ''), u.public_github_link_enabled, u.public_show_total_time, u.public_show_projects,
	u.public_project_visibility, u.public_show_languages, u.public_show_editors, u.public_show_machines, u.public_show_operating_systems,
	u.public_show_categories, u.public_show_ai, u.public_show_summaries, coalesce(u.public_profile, '{}'::jsonb)`

type User struct {
	ID                      uuid.UUID     `json:"id"`
	GitHubID                int64         `json:"github_id"`
	GitHubUsername          string        `json:"github_username"`
	Email                   string        `json:"email,omitempty"`
	FullName                string        `json:"full_name,omitempty"`
	AvatarURL               string        `json:"avatar_url,omitempty"`
	Country                 string        `json:"country,omitempty"`
	Timezone                string        `json:"timezone"`
	TimeoutMinutes          int           `json:"timeout_minutes"`
	WritesOnly              bool          `json:"writes_only"`
	IsHireable              bool          `json:"is_hireable"`
	HasPublicProfile        bool          `json:"has_public_profile"`
	HeartbeatRetentionDays  int           `json:"heartbeat_retention_days"`
	PublicUsername          string        `json:"public_username,omitempty"`
	PublicDisplayName       string        `json:"public_display_name,omitempty"`
	PublicGitHubLink        bool          `json:"public_github_link_enabled"`
	PublicShowTotalTime     bool          `json:"public_show_total_time"`
	PublicShowProjects      bool          `json:"public_show_projects"`
	PublicProjectVisibility string        `json:"public_project_visibility"`
	PublicShowLanguages     bool          `json:"public_show_languages"`
	PublicShowEditors       bool          `json:"public_show_editors"`
	PublicShowMachines      bool          `json:"public_show_machines"`
	PublicShowOS            bool          `json:"public_show_operating_systems"`
	PublicShowCategories    bool          `json:"public_show_categories"`
	PublicShowAI            bool          `json:"public_show_ai"`
	PublicShowSummaries     bool          `json:"public_show_summaries"`
	PublicProfile           PublicProfile `json:"public_profile"`
}

// PublicProfile holds optional personal-info fields and the owner-selected
// public-page layout. It is persisted as a single jsonb column so new fields
// and an evolving per-field visibility model extend without a migration.
// Visibility maps a field key to "public" or "private"; an absent key means
// public. The value set is intentionally open so org/team scopes can be added
// later without breaking stored data.
type PublicProfile struct {
	Bio              string            `json:"bio,omitempty"`
	Location         string            `json:"location,omitempty"`
	WebsiteURL       string            `json:"website_url,omitempty"`
	TwitterUsername  string            `json:"twitter_username,omitempty"`
	LinkedInURL      string            `json:"linkedin_url,omitempty"`
	MastodonURL      string            `json:"mastodon_url,omitempty"`
	Pronouns         string            `json:"pronouns,omitempty"`
	Company          string            `json:"company,omitempty"`
	Role             string            `json:"role,omitempty"`
	Layout           string            `json:"layout,omitempty"`
	DefaultRange     string            `json:"default_range,omitempty"`
	AvailableForHire bool              `json:"available_for_hire,omitempty"`
	EmailPublic      bool              `json:"email_public,omitempty"`
	Visibility       map[string]string `json:"visibility,omitempty"`
}

func (p PublicProfile) Value() (driver.Value, error) {
	return json.Marshal(p)
}

func (p *PublicProfile) Scan(src any) error {
	*p = PublicProfile{}
	if src == nil {
		return nil
	}
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("unsupported public_profile source type %T", src)
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, p)
}

type UserSettingsInput struct {
	Timezone                string
	TimeoutMinutes          int
	WritesOnly              bool
	HasPublicProfile        bool
	Country                 string
	HeartbeatRetentionDays  int
	PublicUsername          string
	PublicDisplayName       string
	PublicGitHubLink        bool
	PublicShowTotalTime     bool
	PublicShowProjects      bool
	PublicProjectVisibility string
	PublicShowLanguages     bool
	PublicShowEditors       bool
	PublicShowMachines      bool
	PublicShowOS            bool
	PublicShowCategories    bool
	PublicShowAI            bool
	PublicShowSummaries     bool
	PublicProfile           PublicProfile
}

type APIKey struct {
	ID          uuid.UUID  `json:"id"`
	UserID      uuid.UUID  `json:"user_id"`
	Name        string     `json:"name"`
	Fingerprint string     `json:"fingerprint"`
	Scopes      []string   `json:"scopes"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func DefaultAPIKeyScopes() []string {
	return []string{
		"read_stats",
		"read_summaries",
		"read_heartbeats",
		"write_heartbeats",
		"read_goals",
		"read_private_leaderboards",
		"write_private_leaderboards",
		"email",
	}
}

type Project struct {
	ID               uuid.UUID  `json:"id"`
	Name             string     `json:"name"`
	Color            string     `json:"color,omitempty"`
	HasPublicURL     bool       `json:"has_public_url"`
	Badge            string     `json:"badge,omitempty"`
	FirstHeartbeatAt *time.Time `json:"first_heartbeat_at,omitempty"`
	LastHeartbeatAt  *time.Time `json:"last_heartbeat_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type MachineName struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Value      string     `json:"value,omitempty"`
	Timezone   string     `json:"timezone,omitempty"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type Goal struct {
	ID               uuid.UUID  `json:"id"`
	Title            string     `json:"title"`
	CustomTitle      string     `json:"custom_title,omitempty"`
	Delta            string     `json:"delta"`
	Seconds          int        `json:"seconds"`
	Languages        []string   `json:"languages"`
	Editors          []string   `json:"editors"`
	Projects         []string   `json:"projects"`
	IgnoreDays       []string   `json:"ignore_days"`
	IgnoreZeroDays   bool       `json:"ignore_zero_days"`
	ImproveByPercent *float64   `json:"improve_by_percent,omitempty"`
	IsEnabled        bool       `json:"is_enabled"`
	IsInverse        bool       `json:"is_inverse"`
	IsSnoozed        bool       `json:"is_snoozed"`
	SnoozeUntil      *time.Time `json:"snooze_until,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	ModifiedAt       time.Time  `json:"modified_at"`
}

type GoalInput struct {
	Title            string     `json:"title"`
	CustomTitle      string     `json:"custom_title"`
	Delta            string     `json:"delta"`
	Seconds          int        `json:"seconds"`
	Languages        []string   `json:"languages"`
	Editors          []string   `json:"editors"`
	Projects         []string   `json:"projects"`
	IgnoreDays       []string   `json:"ignore_days"`
	IgnoreZeroDays   bool       `json:"ignore_zero_days"`
	ImproveByPercent *float64   `json:"improve_by_percent"`
	IsEnabled        *bool      `json:"is_enabled"`
	IsInverse        bool       `json:"is_inverse"`
	IsSnoozed        bool       `json:"is_snoozed"`
	SnoozeUntil      *time.Time `json:"snooze_until"`
}

type GoalEvaluation struct {
	ID            uuid.UUID `json:"id"`
	UserID        uuid.UUID `json:"user_id"`
	GoalID        uuid.UUID `json:"goal_id"`
	PeriodStart   time.Time `json:"period_start"`
	PeriodEnd     time.Time `json:"period_end"`
	ActualSeconds int       `json:"actual_seconds"`
	TargetSeconds int       `json:"target_seconds"`
	Percent       int       `json:"percent"`
	IsComplete    bool      `json:"is_complete"`
	EvaluatedAt   time.Time `json:"evaluated_at"`
}

type UserAgent struct {
	ID                 string    `json:"id"`
	Value              string    `json:"value"`
	Editor             string    `json:"editor"`
	AIModel            string    `json:"ai_model,omitempty"`
	AIProvider         string    `json:"ai_provider,omitempty"`
	AIAgent            string    `json:"ai_agent,omitempty"`
	AIAgentVersion     string    `json:"ai_agent_version,omitempty"`
	AIAgentComplexity  string    `json:"ai_agent_complexity,omitempty"`
	Version            string    `json:"version,omitempty"`
	OS                 string    `json:"os,omitempty"`
	LastSeenAt         time.Time `json:"last_seen_at"`
	IsBrowserExtension bool      `json:"is_browser_extension"`
	IsDesktopApp       bool      `json:"is_desktop_app"`
	CreatedAt          time.Time `json:"created_at"`
}

type ExternalDuration struct {
	ID         uuid.UUID `json:"id"`
	ExternalID string    `json:"external_id"`
	Provider   string    `json:"provider"`
	Entity     string    `json:"entity"`
	Type       string    `json:"type"`
	Category   string    `json:"category,omitempty"`
	StartTime  float64   `json:"start_time"`
	EndTime    float64   `json:"end_time"`
	Project    string    `json:"project,omitempty"`
	Branch     string    `json:"branch,omitempty"`
	Language   string    `json:"language,omitempty"`
	Meta       string    `json:"meta,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type Leaderboard struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	TimeRange  string    `json:"time_range"`
	CreatedAt  time.Time `json:"created_at"`
	ModifiedAt time.Time `json:"modified_at"`
}

type LeaderboardMember struct {
	UserID   uuid.UUID `json:"user_id"`
	Username string    `json:"username"`
	FullName string    `json:"full_name,omitempty"`
	Role     string    `json:"role"`
}

type DataDump struct {
	ID              uuid.UUID  `json:"id"`
	Type            string     `json:"type"`
	Status          string     `json:"status"`
	PercentComplete float64    `json:"percent_complete"`
	DownloadURL     string     `json:"download_url,omitempty"`
	IsProcessing    bool       `json:"is_processing"`
	IsStuck         bool       `json:"is_stuck"`
	HasFailed       bool       `json:"has_failed"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type CustomRule struct {
	ID           uuid.UUID                        `json:"id"`
	Action       string                           `json:"action"`
	Source       string                           `json:"source"`
	Operation    string                           `json:"operation"`
	SourceValue  string                           `json:"source_value"`
	Priority     int                              `json:"priority"`
	Destinations []services.CustomRuleDestination `json:"destinations"`
	CreatedAt    time.Time                        `json:"created_at"`
	ModifiedAt   time.Time                        `json:"modified_at"`
}

type CustomRulesProgress struct {
	Status          string     `json:"status"`
	PercentComplete int        `json:"percent_complete"`
	Total           int        `json:"total"`
	Changed         int        `json:"changed"`
	Deleted         int        `json:"deleted"`
	Error           string     `json:"error,omitempty"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	ModifiedAt      time.Time  `json:"modified_at"`
}

type OAuthApp struct {
	ID                      uuid.UUID `json:"id"`
	UserID                  uuid.UUID `json:"user_id,omitempty"`
	Name                    string    `json:"name"`
	ClientID                string    `json:"client_id"`
	ClientSecret            string    `json:"client_secret,omitempty"`
	ClientSecretFingerprint string    `json:"client_secret_fingerprint,omitempty"`
	RedirectURIs            []string  `json:"redirect_uris"`
	Scopes                  []string  `json:"scopes"`
	CreatedAt               time.Time `json:"created_at"`
	ModifiedAt              time.Time `json:"modified_at"`
}

type OAuthAppInput struct {
	Name         string
	RedirectURIs []string
	Scopes       []string
}

type OAuthTokenResult struct {
	User         User
	App          OAuthApp
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	ExpiresIn    int
	Scopes       []string
}

type AuthResult struct {
	User    User
	Kind    string
	Subject string
	Scopes  []string
}

type ShareToken struct {
	ID          uuid.UUID  `json:"id"`
	UserID      uuid.UUID  `json:"user_id,omitempty"`
	Name        string     `json:"name"`
	Token       string     `json:"token,omitempty"`
	Fingerprint string     `json:"fingerprint"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type AICostSetting struct {
	Agent                     string    `json:"agent"`
	InputCostPerMillionCents  int       `json:"input_cost_per_million_cents"`
	OutputCostPerMillionCents int       `json:"output_cost_per_million_cents"`
	ModifiedAt                time.Time `json:"modified_at"`
}

type GitHubProfile struct {
	ID        int64
	Username  string
	Email     string
	FullName  string
	AvatarURL string
}

type PoolConfig struct {
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	return OpenWithPoolConfig(ctx, databaseURL, PoolConfig{
		MaxConns:        16,
		MinConns:        0,
		MaxConnLifetime: time.Hour,
		MaxConnIdleTime: 15 * time.Minute,
	})
}

func OpenWithPoolConfig(ctx context.Context, databaseURL string, poolConfig PoolConfig) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = poolConfig.MaxConns
	cfg.MinConns = poolConfig.MinConns
	cfg.MaxConnLifetime = poolConfig.MaxConnLifetime
	cfg.MaxConnIdleTime = poolConfig.MaxConnIdleTime
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() {
	if s != nil && s.Pool != nil {
		s.Pool.Close()
	}
}

const (
	migrationLockTimeout   = 30 * time.Second
	migrationUnlockTimeout = 5 * time.Second
)

func (s *Store) RunMigrations(ctx context.Context) error {
	lockCtx, cancelLock := context.WithTimeout(ctx, migrationLockTimeout)
	defer cancelLock()
	if _, err := s.Pool.Exec(lockCtx, `SELECT pg_advisory_lock(447196290717551)`); err != nil {
		return err
	}
	defer func() {
		unlockCtx, cancelUnlock := context.WithTimeout(context.Background(), migrationUnlockTimeout)
		defer cancelUnlock()
		_, _ = s.Pool.Exec(unlockCtx, `SELECT pg_advisory_unlock(447196290717551)`)
	}()

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		content, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		if _, err := s.Pool.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("%s: %w", entry.Name(), err)
		}
	}
	return nil
}

func (s *Store) ListProgramLanguages(ctx context.Context) ([]services.ProgramLanguage, error) {
	rows, err := s.Pool.Query(ctx, `SELECT name, color FROM program_languages ORDER BY lower(name) ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var languages []services.ProgramLanguage
	for rows.Next() {
		var language services.ProgramLanguage
		if err := rows.Scan(&language.Name, &language.Color); err != nil {
			return nil, err
		}
		languages = append(languages, language)
	}
	return languages, rows.Err()
}

func (s *Store) ListEditors(ctx context.Context) ([]services.EditorMetadata, error) {
	rows, err := s.Pool.Query(ctx, `SELECT name, key, coalesce(version, '') FROM editor_plugins ORDER BY lower(name) ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var editors []services.EditorMetadata
	for rows.Next() {
		var editor services.EditorMetadata
		if err := rows.Scan(&editor.Name, &editor.Key, &editor.Version); err != nil {
			return nil, err
		}
		editors = append(editors, editor)
	}
	return editors, rows.Err()
}

func (s *Store) UpsertGitHubUser(ctx context.Context, profile GitHubProfile) (User, error) {
	row := s.Pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO users (github_id, github_username, email, full_name, avatar_url)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (github_id) DO UPDATE SET
			github_username = EXCLUDED.github_username,
			email = EXCLUDED.email,
			full_name = EXCLUDED.full_name,
			avatar_url = EXCLUDED.avatar_url,
			modified_at = now()
		RETURNING %s`, userColumns),
		profile.ID, profile.Username, nullEmpty(profile.Email), nullEmpty(profile.FullName), nullEmpty(profile.AvatarURL))
	return scanUser(row)
}

func (s *Store) GetUser(ctx context.Context, id uuid.UUID) (User, error) {
	row := s.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM users WHERE id = $1`, userColumns), id)
	return scanUser(row)
}

func (s *Store) UpdateUser(ctx context.Context, id uuid.UUID, input UserSettingsInput) (User, error) {
	if err := ValidateUserSettings(input); err != nil {
		return User{}, err
	}
	input = NormalizeUserSettings(input)
	row := s.Pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE users SET timezone = $2, timeout_minutes = $3, writes_only = $4, has_public_profile = $5, country = $6,
			heartbeat_retention_days = $7, public_username = $8, public_display_name = $9, public_github_link_enabled = $10,
			public_show_total_time = $11, public_show_projects = $12, public_project_visibility = $13, public_show_languages = $14,
			public_show_editors = $15, public_show_machines = $16, public_show_operating_systems = $17, public_show_categories = $18,
			public_show_ai = $19, public_show_summaries = $20, public_profile = $21, modified_at = now()
		WHERE id = $1
		RETURNING %s`, userColumns),
		id, input.Timezone, input.TimeoutMinutes, input.WritesOnly, input.HasPublicProfile, nullEmpty(input.Country), input.HeartbeatRetentionDays,
		nullEmpty(input.PublicUsername), nullEmpty(input.PublicDisplayName), input.PublicGitHubLink, input.PublicShowTotalTime, input.PublicShowProjects,
		input.PublicProjectVisibility, input.PublicShowLanguages, input.PublicShowEditors, input.PublicShowMachines, input.PublicShowOS,
		input.PublicShowCategories, input.PublicShowAI, input.PublicShowSummaries, input.PublicProfile)
	user, err := scanUser(row)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.ConstraintName == "users_public_username_lower_idx" {
			return User{}, ErrDuplicatePublicUsername
		}
		return User{}, err
	}
	if err := s.MarkStatsStale(ctx, id); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Store) DeleteUser(ctx context.Context, id uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) CreateSession(ctx context.Context, userID uuid.UUID) (string, error) {
	token, err := randomHex(32)
	if err != nil {
		return "", err
	}
	hash := tokenHash(token)
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`, userID, hash, time.Now().Add(30*24*time.Hour))
	return token, err
}

func (s *Store) UserBySessionToken(ctx context.Context, token string) (User, error) {
	row := s.Pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, userColumnsU), tokenHash(token))
	return scanUser(row)
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash(token))
	return err
}

func (s *Store) UserByID(ctx context.Context, userID uuid.UUID) (User, error) {
	row := s.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM users WHERE id = $1`, userColumns), userID)
	return scanUser(row)
}

func (s *Store) UserByGitHubUsername(ctx context.Context, username string) (User, error) {
	row := s.Pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s
		FROM users WHERE lower(github_username) = lower($1)
		ORDER BY created_at ASC
		LIMIT 1`, userColumns), strings.TrimSpace(username))
	return scanUser(row)
}

func (s *Store) UserByGitHubID(ctx context.Context, githubID int64) (User, error) {
	row := s.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM users WHERE github_id = $1`, userColumns), githubID)
	return scanUser(row)
}

func (s *Store) PublicUserByRef(ctx context.Context, userRef string) (User, error) {
	userRef = strings.TrimPrefix(strings.TrimSpace(userRef), "@")
	if id, err := uuid.Parse(userRef); err == nil {
		row := s.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM users WHERE id = $1 AND has_public_profile = true`, userColumns), id)
		return scanUser(row)
	}
	row := s.Pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s
		FROM users
		WHERE has_public_profile = true
			AND (lower(public_username) = lower($1) OR lower(github_username) = lower($1))
		ORDER BY CASE WHEN lower(public_username) = lower($1) THEN 0 ELSE 1 END, created_at ASC
		LIMIT 1`, userColumns), userRef)
	return scanUser(row)
}

func (s *Store) CreateAPIKey(ctx context.Context, userID uuid.UUID, name string) (APIKey, string, error) {
	return s.CreateAPIKeyWithScopes(ctx, userID, name, nil)
}

func ValidateAPIKeyName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: API key name is required", ErrInvalidResourceName)
	}
	return nil
}

func (s *Store) CreateAPIKeyWithScopes(ctx context.Context, userID uuid.UUID, name string, scopes []string) (APIKey, string, error) {
	name = strings.TrimSpace(name)
	if err := ValidateAPIKeyName(name); err != nil {
		return APIKey{}, "", err
	}
	scopes, err := normalizeAPIKeyScopes(scopes)
	if err != nil {
		return APIKey{}, "", err
	}
	key, fingerprint, err := auth.GenerateAPIKey()
	if err != nil {
		return APIKey{}, "", err
	}
	hash, err := auth.HashAPIKey(key)
	if err != nil {
		return APIKey{}, "", err
	}
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO api_keys (user_id, name, key_hash, key_fingerprint, scopes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, name, key_fingerprint, scopes, last_used_at, created_at`,
		userID, name, hash, fingerprint, scopes)
	apiKey, err := scanAPIKey(row)
	return apiKey, key, err
}

func (s *Store) ListAPIKeys(ctx context.Context, userID uuid.UUID) ([]APIKey, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, user_id, name, key_fingerprint, scopes, last_used_at, created_at
		FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []APIKey
	for rows.Next() {
		key, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Store) RevokeAPIKey(ctx context.Context, userID, keyID uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM api_keys WHERE user_id = $1 AND id = $2`, userID, keyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) UserByAPIKey(ctx context.Context, key string) (User, error) {
	result, err := s.AuthByAPIKey(ctx, key)
	if err != nil {
		return User{}, err
	}
	return result.User, nil
}

func (s *Store) AuthByAPIKey(ctx context.Context, key string) (AuthResult, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		SELECT k.id, k.key_hash, k.scopes, %s
		FROM api_keys k
		JOIN users u ON u.id = k.user_id
		WHERE k.key_fingerprint = $1`, userColumnsU), auth.KeyFingerprint(key))
	if err != nil {
		return AuthResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var keyID uuid.UUID
		var hash string
		var scopes []string
		var user User
		destinations := append([]any{&keyID, &hash, &scopes}, userScanDestinations(&user)...)
		if err := rows.Scan(destinations...); err != nil {
			return AuthResult{}, err
		}
		if s.verifyTokenHash(ctx, hash, key, func(ctx context.Context, token string) error {
			return s.upgradeAPIKeyHash(ctx, keyID, token)
		}) {
			_, _ = s.Pool.Exec(ctx, `UPDATE api_keys SET last_used_at = now() WHERE id = $1`, keyID)
			return AuthResult{User: user, Kind: "api_key", Subject: keyID.String(), Scopes: scopes}, nil
		}
	}
	if err := rows.Err(); err != nil {
		return AuthResult{}, err
	}
	return AuthResult{}, pgx.ErrNoRows
}

func (s *Store) CreateOAuthApp(ctx context.Context, userID uuid.UUID, name string, redirectURIs, scopes []string) (OAuthApp, error) {
	if err := ValidateOAuthAppInput(OAuthAppInput{Name: name, RedirectURIs: redirectURIs, Scopes: scopes}); err != nil {
		return OAuthApp{}, err
	}
	name = strings.TrimSpace(name)
	redirectURIs = normalizeStringList(redirectURIs)
	if len(redirectURIs) == 0 {
		return OAuthApp{}, errors.New("at least one redirect URI is required")
	}
	scopes, err := normalizeOAuthScopes(scopes)
	if err != nil {
		return OAuthApp{}, err
	}
	clientID := "stintc_" + uuid.NewString()
	secret, fingerprint, hash, err := auth.GenerateOAuthClientSecret()
	if err != nil {
		return OAuthApp{}, err
	}
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO oauth_apps (user_id, name, client_id, client_secret_hash, client_secret_fingerprint, redirect_uris, scopes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, name, client_id, client_secret_fingerprint, redirect_uris, scopes, created_at, modified_at`,
		userID, name, clientID, hash, fingerprint, redirectURIs, scopes)
	app, err := scanOAuthApp(row)
	if err != nil {
		return OAuthApp{}, err
	}
	app.ClientSecret = secret
	return app, nil
}

func (s *Store) ListOAuthApps(ctx context.Context, userID uuid.UUID) ([]OAuthApp, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, user_id, name, client_id, client_secret_fingerprint, redirect_uris, scopes, created_at, modified_at
		FROM oauth_apps
		WHERE user_id = $1
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var apps []OAuthApp
	for rows.Next() {
		app, err := scanOAuthApp(rows)
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

func (s *Store) DeleteOAuthApp(ctx context.Context, userID, appID uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM oauth_apps WHERE user_id = $1 AND id = $2`, userID, appID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) OAuthAppByClientID(ctx context.Context, clientID string) (OAuthApp, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, user_id, name, client_id, client_secret_fingerprint, redirect_uris, scopes, created_at, modified_at
		FROM oauth_apps
		WHERE client_id = $1`, clientID)
	return scanOAuthApp(row)
}

func (s *Store) VerifyOAuthAppSecret(ctx context.Context, clientID, secret string) (OAuthApp, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, user_id, name, client_id, client_secret_fingerprint, redirect_uris, scopes, created_at, modified_at, client_secret_hash
		FROM oauth_apps
		WHERE client_id = $1`, clientID)
	app, hash, err := scanOAuthAppWithHash(row)
	if err != nil {
		return OAuthApp{}, err
	}
	if !s.verifyTokenHash(ctx, hash, secret, func(ctx context.Context, token string) error {
		return s.upgradeOAuthAppSecretHash(ctx, app.ID, token)
	}) {
		return OAuthApp{}, pgx.ErrNoRows
	}
	return app, nil
}

func (s *Store) CreateOAuthAuthorizationCode(ctx context.Context, userID, appID uuid.UUID, redirectURI string, scopes []string) (string, error) {
	var err error
	scopes, err = normalizeOAuthScopes(scopes)
	if err != nil {
		return "", err
	}
	code, fingerprint, hash, err := auth.GenerateOAuthCode()
	if err != nil {
		return "", err
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO oauth_authorization_codes (app_id, user_id, code_hash, code_fingerprint, redirect_uri, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, now() + interval '10 minutes')`,
		appID, userID, hash, fingerprint, redirectURI, scopes)
	return code, err
}

func (s *Store) CreateOAuthImplicitToken(ctx context.Context, user User, app OAuthApp, scopes []string, ttl time.Duration) (OAuthTokenResult, error) {
	return insertOAuthAccessToken(ctx, s.Pool, user, app, scopes, ttl)
}

func (s *Store) OAuthAuthorizationCodeUserID(ctx context.Context, clientID, code, redirectURI string) (uuid.UUID, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT c.user_id, c.code_hash
		FROM oauth_authorization_codes c
		JOIN oauth_apps a ON a.id = c.app_id
		WHERE c.code_fingerprint = $1
			AND a.client_id = $2
			AND c.redirect_uri = $3
			AND c.expires_at > now()
			AND c.used_at IS NULL`,
		auth.KeyFingerprint(code), clientID, redirectURI)
	if err != nil {
		return uuid.Nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var userID uuid.UUID
		var hash string
		if err := rows.Scan(&userID, &hash); err != nil {
			return uuid.Nil, err
		}
		if s.verifyTokenHash(ctx, hash, code, nil) {
			return userID, nil
		}
	}
	if err := rows.Err(); err != nil {
		return uuid.Nil, err
	}
	return uuid.Nil, pgx.ErrNoRows
}

func (s *Store) ExchangeOAuthAuthorizationCode(ctx context.Context, clientID, code, redirectURI string, ttl time.Duration) (OAuthTokenResult, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		SELECT c.id, c.code_hash, c.scopes,
			%s,
			a.id, a.user_id, a.name, a.client_id, a.client_secret_fingerprint, a.redirect_uris, a.scopes, a.created_at, a.modified_at
		FROM oauth_authorization_codes c
		JOIN oauth_apps a ON a.id = c.app_id
		JOIN users u ON u.id = c.user_id
		WHERE c.code_fingerprint = $1
			AND a.client_id = $2
			AND c.redirect_uri = $3
			AND c.expires_at > now()
			AND c.used_at IS NULL`, userColumnsU),
		auth.KeyFingerprint(code), clientID, redirectURI)
	if err != nil {
		return OAuthTokenResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var codeID uuid.UUID
		var hash string
		var scopes []string
		var user User
		var app OAuthApp
		destinations := append([]any{&codeID, &hash, &scopes}, userScanDestinations(&user)...)
		destinations = append(destinations, &app.ID, &app.UserID, &app.Name, &app.ClientID, &app.ClientSecretFingerprint, &app.RedirectURIs, &app.Scopes, &app.CreatedAt, &app.ModifiedAt)
		if err := rows.Scan(destinations...); err != nil {
			return OAuthTokenResult{}, err
		}
		if !s.verifyTokenHash(ctx, hash, code, func(ctx context.Context, token string) error {
			return s.upgradeOAuthAuthorizationCodeHash(ctx, codeID, token)
		}) {
			continue
		}
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return OAuthTokenResult{}, err
		}
		defer tx.Rollback(ctx)
		tag, err := tx.Exec(ctx, `UPDATE oauth_authorization_codes SET used_at = now() WHERE id = $1 AND used_at IS NULL`, codeID)
		if err != nil {
			return OAuthTokenResult{}, err
		}
		if tag.RowsAffected() == 0 {
			return OAuthTokenResult{}, pgx.ErrNoRows
		}
		result, err := insertOAuthTokenPair(ctx, tx, user, app, scopes, ttl)
		if err != nil {
			return OAuthTokenResult{}, err
		}
		return result, tx.Commit(ctx)
	}
	if err := rows.Err(); err != nil {
		return OAuthTokenResult{}, err
	}
	return OAuthTokenResult{}, pgx.ErrNoRows
}

func (s *Store) OAuthRefreshTokenUserID(ctx context.Context, clientID, refreshToken string) (uuid.UUID, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT t.user_id, t.refresh_token_hash
		FROM oauth_tokens t
		JOIN oauth_apps a ON a.id = t.app_id
		WHERE t.refresh_token_fingerprint = $1
			AND a.client_id = $2
			AND t.revoked_at IS NULL
			AND t.expires_at > now()`,
		auth.KeyFingerprint(refreshToken), clientID)
	if err != nil {
		return uuid.Nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var userID uuid.UUID
		var hash string
		if err := rows.Scan(&userID, &hash); err != nil {
			return uuid.Nil, err
		}
		if s.verifyTokenHash(ctx, hash, refreshToken, nil) {
			return userID, nil
		}
	}
	if err := rows.Err(); err != nil {
		return uuid.Nil, err
	}
	return uuid.Nil, pgx.ErrNoRows
}

func (s *Store) RefreshOAuthToken(ctx context.Context, clientID, refreshToken string, ttl time.Duration) (OAuthTokenResult, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		SELECT t.id, t.refresh_token_hash, t.scopes,
			%s,
			a.id, a.user_id, a.name, a.client_id, a.client_secret_fingerprint, a.redirect_uris, a.scopes, a.created_at, a.modified_at
		FROM oauth_tokens t
		JOIN oauth_apps a ON a.id = t.app_id
		JOIN users u ON u.id = t.user_id
		WHERE t.refresh_token_fingerprint = $1
			AND a.client_id = $2
			AND t.revoked_at IS NULL
			AND t.expires_at > now()`, userColumnsU),
		auth.KeyFingerprint(refreshToken), clientID)
	if err != nil {
		return OAuthTokenResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var tokenID uuid.UUID
		var hash string
		var scopes []string
		var user User
		var app OAuthApp
		destinations := append([]any{&tokenID, &hash, &scopes}, userScanDestinations(&user)...)
		destinations = append(destinations, &app.ID, &app.UserID, &app.Name, &app.ClientID, &app.ClientSecretFingerprint, &app.RedirectURIs, &app.Scopes, &app.CreatedAt, &app.ModifiedAt)
		if err := rows.Scan(destinations...); err != nil {
			return OAuthTokenResult{}, err
		}
		if !s.verifyTokenHash(ctx, hash, refreshToken, func(ctx context.Context, token string) error {
			return s.upgradeOAuthRefreshTokenHash(ctx, tokenID, token)
		}) {
			continue
		}
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return OAuthTokenResult{}, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `UPDATE oauth_tokens SET revoked_at = now() WHERE id = $1`, tokenID); err != nil {
			return OAuthTokenResult{}, err
		}
		result, err := insertOAuthTokenPair(ctx, tx, user, app, scopes, ttl)
		if err != nil {
			return OAuthTokenResult{}, err
		}
		return result, tx.Commit(ctx)
	}
	if err := rows.Err(); err != nil {
		return OAuthTokenResult{}, err
	}
	return OAuthTokenResult{}, pgx.ErrNoRows
}

func (s *Store) RevokeOAuthToken(ctx context.Context, clientID, token string) error {
	fingerprint := auth.KeyFingerprint(token)
	rows, err := s.Pool.Query(ctx, `
		SELECT t.id, t.access_token_hash, coalesce(t.refresh_token_hash, '')
		FROM oauth_tokens t
		JOIN oauth_apps a ON a.id = t.app_id
		WHERE (t.access_token_fingerprint = $1 OR t.refresh_token_fingerprint = $1)
			AND a.client_id = $2
			AND t.revoked_at IS NULL`, fingerprint, clientID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var tokenID uuid.UUID
		var accessHash, refreshHash string
		if err := rows.Scan(&tokenID, &accessHash, &refreshHash); err != nil {
			return err
		}
		if s.verifyTokenHash(ctx, accessHash, token, func(ctx context.Context, value string) error {
			return s.upgradeOAuthAccessTokenHash(ctx, tokenID, value)
		}) || s.verifyTokenHash(ctx, refreshHash, token, func(ctx context.Context, value string) error {
			return s.upgradeOAuthRefreshTokenHash(ctx, tokenID, value)
		}) {
			_, err := s.Pool.Exec(ctx, `UPDATE oauth_tokens SET revoked_at = now() WHERE id = $1`, tokenID)
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return pgx.ErrNoRows
}

func (s *Store) UserByOAuthAccessToken(ctx context.Context, token string) (User, error) {
	result, err := s.AuthByOAuthAccessToken(ctx, token)
	if err != nil {
		return User{}, err
	}
	return result.User, nil
}

func (s *Store) AuthByOAuthAccessToken(ctx context.Context, token string) (AuthResult, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		SELECT t.id, t.access_token_hash, t.scopes, %s
		FROM oauth_tokens t
		JOIN users u ON u.id = t.user_id
		WHERE t.access_token_fingerprint = $1
			AND t.revoked_at IS NULL
			AND t.expires_at > now()`, userColumnsU), auth.KeyFingerprint(token))
	if err != nil {
		return AuthResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var tokenID uuid.UUID
		var hash string
		var scopes []string
		var user User
		destinations := append([]any{&tokenID, &hash, &scopes}, userScanDestinations(&user)...)
		if err := rows.Scan(destinations...); err != nil {
			return AuthResult{}, err
		}
		if s.verifyTokenHash(ctx, hash, token, func(ctx context.Context, value string) error {
			return s.upgradeOAuthAccessTokenHash(ctx, tokenID, value)
		}) {
			_, _ = s.Pool.Exec(ctx, `UPDATE oauth_tokens SET last_used_at = now() WHERE id = $1`, tokenID)
			return AuthResult{User: user, Kind: "oauth", Subject: tokenID.String(), Scopes: scopes}, nil
		}
	}
	if err := rows.Err(); err != nil {
		return AuthResult{}, err
	}
	return AuthResult{}, pgx.ErrNoRows
}

func ValidateShareTokenName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: share token name is required", ErrInvalidResourceName)
	}
	return nil
}

func (s *Store) CreateShareToken(ctx context.Context, userID uuid.UUID, name string) (ShareToken, error) {
	name = strings.TrimSpace(name)
	if err := ValidateShareTokenName(name); err != nil {
		return ShareToken{}, err
	}
	token, err := randomPrefixedToken("stintshare_", 24)
	if err != nil {
		return ShareToken{}, err
	}
	hash, err := auth.HashAPIKey(token)
	if err != nil {
		return ShareToken{}, err
	}
	fingerprint := auth.KeyFingerprint(token)
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO share_tokens (user_id, name, token_hash, token_fingerprint)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, name, token_fingerprint, last_used_at, created_at`,
		userID, name, hash, fingerprint)
	share, err := scanShareToken(row)
	if err != nil {
		return ShareToken{}, err
	}
	share.Token = token
	return share, nil
}

func (s *Store) ListShareTokens(ctx context.Context, userID uuid.UUID) ([]ShareToken, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, user_id, name, token_fingerprint, last_used_at, created_at
		FROM share_tokens
		WHERE user_id = $1
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tokens []ShareToken
	for rows.Next() {
		token, err := scanShareToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

func (s *Store) DeleteShareToken(ctx context.Context, userID, tokenID uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM share_tokens WHERE user_id = $1 AND id = $2`, userID, tokenID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) UserByShareToken(ctx context.Context, userRef, token string) (User, error) {
	userRef = strings.TrimPrefix(strings.TrimSpace(userRef), "@")
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		SELECT st.id, st.token_hash, %s
		FROM share_tokens st
		JOIN users u ON u.id = st.user_id
		WHERE st.token_fingerprint = $1
			AND (u.id::text = $2 OR lower(u.github_username) = lower($2) OR lower(u.public_username) = lower($2))`, userColumnsU),
		auth.KeyFingerprint(token), userRef)
	if err != nil {
		return User{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var tokenID uuid.UUID
		var hash string
		var user User
		destinations := append([]any{&tokenID, &hash}, userScanDestinations(&user)...)
		if err := rows.Scan(destinations...); err != nil {
			return User{}, err
		}
		if s.verifyTokenHash(ctx, hash, token, func(ctx context.Context, value string) error {
			return s.upgradeShareTokenHash(ctx, tokenID, value)
		}) {
			_, _ = s.Pool.Exec(ctx, `UPDATE share_tokens SET last_used_at = now() WHERE id = $1`, tokenID)
			return user, nil
		}
	}
	if err := rows.Err(); err != nil {
		return User{}, err
	}
	return User{}, pgx.ErrNoRows
}

func (s *Store) UserByShareTokenOnly(ctx context.Context, token string) (User, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		SELECT st.id, st.token_hash, %s
		FROM share_tokens st
		JOIN users u ON u.id = st.user_id
		WHERE st.token_fingerprint = $1`, userColumnsU),
		auth.KeyFingerprint(token))
	if err != nil {
		return User{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var tokenID uuid.UUID
		var hash string
		var user User
		destinations := append([]any{&tokenID, &hash}, userScanDestinations(&user)...)
		if err := rows.Scan(destinations...); err != nil {
			return User{}, err
		}
		if s.verifyTokenHash(ctx, hash, token, func(ctx context.Context, value string) error {
			return s.upgradeShareTokenHash(ctx, tokenID, value)
		}) {
			_, _ = s.Pool.Exec(ctx, `UPDATE share_tokens SET last_used_at = now() WHERE id = $1`, tokenID)
			return user, nil
		}
	}
	if err := rows.Err(); err != nil {
		return User{}, err
	}
	return User{}, pgx.ErrNoRows
}

func (s *Store) ListAICostSettings(ctx context.Context, userID uuid.UUID) ([]AICostSetting, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT agent, input_cost_per_million_cents, output_cost_per_million_cents, modified_at
		FROM ai_cost_settings
		WHERE user_id = $1
		ORDER BY agent ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var settings []AICostSetting
	for rows.Next() {
		setting, err := scanAICostSetting(rows)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	return settings, rows.Err()
}

func (s *Store) AICostRates(ctx context.Context, userID uuid.UUID) (map[string]services.AICostRate, error) {
	settings, err := s.ListAICostSettings(ctx, userID)
	if err != nil {
		return nil, err
	}
	rates := map[string]services.AICostRate{}
	for _, setting := range settings {
		rates[setting.Agent] = services.AICostRate{
			InputCostPerMillionCents:  setting.InputCostPerMillionCents,
			OutputCostPerMillionCents: setting.OutputCostPerMillionCents,
		}
	}
	return rates, nil
}

func (s *Store) ReplaceAICostSettings(ctx context.Context, userID uuid.UUID, settings []AICostSetting) ([]AICostSetting, error) {
	if err := ValidateAICostSettings(settings); err != nil {
		return nil, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM ai_cost_settings WHERE user_id = $1`, userID); err != nil {
		return nil, err
	}
	if len(settings) > 0 {
		batch := &pgx.Batch{}
		for _, setting := range settings {
			agent := strings.TrimSpace(setting.Agent)
			batch.Queue(`
			INSERT INTO ai_cost_settings (user_id, agent, input_cost_per_million_cents, output_cost_per_million_cents)
			VALUES ($1, $2, $3, $4)`,
				userID, agent, setting.InputCostPerMillionCents, setting.OutputCostPerMillionCents)
		}
		results := tx.SendBatch(ctx, batch)
		for range settings {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return nil, err
			}
		}
		if err := results.Close(); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if err := s.MarkStatsStale(ctx, userID); err != nil {
		return nil, err
	}
	return s.ListAICostSettings(ctx, userID)
}

func ValidateAICostSettings(settings []AICostSetting) error {
	if len(settings) > 50 {
		return errors.New("AI cost settings limit is 50 agents")
	}
	for _, setting := range settings {
		if strings.TrimSpace(setting.Agent) == "" {
			return errors.New("agent is required")
		}
		if setting.InputCostPerMillionCents < 0 || setting.OutputCostPerMillionCents < 0 {
			return errors.New("AI cost settings must be non-negative")
		}
	}
	return nil
}

func (s *Store) InsertHeartbeat(ctx context.Context, userID uuid.UUID, heartbeat services.Heartbeat) (services.Heartbeat, error) {
	machineID, err := s.upsertMachine(ctx, userID, heartbeat.MachineName)
	if err != nil {
		return heartbeat, err
	}
	if heartbeat.Project != "" {
		if err := s.upsertProject(ctx, userID, heartbeat.Project, heartbeat.Time); err != nil {
			return heartbeat, err
		}
	}
	if heartbeat.Type == "" {
		heartbeat.Type = "file"
	}

	var id uuid.UUID
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO heartbeats (
			user_id, entity, type, category, time, project, branch, language, machine_name_id,
			machine_name, plugin, plugin_version, editor, editor_version, operating_system, architecture,
			dependencies, lines, line_number, cursor_pos, is_write, ai_line_changes, human_line_changes,
			ai_session, ai_input_tokens, ai_output_tokens, ai_prompt_length, ai_subscription_plan,
			ai_model, ai_provider, ai_agent, ai_agent_version, ai_agent_complexity, commit_hash, metadata, raw_payload
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13, $14, $15, $16,
			$17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28,
			$29, $30, $31, $32, $33, $34, $35::jsonb, $36::jsonb
		)
		RETURNING id`,
		userID, heartbeat.Entity, heartbeat.Type, nullEmpty(heartbeat.Category), heartbeat.Time,
		nullEmpty(heartbeat.Project), nullEmpty(heartbeat.Branch), nullEmpty(heartbeat.Language), machineID,
		nullEmpty(heartbeat.MachineName), nullEmpty(heartbeat.Plugin), nullEmpty(heartbeat.PluginVersion),
		nullEmpty(heartbeat.Editor), nullEmpty(heartbeat.EditorVersion), nullEmpty(heartbeat.OperatingSystem),
		nullEmpty(heartbeat.Architecture), nullEmpty(heartbeat.Dependencies), heartbeat.Lines, heartbeat.LineNumber,
		heartbeat.CursorPosition, heartbeat.IsWrite, heartbeat.AILineChanges, heartbeat.HumanLineChanges, nullEmpty(heartbeat.AISession),
		heartbeat.AIInputTokens, heartbeat.AIOutputTokens, heartbeat.AIPromptLength, nullEmpty(heartbeat.AISubscriptionPlan),
		nullEmpty(heartbeat.AIModel), nullEmpty(heartbeat.AIProvider), nullEmpty(heartbeat.AIAgent), nullEmpty(heartbeat.AIAgentVersion),
		nullEmpty(heartbeat.AIAgentComplexity), nullEmpty(heartbeat.CommitHash), jsonMapArg(heartbeat.Metadata), jsonMapArg(heartbeat.RawPayload)).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return heartbeat, ErrDuplicateHeartbeat
		}
		return heartbeat, err
	}
	heartbeat.ID = id.String()
	if err := s.MarkStatsStale(ctx, userID); err != nil {
		return heartbeat, err
	}
	return heartbeat, nil
}

type HeartbeatInsertResult struct {
	Heartbeat services.Heartbeat
	Duplicate bool
	Stored    bool
	Err       error
}

type WakaTimeImport struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Heartbeats  []services.Heartbeat
	TotalCount  int
	CreatedAt   time.Time
	ProcessedAt *time.Time
}

func (s *Store) CreateWakaTimeImport(ctx context.Context, userID uuid.UUID, heartbeats []services.Heartbeat) (WakaTimeImport, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return WakaTimeImport{}, err
	}
	defer tx.Rollback(ctx)
	var importRow WakaTimeImport
	if err := tx.QueryRow(ctx, `
		INSERT INTO wakatime_imports (user_id, heartbeats, total_count)
		VALUES ($1, '[]'::jsonb, $2)
		RETURNING id, user_id, created_at, processed_at`, userID, len(heartbeats)).
		Scan(&importRow.ID, &importRow.UserID, &importRow.CreatedAt, &importRow.ProcessedAt); err != nil {
		return WakaTimeImport{}, err
	}
	batch := &pgx.Batch{}
	chunkCount := 0
	for start := 0; start < len(heartbeats); start += wakaTimeImportChunkSize {
		end := min(start+wakaTimeImportChunkSize, len(heartbeats))
		data, err := json.Marshal(heartbeats[start:end])
		if err != nil {
			return WakaTimeImport{}, err
		}
		batch.Queue(`
			INSERT INTO wakatime_import_chunks (import_id, chunk_index, heartbeats)
			VALUES ($1, $2, $3::jsonb)`, importRow.ID, chunkCount, data)
		chunkCount++
	}
	if chunkCount > 0 {
		results := tx.SendBatch(ctx, batch)
		for i := 0; i < chunkCount; i++ {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return WakaTimeImport{}, err
			}
		}
		if err := results.Close(); err != nil {
			return WakaTimeImport{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return WakaTimeImport{}, err
	}
	importRow.Heartbeats = heartbeats
	importRow.TotalCount = len(heartbeats)
	return importRow, nil
}

func (s *Store) GetWakaTimeImport(ctx context.Context, userID, importID uuid.UUID) (WakaTimeImport, error) {
	var importRow WakaTimeImport
	var legacyRaw []byte
	if err := s.Pool.QueryRow(ctx, `
		SELECT id, user_id, total_count, heartbeats, created_at, processed_at
		FROM wakatime_imports
		WHERE user_id = $1 AND id = $2`, userID, importID).
		Scan(&importRow.ID, &importRow.UserID, &importRow.TotalCount, &legacyRaw, &importRow.CreatedAt, &importRow.ProcessedAt); err != nil {
		return WakaTimeImport{}, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT heartbeats
		FROM wakatime_import_chunks
		WHERE import_id = $1
		ORDER BY chunk_index ASC`, importID)
	if err != nil {
		return WakaTimeImport{}, err
	}
	defer rows.Close()
	var heartbeats []services.Heartbeat
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return WakaTimeImport{}, err
		}
		var chunk []services.Heartbeat
		if err := json.Unmarshal(raw, &chunk); err != nil {
			return WakaTimeImport{}, err
		}
		heartbeats = append(heartbeats, chunk...)
	}
	if err := rows.Err(); err != nil {
		return WakaTimeImport{}, err
	}
	if len(heartbeats) == 0 && len(legacyRaw) > 0 {
		if err := json.Unmarshal(legacyRaw, &heartbeats); err != nil {
			return WakaTimeImport{}, err
		}
	}
	importRow.Heartbeats = heartbeats
	if importRow.TotalCount == 0 {
		importRow.TotalCount = len(heartbeats)
	}
	return importRow, nil
}

func (s *Store) MarkWakaTimeImportProcessed(ctx context.Context, userID, importID uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE wakatime_imports
		SET processed_at = now()
		WHERE user_id = $1 AND id = $2`, userID, importID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) InsertHeartbeats(ctx context.Context, userID uuid.UUID, heartbeats []services.Heartbeat) ([]HeartbeatInsertResult, error) {
	return s.insertHeartbeatsWithCopyStaging(ctx, userID, heartbeats)
}

func (s *Store) HeartbeatsBetween(ctx context.Context, userID uuid.UUID, start, end float64) ([]services.Heartbeat, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		SELECT %s
		FROM heartbeats
		WHERE user_id = $1 AND time >= $2 AND time < $3
		ORDER BY time ASC`, heartbeatSelectColumns), userID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var heartbeats []services.Heartbeat
	for rows.Next() {
		heartbeat, err := scanHeartbeat(rows)
		if err != nil {
			return nil, err
		}
		heartbeats = append(heartbeats, heartbeat)
	}
	return heartbeats, rows.Err()
}

func (s *Store) HeartbeatsForEntity(ctx context.Context, userID uuid.UUID, entity, project string) ([]services.Heartbeat, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		SELECT %s
		FROM heartbeats
		WHERE user_id = $1 AND entity = $2 AND ($3 = '' OR project = $3)
		ORDER BY time ASC`, heartbeatSelectColumns), userID, entity, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var heartbeats []services.Heartbeat
	for rows.Next() {
		heartbeat, err := scanHeartbeat(rows)
		if err != nil {
			return nil, err
		}
		heartbeats = append(heartbeats, heartbeat)
	}
	return heartbeats, rows.Err()
}

func (s *Store) HeartbeatsForProject(ctx context.Context, userID uuid.UUID, project string) ([]services.Heartbeat, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		SELECT %s
		FROM heartbeats
		WHERE user_id = $1 AND project = $2
		ORDER BY time ASC`, heartbeatSelectColumns), userID, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var heartbeats []services.Heartbeat
	for rows.Next() {
		heartbeat, err := scanHeartbeat(rows)
		if err != nil {
			return nil, err
		}
		heartbeats = append(heartbeats, heartbeat)
	}
	return heartbeats, rows.Err()
}

func (s *Store) HeartbeatsForProjectStatsRange(ctx context.Context, userID uuid.UUID, project string, now time.Time, rangeName string) ([]services.Heartbeat, error) {
	if rangeName == "all_time" {
		return s.HeartbeatsForProject(ctx, userID, project)
	}
	window, err := services.WindowForRange(now, rangeName)
	if err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		SELECT %s
		FROM heartbeats
		WHERE user_id = $1 AND project = $2 AND time >= $3 AND time < $4
		ORDER BY time ASC`, heartbeatSelectColumns), userID, project, float64(window.Start.Unix()), float64(window.End.Unix()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var heartbeats []services.Heartbeat
	for rows.Next() {
		heartbeat, err := scanHeartbeat(rows)
		if err != nil {
			return nil, err
		}
		heartbeats = append(heartbeats, heartbeat)
	}
	return heartbeats, rows.Err()
}

// IngestionStats reports global heartbeat-ingestion freshness so an external
// monitor can detect a stalled feed (e.g. a dead fanout target or a
// misconfigured editor). It returns coarse counts only, never file or project
// detail. now is the reference epoch second.
type IngestionStats struct {
	LastHeartbeatTime float64
	CountLastHour     int
	CountLast24h      int
}

func (s *Store) IngestionStats(ctx context.Context, now float64) (IngestionStats, error) {
	var stats IngestionStats
	err := s.Pool.QueryRow(ctx, `
		SELECT time
		FROM heartbeats
		ORDER BY time DESC
		LIMIT 1`).Scan(&stats.LastHeartbeatTime)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return IngestionStats{}, err
	}
	if err := s.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM heartbeats
		WHERE time >= $1 - 3600`, now).Scan(&stats.CountLastHour); err != nil {
		return IngestionStats{}, err
	}
	if err := s.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM heartbeats
		WHERE time >= $1 - 86400`, now).Scan(&stats.CountLast24h); err != nil {
		return IngestionStats{}, err
	}
	return stats, nil
}

func (s *Store) ListUserAgents(ctx context.Context, userID uuid.UUID) ([]UserAgent, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT
			encode(substring(digest(
				coalesce(plugin, '') || '|' || coalesce(plugin_version, '') || '|' ||
				coalesce(editor, '') || '|' || coalesce(editor_version, '') || '|' ||
				coalesce(operating_system, '') || '|' || coalesce(architecture, '') || '|' ||
				coalesce(ai_agent, '') || '|' || coalesce(ai_agent_version, '') || '|' ||
				coalesce(ai_agent_complexity, ''),
				'sha256'
			) from 1 for 16), 'hex') AS id,
			coalesce(plugin, ''), coalesce(plugin_version, ''), coalesce(editor, ''), coalesce(editor_version, ''),
			coalesce(operating_system, ''), coalesce(architecture, ''), coalesce(ai_agent, ''),
			coalesce(ai_agent_version, ''), coalesce(ai_agent_complexity, ''),
			coalesce((array_remove(array_agg(nullif(ai_model, '') ORDER BY time DESC), NULL))[1], '') AS ai_model,
			coalesce((array_remove(array_agg(nullif(ai_provider, '') ORDER BY time DESC), NULL))[1], '') AS ai_provider,
			min(created_at), max(created_at)
		FROM heartbeats
		WHERE user_id = $1
		GROUP BY plugin, plugin_version, editor, editor_version, operating_system, architecture,
			ai_agent, ai_agent_version, ai_agent_complexity
		ORDER BY max(created_at) DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := []UserAgent{}
	for rows.Next() {
		var id, plugin, pluginVersion, editor, editorVersion, osName, arch, aiAgent, aiAgentVersion, aiAgentComplexity, aiModel, aiProvider string
		var createdAt, lastSeenAt time.Time
		if err := rows.Scan(&id, &plugin, &pluginVersion, &editor, &editorVersion, &osName, &arch, &aiAgent, &aiAgentVersion, &aiAgentComplexity, &aiModel, &aiProvider, &createdAt, &lastSeenAt); err != nil {
			return nil, err
		}
		value := formatUserAgentValue(plugin, pluginVersion, editor, editorVersion, osName, arch)
		displayEditor := editor
		if displayEditor == "" {
			displayEditor = "Unknown"
		}
		agents = append(agents, UserAgent{
			ID:                 id,
			Value:              value,
			Editor:             displayEditor,
			AIModel:            aiModel,
			AIProvider:         aiProvider,
			AIAgent:            aiAgent,
			AIAgentVersion:     aiAgentVersion,
			AIAgentComplexity:  aiAgentComplexity,
			Version:            pluginVersion,
			OS:                 osName,
			LastSeenAt:         lastSeenAt,
			IsBrowserExtension: strings.Contains(value, "browser-wakatime"),
			IsDesktopApp:       strings.Contains(value, "macos-wakatime"),
			CreatedAt:          createdAt,
		})
	}
	return agents, rows.Err()
}

func formatUserAgentValue(plugin, pluginVersion, editor, editorVersion, osName, arch string) string {
	parts := []string{}
	if plugin != "" {
		value := plugin
		if pluginVersion != "" {
			value += "/" + pluginVersion
		}
		parts = append(parts, value)
	}
	if osName != "" {
		value := osName
		if arch != "" {
			value += "-" + arch
		}
		parts = append(parts, "("+value+")")
	}
	if editor != "" && editor != "Unknown" {
		value := editor
		if editorVersion != "" {
			value += "/" + editorVersion
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, " ")
}

func (s *Store) Last7DaysHeartbeats(ctx context.Context, userID uuid.UUID, now time.Time) ([]services.Heartbeat, error) {
	return s.HeartbeatsForStatsRange(ctx, userID, now, "last_7_days")
}

func (s *Store) HeartbeatsForStatsRange(ctx context.Context, userID uuid.UUID, now time.Time, rangeName string) ([]services.Heartbeat, error) {
	window, err := services.WindowForRange(now, rangeName)
	if err != nil {
		return nil, err
	}
	return s.HeartbeatsBetween(ctx, userID, float64(window.Start.Unix()), float64(window.End.Unix()))
}

func (s *Store) HeartbeatsForStatsRangeByUser(ctx context.Context, userIDs []uuid.UUID, now time.Time, rangeName string) (map[uuid.UUID][]services.Heartbeat, error) {
	window, err := services.WindowForRange(now, rangeName)
	if err != nil {
		return nil, err
	}
	return s.HeartbeatsBetweenByUser(ctx, userIDs, float64(window.Start.Unix()), float64(window.End.Unix()))
}

func (s *Store) HeartbeatsBetweenByUser(ctx context.Context, userIDs []uuid.UUID, start, end float64) (map[uuid.UUID][]services.Heartbeat, error) {
	result := map[uuid.UUID][]services.Heartbeat{}
	if len(userIDs) == 0 {
		return result, nil
	}
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		SELECT user_id, %s
		FROM heartbeats
		WHERE user_id = ANY($1) AND time >= $2 AND time < $3
		ORDER BY user_id, time ASC`, heartbeatSelectColumns), userIDs, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		userID, heartbeat, err := scanHeartbeatWithUserID(rows)
		if err != nil {
			return nil, err
		}
		result[userID] = append(result[userID], heartbeat)
	}
	return result, rows.Err()
}

func (s *Store) HeartbeatsForDay(ctx context.Context, userID uuid.UUID, day time.Time) ([]services.Heartbeat, error) {
	start := time.Date(day.UTC().Year(), day.UTC().Month(), day.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return s.HeartbeatsBetween(ctx, userID, float64(start.Unix()), float64(start.AddDate(0, 0, 1).Unix()))
}

func (s *Store) DeleteHeartbeatsByID(ctx context.Context, userID uuid.UUID, ids []uuid.UUID, start, end float64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM heartbeats
		WHERE user_id = $1 AND id = ANY($2) AND time >= $3 AND time < $4`, userID, ids, start, end)
	if err != nil {
		return 0, err
	}
	if err := s.MarkStatsStale(ctx, userID); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) PurgeHeartbeatsBefore(ctx context.Context, cutoffUnix float64) (int64, error) {
	rows, err := s.Pool.Query(ctx, `
		WITH deleted AS (
			DELETE FROM heartbeats
			WHERE time < $1
			RETURNING user_id
		)
		SELECT user_id, count(*) FROM deleted GROUP BY user_id`, cutoffUnix)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var total int64
	var userIDs []uuid.UUID
	for rows.Next() {
		var userID uuid.UUID
		var count int64
		if err := rows.Scan(&userID, &count); err != nil {
			return total, err
		}
		total += count
		userIDs = append(userIDs, userID)
	}
	if err := rows.Err(); err != nil {
		return total, err
	}
	if err := s.markStatsStaleForUsers(ctx, userIDs); err != nil {
		return total, err
	}
	return total, nil
}

func (s *Store) PurgeHeartbeatsByUserRetention(ctx context.Context, now time.Time) (int64, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rows, err := s.Pool.Query(ctx, `
		WITH deleted AS (
			DELETE FROM heartbeats h
			USING users u
			WHERE h.user_id = u.id
				AND u.heartbeat_retention_days > 0
				AND h.time < extract(epoch FROM ($1::timestamptz - make_interval(days => u.heartbeat_retention_days)))
			RETURNING h.user_id
		)
		SELECT user_id, count(*) FROM deleted GROUP BY user_id`, now.UTC())
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var total int64
	var userIDs []uuid.UUID
	for rows.Next() {
		var userID uuid.UUID
		var count int64
		if err := rows.Scan(&userID, &count); err != nil {
			return total, err
		}
		total += count
		userIDs = append(userIDs, userID)
	}
	if err := rows.Err(); err != nil {
		return total, err
	}
	if err := s.markStatsStaleForUsers(ctx, userIDs); err != nil {
		return total, err
	}
	return total, nil
}

func (s *Store) ListProjects(ctx context.Context, userID uuid.UUID) ([]Project, error) {
	rows, err := s.Pool.Query(ctx, `
			SELECT id, name, coalesce(color, ''), has_public_url, coalesce(badge, ''), first_heartbeat_at, last_heartbeat_at, created_at
			FROM projects
			WHERE user_id = $1
			ORDER BY last_heartbeat_at DESC NULLS LAST, name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []Project
	for rows.Next() {
		var project Project
		if err := rows.Scan(&project.ID, &project.Name, &project.Color, &project.HasPublicURL, &project.Badge, &project.FirstHeartbeatAt, &project.LastHeartbeatAt, &project.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func (s *Store) GetProject(ctx context.Context, userID uuid.UUID, name string) (Project, error) {
	row := s.Pool.QueryRow(ctx, `
			SELECT id, name, coalesce(color, ''), has_public_url, coalesce(badge, ''), first_heartbeat_at, last_heartbeat_at, created_at
			FROM projects
			WHERE user_id = $1 AND name = $2`, userID, name)
	var project Project
	err := row.Scan(&project.ID, &project.Name, &project.Color, &project.HasPublicURL, &project.Badge, &project.FirstHeartbeatAt, &project.LastHeartbeatAt, &project.CreatedAt)
	return project, err
}

func (s *Store) PublicProjectNames(ctx context.Context, userID uuid.UUID) (map[string]bool, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT name
		FROM projects
		WHERE user_id = $1 AND has_public_url = true`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names[name] = true
	}
	return names, rows.Err()
}

func (s *Store) ListMachineNames(ctx context.Context, userID uuid.UUID) ([]MachineName, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, name, coalesce(value, ''), coalesce(timezone, ''), last_seen_at, created_at
		FROM machine_names
		WHERE user_id = $1
		ORDER BY last_seen_at DESC NULLS LAST, name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var machines []MachineName
	for rows.Next() {
		var machine MachineName
		if err := rows.Scan(&machine.ID, &machine.Name, &machine.Value, &machine.Timezone, &machine.LastSeenAt, &machine.CreatedAt); err != nil {
			return nil, err
		}
		machines = append(machines, machine)
	}
	return machines, rows.Err()
}

func (s *Store) ListGoals(ctx context.Context, userID uuid.UUID) ([]Goal, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, title, coalesce(custom_title, ''), delta, seconds, languages, editors, projects, ignore_days,
			ignore_zero_days, improve_by_percent, is_enabled, is_inverse, is_snoozed, snooze_until, created_at, modified_at
		FROM goals
		WHERE user_id = $1
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var goals []Goal
	for rows.Next() {
		goal, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		goals = append(goals, goal)
	}
	return goals, rows.Err()
}

func (s *Store) GetGoal(ctx context.Context, userID, goalID uuid.UUID) (Goal, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, title, coalesce(custom_title, ''), delta, seconds, languages, editors, projects, ignore_days,
			ignore_zero_days, improve_by_percent, is_enabled, is_inverse, is_snoozed, snooze_until, created_at, modified_at
		FROM goals
		WHERE user_id = $1 AND id = $2`, userID, goalID)
	return scanGoal(row)
}

func (s *Store) CreateGoal(ctx context.Context, userID uuid.UUID, input GoalInput) (Goal, error) {
	if err := ValidateGoalInput(input); err != nil {
		return Goal{}, err
	}
	input = normalizeGoalInput(input)
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO goals (user_id, title, custom_title, delta, seconds, languages, editors, projects, ignore_days,
			ignore_zero_days, improve_by_percent, is_enabled, is_inverse, is_snoozed, snooze_until)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	RETURNING id, title, coalesce(custom_title, ''), delta, seconds, languages, editors, projects, ignore_days,
			ignore_zero_days, improve_by_percent, is_enabled, is_inverse, is_snoozed, snooze_until, created_at, modified_at`,
		userID, input.Title, nullEmpty(input.CustomTitle), input.Delta, input.Seconds, input.Languages, input.Editors,
		input.Projects, input.IgnoreDays, input.IgnoreZeroDays, input.ImproveByPercent, *input.IsEnabled, input.IsInverse,
		input.IsSnoozed, input.SnoozeUntil)
	return scanGoal(row)
}

func (s *Store) UpdateGoal(ctx context.Context, userID, goalID uuid.UUID, input GoalInput) (Goal, error) {
	if err := ValidateGoalInput(input); err != nil {
		return Goal{}, err
	}
	input = normalizeGoalInput(input)
	row := s.Pool.QueryRow(ctx, `
		UPDATE goals SET title = $3, custom_title = $4, delta = $5, seconds = $6, languages = $7, editors = $8,
			projects = $9, ignore_days = $10, ignore_zero_days = $11, improve_by_percent = $12,
			is_enabled = $13, is_inverse = $14, is_snoozed = $15, snooze_until = $16, modified_at = now()
		WHERE user_id = $1 AND id = $2
		RETURNING id, title, coalesce(custom_title, ''), delta, seconds, languages, editors, projects, ignore_days,
			ignore_zero_days, improve_by_percent, is_enabled, is_inverse, is_snoozed, snooze_until, created_at, modified_at`,
		userID, goalID, input.Title, nullEmpty(input.CustomTitle), input.Delta, input.Seconds, input.Languages, input.Editors,
		input.Projects, input.IgnoreDays, input.IgnoreZeroDays, input.ImproveByPercent, *input.IsEnabled, input.IsInverse,
		input.IsSnoozed, input.SnoozeUntil)
	return scanGoal(row)
}

func (s *Store) DeleteGoal(ctx context.Context, userID, goalID uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM goals WHERE user_id = $1 AND id = $2`, userID, goalID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) UpsertGoalEvaluation(ctx context.Context, userID uuid.UUID, goal Goal, progress services.GoalProgress, periodStart, periodEnd time.Time) (GoalEvaluation, error) {
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO goal_evaluations (user_id, goal_id, period_start, period_end, actual_seconds, target_seconds, percent, is_complete, evaluated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (goal_id, period_start, period_end) DO UPDATE SET
			actual_seconds = EXCLUDED.actual_seconds,
			target_seconds = EXCLUDED.target_seconds,
			percent = EXCLUDED.percent,
			is_complete = EXCLUDED.is_complete,
			evaluated_at = now()
		RETURNING id, user_id, goal_id, period_start, period_end, actual_seconds, target_seconds, percent, is_complete, evaluated_at`,
		userID, goal.ID, periodStart, periodEnd, progress.ActualSeconds, progress.TargetSeconds, progress.Percent, progress.IsComplete)
	return scanGoalEvaluation(row)
}

func (s *Store) CountGoalEvaluations(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM goal_evaluations WHERE user_id = $1`, userID).Scan(&count)
	return count, err
}

func (s *Store) HeartbeatsForAllTimeStats(ctx context.Context, userID uuid.UUID) ([]services.Heartbeat, error) {
	return s.heartbeatsForFullHistory(ctx, userID)
}

func (s *Store) HeartbeatsForExport(ctx context.Context, userID uuid.UUID) ([]services.Heartbeat, error) {
	return s.heartbeatsForFullHistory(ctx, userID)
}

func (s *Store) ForEachHeartbeatForExport(ctx context.Context, userID uuid.UUID, fn func(services.Heartbeat) error) error {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		SELECT %s
		FROM heartbeats
		WHERE user_id = $1
		ORDER BY time ASC`, heartbeatSelectColumns), userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		heartbeat, err := scanHeartbeat(rows)
		if err != nil {
			return err
		}
		if err := fn(heartbeat); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) ForEachHeartbeatForCustomRules(ctx context.Context, userID uuid.UUID, fn func(int, services.Heartbeat) error) error {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		SELECT %s
		FROM heartbeats
		WHERE user_id = $1
		ORDER BY time ASC`, heartbeatSelectColumns), userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		heartbeat, err := scanHeartbeat(rows)
		if err != nil {
			return err
		}
		if err := fn(index, heartbeat); err != nil {
			return err
		}
		index++
	}
	return rows.Err()
}

func (s *Store) heartbeatsForFullHistory(ctx context.Context, userID uuid.UUID) ([]services.Heartbeat, error) {
	return s.HeartbeatsBetween(ctx, userID, 0, float64(time.Now().Add(24*time.Hour).Unix()))
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`SELECT %s FROM users ORDER BY github_username ASC`, userColumns))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) ListExternalDurations(ctx context.Context, userID uuid.UUID) ([]ExternalDuration, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, external_id, provider, entity, type, coalesce(category, ''), start_time, end_time,
			coalesce(project, ''), coalesce(branch, ''), coalesce(language, ''), coalesce(meta, ''), created_at
		FROM external_durations
		WHERE user_id = $1
		ORDER BY start_time DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var durations []ExternalDuration
	for rows.Next() {
		duration, err := scanExternalDuration(rows)
		if err != nil {
			return nil, err
		}
		durations = append(durations, duration)
	}
	return durations, rows.Err()
}

func (s *Store) ListExternalDurationsForProject(ctx context.Context, userID uuid.UUID, project string) ([]ExternalDuration, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, external_id, provider, entity, type, coalesce(category, ''), start_time, end_time,
			coalesce(project, ''), coalesce(branch, ''), coalesce(language, ''), coalesce(meta, ''), created_at
		FROM external_durations
		WHERE user_id = $1 AND project = $2
		ORDER BY start_time DESC`, userID, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var durations []ExternalDuration
	for rows.Next() {
		duration, err := scanExternalDuration(rows)
		if err != nil {
			return nil, err
		}
		durations = append(durations, duration)
	}
	return durations, rows.Err()
}

func (s *Store) ExternalDurationsBetween(ctx context.Context, userID uuid.UUID, start, end time.Time) ([]ExternalDuration, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, external_id, provider, entity, type, coalesce(category, ''), start_time, end_time,
			coalesce(project, ''), coalesce(branch, ''), coalesce(language, ''), coalesce(meta, ''), created_at
		FROM external_durations
		WHERE user_id = $1 AND start_time < $3 AND end_time > $2
		ORDER BY start_time ASC`, userID, float64(start.Unix()), float64(end.Unix()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var durations []ExternalDuration
	for rows.Next() {
		duration, err := scanExternalDuration(rows)
		if err != nil {
			return nil, err
		}
		durations = append(durations, duration)
	}
	return durations, rows.Err()
}

func (s *Store) ExternalDurationsForProjectBetween(ctx context.Context, userID uuid.UUID, project string, start, end time.Time) ([]ExternalDuration, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, external_id, provider, entity, type, coalesce(category, ''), start_time, end_time,
			coalesce(project, ''), coalesce(branch, ''), coalesce(language, ''), coalesce(meta, ''), created_at
		FROM external_durations
		WHERE user_id = $1 AND project = $2 AND start_time < $4 AND end_time > $3
		ORDER BY start_time ASC`, userID, project, float64(start.Unix()), float64(end.Unix()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var durations []ExternalDuration
	for rows.Next() {
		duration, err := scanExternalDuration(rows)
		if err != nil {
			return nil, err
		}
		durations = append(durations, duration)
	}
	return durations, rows.Err()
}

func (s *Store) ExternalDurationsBetweenByUser(ctx context.Context, userIDs []uuid.UUID, start, end time.Time) (map[uuid.UUID][]ExternalDuration, error) {
	result := map[uuid.UUID][]ExternalDuration{}
	if len(userIDs) == 0 {
		return result, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT user_id, id, external_id, provider, entity, type, coalesce(category, ''), start_time, end_time,
			coalesce(project, ''), coalesce(branch, ''), coalesce(language, ''), coalesce(meta, ''), created_at
		FROM external_durations
		WHERE user_id = ANY($1) AND start_time < $3 AND end_time > $2
		ORDER BY user_id, start_time ASC`, userIDs, float64(start.Unix()), float64(end.Unix()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		userID, duration, err := scanExternalDurationWithUserID(rows)
		if err != nil {
			return nil, err
		}
		result[userID] = append(result[userID], duration)
	}
	return result, rows.Err()
}

func (s *Store) UpsertExternalDuration(ctx context.Context, userID uuid.UUID, input services.ExternalDuration) (ExternalDuration, error) {
	if err := services.ValidateExternalDuration(input); err != nil {
		return ExternalDuration{}, err
	}
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO external_durations (user_id, external_id, provider, entity, type, category, start_time, end_time, project, branch, language, meta)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (user_id, provider, external_id) DO UPDATE SET
			entity = EXCLUDED.entity,
			type = EXCLUDED.type,
			category = EXCLUDED.category,
			start_time = EXCLUDED.start_time,
			end_time = EXCLUDED.end_time,
			project = EXCLUDED.project,
			branch = EXCLUDED.branch,
			language = EXCLUDED.language,
			meta = EXCLUDED.meta
		RETURNING id, external_id, provider, entity, type, coalesce(category, ''), start_time, end_time,
			coalesce(project, ''), coalesce(branch, ''), coalesce(language, ''), coalesce(meta, ''), created_at`,
		userID, input.ExternalID, input.Provider, input.Entity, input.Type, nullEmpty(input.Category), input.StartTime, input.EndTime,
		nullEmpty(input.Project), nullEmpty(input.Branch), nullEmpty(input.Language), nullEmpty(input.Meta))
	duration, err := scanExternalDuration(row)
	if err != nil {
		return ExternalDuration{}, err
	}
	return duration, s.MarkStatsStale(ctx, userID)
}

func (s *Store) UpsertExternalDurations(ctx context.Context, userID uuid.UUID, inputs []services.ExternalDuration) ([]ExternalDuration, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	batch := &pgx.Batch{}
	for _, input := range inputs {
		if err := services.ValidateExternalDuration(input); err != nil {
			return nil, err
		}
		batch.Queue(`
			INSERT INTO external_durations (user_id, external_id, provider, entity, type, category, start_time, end_time, project, branch, language, meta)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT (user_id, provider, external_id) DO UPDATE SET
				entity = EXCLUDED.entity,
				type = EXCLUDED.type,
				category = EXCLUDED.category,
				start_time = EXCLUDED.start_time,
				end_time = EXCLUDED.end_time,
				project = EXCLUDED.project,
				branch = EXCLUDED.branch,
				language = EXCLUDED.language,
				meta = EXCLUDED.meta
			RETURNING id, external_id, provider, entity, type, coalesce(category, ''), start_time, end_time,
				coalesce(project, ''), coalesce(branch, ''), coalesce(language, ''), coalesce(meta, ''), created_at`,
			userID, input.ExternalID, input.Provider, input.Entity, input.Type, nullEmpty(input.Category), input.StartTime, input.EndTime,
			nullEmpty(input.Project), nullEmpty(input.Branch), nullEmpty(input.Language), nullEmpty(input.Meta))
	}
	results := s.Pool.SendBatch(ctx, batch)
	closed := false
	defer func() {
		if !closed {
			_ = results.Close()
		}
	}()
	durations := make([]ExternalDuration, 0, len(inputs))
	for range inputs {
		duration, err := scanExternalDuration(results.QueryRow())
		if err != nil {
			return nil, err
		}
		durations = append(durations, duration)
	}
	err := results.Close()
	closed = true
	if err != nil {
		return nil, err
	}
	return durations, s.MarkStatsStale(ctx, userID)
}

func (s *Store) DeleteExternalDurations(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM external_durations WHERE user_id = $1 AND id = ANY($2)`, userID, ids)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), s.MarkStatsStale(ctx, userID)
}

func (s *Store) ListLeaderboards(ctx context.Context, userID uuid.UUID) ([]Leaderboard, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT l.id, l.name, l.time_range, l.created_at, l.modified_at
		FROM leaderboards l
		JOIN leaderboard_members m ON m.leaderboard_id = l.id
		WHERE m.user_id = $1
		ORDER BY l.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var boards []Leaderboard
	for rows.Next() {
		board, err := scanLeaderboard(rows)
		if err != nil {
			return nil, err
		}
		boards = append(boards, board)
	}
	return boards, rows.Err()
}

func (s *Store) CreateLeaderboard(ctx context.Context, userID uuid.UUID, name, timeRange string) (Leaderboard, error) {
	if err := services.ValidateLeaderboardInput(name, timeRange); err != nil {
		return Leaderboard{}, err
	}
	name, timeRange = services.NormalizeLeaderboardInput(name, timeRange)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Leaderboard{}, err
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, `
		INSERT INTO leaderboards (user_id, name, time_range)
		VALUES ($1, $2, $3)
		RETURNING id, name, time_range, created_at, modified_at`, userID, name, timeRange)
	board, err := scanLeaderboard(row)
	if err != nil {
		return Leaderboard{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO leaderboard_members (leaderboard_id, user_id, role) VALUES ($1, $2, 'owner')`, board.ID, userID); err != nil {
		return Leaderboard{}, err
	}
	return board, tx.Commit(ctx)
}

func (s *Store) GetLeaderboardForUser(ctx context.Context, userID, boardID uuid.UUID) (Leaderboard, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT l.id, l.name, l.time_range, l.created_at, l.modified_at
		FROM leaderboards l
		JOIN leaderboard_members m ON m.leaderboard_id = l.id
		WHERE l.id = $1 AND m.user_id = $2`, boardID, userID)
	return scanLeaderboard(row)
}

func (s *Store) UpdateLeaderboard(ctx context.Context, userID, boardID uuid.UUID, name, timeRange string) (Leaderboard, error) {
	if err := services.ValidateLeaderboardInput(name, timeRange); err != nil {
		return Leaderboard{}, err
	}
	name, timeRange = services.NormalizeLeaderboardInput(name, timeRange)
	row := s.Pool.QueryRow(ctx, `
		UPDATE leaderboards SET name = $3, time_range = $4, modified_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING id, name, time_range, created_at, modified_at`, boardID, userID, name, timeRange)
	return scanLeaderboard(row)
}

func (s *Store) DeleteLeaderboard(ctx context.Context, userID, boardID uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM leaderboards WHERE id = $1 AND user_id = $2`, boardID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) AddLeaderboardMember(ctx context.Context, ownerID, boardID, memberID uuid.UUID) error {
	var exists bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM leaderboards WHERE id = $1 AND user_id = $2)`, boardID, ownerID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return pgx.ErrNoRows
	}
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO leaderboard_members (leaderboard_id, user_id, role)
		VALUES ($1, $2, 'member')
		ON CONFLICT (leaderboard_id, user_id) DO NOTHING`, boardID, memberID)
	return err
}

func (s *Store) RemoveLeaderboardMember(ctx context.Context, ownerID, boardID, memberID uuid.UUID) error {
	var exists bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM leaderboards WHERE id = $1 AND user_id = $2)`, boardID, ownerID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return pgx.ErrNoRows
	}
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM leaderboard_members
		WHERE leaderboard_id = $1 AND user_id = $2 AND role <> 'owner'`, boardID, memberID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) LeaderboardMemberRows(ctx context.Context, boardID uuid.UUID) ([]LeaderboardMember, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT u.id, u.github_username, coalesce(u.full_name, ''), m.role
		FROM leaderboard_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.leaderboard_id = $1
		ORDER BY CASE WHEN m.role = 'owner' THEN 0 ELSE 1 END, u.github_username ASC`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []LeaderboardMember
	for rows.Next() {
		var member LeaderboardMember
		if err := rows.Scan(&member.UserID, &member.Username, &member.FullName, &member.Role); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (s *Store) LeaderboardMembers(ctx context.Context, boardID uuid.UUID) ([]User, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		SELECT %s
		FROM leaderboard_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.leaderboard_id = $1
		ORDER BY u.github_username ASC`, userColumnsU), boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) ListDataDumps(ctx context.Context, userID uuid.UUID) ([]DataDump, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, type, status, percent_complete, coalesce(download_url, ''), is_processing, is_stuck, has_failed, expires_at, created_at
		FROM data_dumps WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dumps []DataDump
	for rows.Next() {
		dump, err := scanDataDump(rows)
		if err != nil {
			return nil, err
		}
		dumps = append(dumps, dump)
	}
	return dumps, rows.Err()
}

func (s *Store) CreateDataDump(ctx context.Context, userID uuid.UUID, dumpType string) (DataDump, error) {
	if err := ValidateDataDumpType(dumpType); err != nil {
		return DataDump{}, err
	}
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO data_dumps (user_id, type, status, percent_complete, is_processing)
		VALUES ($1, $2, 'Pending', 0, true)
		RETURNING id, type, status, percent_complete, coalesce(download_url, ''), is_processing, is_stuck, has_failed, expires_at, created_at`, userID, dumpType)
	return scanDataDump(row)
}

func ValidateDataDumpType(dumpType string) error {
	switch dumpType {
	case "daily", "heartbeats":
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrInvalidDataDumpType, dumpType)
	}
}

func (s *Store) CompleteDataDump(ctx context.Context, userID, dumpID uuid.UUID) (DataDump, error) {
	downloadURL := "/api/v1/users/current/data_dumps/" + dumpID.String() + "/download"
	return s.CompleteDataDumpWithURL(ctx, userID, dumpID, downloadURL)
}

func (s *Store) GetDataDump(ctx context.Context, userID, dumpID uuid.UUID) (DataDump, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, type, status, percent_complete, coalesce(download_url, ''), is_processing, is_stuck, has_failed, expires_at, created_at
		FROM data_dumps
		WHERE user_id = $1 AND id = $2`, userID, dumpID)
	return scanDataDump(row)
}

func (s *Store) CompleteDataDumpWithURL(ctx context.Context, userID, dumpID uuid.UUID, downloadURL string) (DataDump, error) {
	row := s.Pool.QueryRow(ctx, `
		UPDATE data_dumps
		SET status = 'Completed',
			percent_complete = 100,
			download_url = $3,
			is_processing = false,
			is_stuck = false,
			has_failed = false,
			expires_at = now() + interval '7 days'
		WHERE user_id = $1 AND id = $2
		RETURNING id, type, status, percent_complete, coalesce(download_url, ''), is_processing, is_stuck, has_failed, expires_at, created_at`,
		userID, dumpID, downloadURL)
	return scanDataDump(row)
}

func (s *Store) FailDataDump(ctx context.Context, userID, dumpID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE data_dumps
		SET status = 'Failed', is_processing = false, has_failed = true
		WHERE user_id = $1 AND id = $2`, userID, dumpID)
	return err
}

func (s *Store) ListCustomRules(ctx context.Context, userID uuid.UUID) ([]CustomRule, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, action, source, operation, source_value, priority, created_at, modified_at
		FROM custom_rules
		WHERE user_id = $1
		ORDER BY priority ASC, created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []CustomRule
	var ruleIDs []uuid.UUID
	for rows.Next() {
		rule, err := scanCustomRule(rows)
		if err != nil {
			return nil, err
		}
		ruleIDs = append(ruleIDs, rule.ID)
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	destinations, err := s.customRuleDestinationsByRule(ctx, ruleIDs)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		rules[i].Destinations = destinations[rules[i].ID]
	}
	return rules, nil
}

func (s *Store) ReplaceCustomRules(ctx context.Context, userID uuid.UUID, inputs []services.CustomRule) ([]CustomRule, error) {
	normalized := make([]services.CustomRule, len(inputs))
	for i, input := range inputs {
		input.Action = strings.ToLower(strings.TrimSpace(input.Action))
		input.Source = strings.TrimSpace(input.Source)
		input.Operation = services.NormalizeCustomRuleOperation(input.Operation)
		input.SourceValue = strings.TrimSpace(input.SourceValue)
		if input.Priority == 0 {
			input.Priority = i + 1
		}
		for j, destination := range input.Destinations {
			input.Destinations[j].Destination = strings.TrimSpace(destination.Destination)
			input.Destinations[j].DestinationValue = strings.TrimSpace(destination.DestinationValue)
		}
		normalized[i] = input
	}
	if err := services.ValidateCustomRules(normalized); err != nil {
		return nil, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM custom_rules WHERE user_id = $1`, userID); err != nil {
		return nil, err
	}
	ruleIDs := make([]uuid.UUID, len(normalized))
	if len(normalized) > 0 {
		ruleBatch := &pgx.Batch{}
		for _, input := range normalized {
			ruleBatch.Queue(`
				INSERT INTO custom_rules (user_id, action, source, operation, source_value, priority)
				VALUES ($1, $2, $3, $4, $5, $6)
				RETURNING id`, userID, input.Action, input.Source, input.Operation, input.SourceValue, input.Priority)
		}
		ruleResults := tx.SendBatch(ctx, ruleBatch)
		for i := range normalized {
			if err := ruleResults.QueryRow().Scan(&ruleIDs[i]); err != nil {
				_ = ruleResults.Close()
				return nil, err
			}
		}
		if err := ruleResults.Close(); err != nil {
			return nil, err
		}
	}
	destinationBatch := &pgx.Batch{}
	destinationCount := 0
	for i, input := range normalized {
		for _, destination := range input.Destinations {
			destinationBatch.Queue(`
				INSERT INTO custom_rule_destinations (rule_id, destination, destination_value)
				VALUES ($1, $2, $3)`, ruleIDs[i], destination.Destination, destination.DestinationValue)
			destinationCount++
		}
	}
	if destinationCount > 0 {
		destinationResults := tx.SendBatch(ctx, destinationBatch)
		for i := 0; i < destinationCount; i++ {
			if _, err := destinationResults.Exec(); err != nil {
				_ = destinationResults.Close()
				return nil, err
			}
		}
		if err := destinationResults.Close(); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.ListCustomRules(ctx, userID)
}

func (s *Store) DeleteCustomRule(ctx context.Context, userID, ruleID uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM custom_rules WHERE user_id = $1 AND id = $2`, userID, ruleID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) CountHeartbeats(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM heartbeats WHERE user_id = $1`, userID).Scan(&count)
	return count, err
}

func (s *Store) GetCustomRulesProgress(ctx context.Context, userID uuid.UUID) (CustomRulesProgress, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT status, percent_complete, total, changed, deleted, coalesce(error, ''), started_at, completed_at, modified_at
		FROM custom_rules_progress WHERE user_id = $1`, userID)
	return scanCustomRulesProgress(row)
}

func (s *Store) SetCustomRulesProgressQueued(ctx context.Context, userID uuid.UUID, total int) (CustomRulesProgress, error) {
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO custom_rules_progress (user_id, status, percent_complete, total, changed, deleted, error, started_at, completed_at, modified_at)
		VALUES ($1, 'Queued', 0, $2, 0, 0, NULL, NULL, NULL, now())
		ON CONFLICT (user_id) DO UPDATE SET
			status = 'Queued',
			percent_complete = 0,
			total = EXCLUDED.total,
			changed = 0,
			deleted = 0,
			error = NULL,
			started_at = NULL,
			completed_at = NULL,
			modified_at = now()
		RETURNING status, percent_complete, total, changed, deleted, coalesce(error, ''), started_at, completed_at, modified_at`,
		userID, total)
	return scanCustomRulesProgress(row)
}

func (s *Store) SetCustomRulesProgressProcessing(ctx context.Context, userID uuid.UUID, total int) (CustomRulesProgress, error) {
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO custom_rules_progress (user_id, status, percent_complete, total, changed, deleted, error, started_at, completed_at, modified_at)
		VALUES ($1, 'Processing', 0, $2, 0, 0, NULL, now(), NULL, now())
		ON CONFLICT (user_id) DO UPDATE SET
			status = 'Processing',
			percent_complete = 0,
			total = EXCLUDED.total,
			error = NULL,
			started_at = now(),
			completed_at = NULL,
			modified_at = now()
		WHERE custom_rules_progress.status <> 'Aborted'
		RETURNING status, percent_complete, total, changed, deleted, coalesce(error, ''), started_at, completed_at, modified_at`,
		userID, total)
	return s.scanCustomRulesProgressOrCurrent(ctx, userID, row)
}

func (s *Store) CompleteCustomRulesProgress(ctx context.Context, userID uuid.UUID, total, changed, deleted int) (CustomRulesProgress, error) {
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO custom_rules_progress (user_id, status, percent_complete, total, changed, deleted, error, started_at, completed_at, modified_at)
		VALUES ($1, 'Completed', 100, $2, $3, $4, NULL, now(), now(), now())
		ON CONFLICT (user_id) DO UPDATE SET
			status = 'Completed',
			percent_complete = 100,
			total = EXCLUDED.total,
			changed = EXCLUDED.changed,
			deleted = EXCLUDED.deleted,
			error = NULL,
			completed_at = now(),
			modified_at = now()
		WHERE custom_rules_progress.status <> 'Aborted'
		RETURNING status, percent_complete, total, changed, deleted, coalesce(error, ''), started_at, completed_at, modified_at`,
		userID, total, changed, deleted)
	return s.scanCustomRulesProgressOrCurrent(ctx, userID, row)
}

func (s *Store) FailCustomRulesProgress(ctx context.Context, userID uuid.UUID, message string) (CustomRulesProgress, error) {
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO custom_rules_progress (user_id, status, percent_complete, total, changed, deleted, error, started_at, completed_at, modified_at)
		VALUES ($1, 'Failed', 100, 0, 0, 0, $2, now(), now(), now())
		ON CONFLICT (user_id) DO UPDATE SET
			status = 'Failed',
			percent_complete = 100,
			error = EXCLUDED.error,
			completed_at = now(),
			modified_at = now()
		WHERE custom_rules_progress.status <> 'Aborted'
		RETURNING status, percent_complete, total, changed, deleted, coalesce(error, ''), started_at, completed_at, modified_at`,
		userID, message)
	return s.scanCustomRulesProgressOrCurrent(ctx, userID, row)
}

func (s *Store) ClearCustomRulesProgress(ctx context.Context, userID uuid.UUID) (CustomRulesProgress, error) {
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO custom_rules_progress (user_id, status, percent_complete, total, changed, deleted, error, started_at, completed_at, modified_at)
		VALUES ($1, 'Cleared', 0, 0, 0, 0, NULL, NULL, NULL, now())
		ON CONFLICT (user_id) DO UPDATE SET
			status = 'Cleared',
			percent_complete = 0,
			total = 0,
			changed = 0,
			deleted = 0,
			error = NULL,
			started_at = NULL,
			completed_at = NULL,
			modified_at = now()
		RETURNING status, percent_complete, total, changed, deleted, coalesce(error, ''), started_at, completed_at, modified_at`,
		userID)
	return scanCustomRulesProgress(row)
}

func (s *Store) AbortCustomRulesProgress(ctx context.Context, userID uuid.UUID) (CustomRulesProgress, error) {
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO custom_rules_progress (user_id, status, percent_complete, total, changed, deleted, error, started_at, completed_at, modified_at)
		VALUES ($1, 'Aborted', 100, 0, 0, 0, 'aborted', NULL, now(), now())
		ON CONFLICT (user_id) DO UPDATE SET
			status = 'Aborted',
			percent_complete = 100,
			error = 'aborted',
			completed_at = now(),
			modified_at = now()
		RETURNING status, percent_complete, total, changed, deleted, coalesce(error, ''), started_at, completed_at, modified_at`,
		userID)
	return scanCustomRulesProgress(row)
}

func (s *Store) scanCustomRulesProgressOrCurrent(ctx context.Context, userID uuid.UUID, row pgx.Row) (CustomRulesProgress, error) {
	progress, err := scanCustomRulesProgress(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return s.GetCustomRulesProgress(ctx, userID)
	}
	return progress, err
}

func customRulesProgressIsAborted(progress CustomRulesProgress) bool {
	return strings.EqualFold(progress.Status, "Aborted")
}

func (s *Store) ApplyCustomRulesToHeartbeats(ctx context.Context, userID uuid.UUID) (int, int, error) {
	rules, err := s.ListCustomRules(ctx, userID)
	if err != nil {
		return 0, 0, err
	}
	serviceRules := make([]services.CustomRule, 0, len(rules))
	for _, rule := range rules {
		serviceRules = append(serviceRules, services.CustomRule{
			Action:       rule.Action,
			Source:       rule.Source,
			Operation:    rule.Operation,
			SourceValue:  rule.SourceValue,
			Priority:     rule.Priority,
			Destinations: rule.Destinations,
		})
	}
	preparedRules, err := services.PrepareCustomRules(serviceRules)
	if err != nil {
		return 0, 0, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)
	changed := 0
	deleted := 0
	queuedWrites := 0
	batch := &pgx.Batch{}
	projectsToUpsert := map[string]projectHeartbeatRange{}
	err = s.ForEachHeartbeatForCustomRules(ctx, userID, func(i int, heartbeat services.Heartbeat) error {
		if i%100 == 0 {
			progress, err := s.GetCustomRulesProgress(ctx, userID)
			if err == nil && customRulesProgressIsAborted(progress) {
				return ErrCustomRulesAborted
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		heartbeatID, err := uuid.Parse(heartbeat.ID)
		if err != nil {
			return nil
		}
		updated, shouldDelete := services.ApplyPreparedCustomRules(heartbeat, preparedRules)
		if shouldDelete {
			batch.Queue(`DELETE FROM heartbeats WHERE user_id = $1 AND id = $2`, userID, heartbeatID)
			queuedWrites++
			deleted++
			if queuedWrites >= customRuleBatchWriteLimit {
				if err := flushCustomRuleBatch(ctx, tx, batch, queuedWrites); err != nil {
					return err
				}
				queuedWrites = 0
				batch = &pgx.Batch{}
			}
			return nil
		}
		if !heartbeatRewriteChanged(heartbeat, updated) {
			return nil
		}
		if updated.Entity != heartbeat.Entity {
			batch.Queue(`
				DELETE FROM heartbeats
				WHERE user_id = $1 AND id <> $2 AND entity = $3 AND time = $4`,
				userID, heartbeatID, updated.Entity, updated.Time)
			queuedWrites++
		}
		batch.Queue(`
			UPDATE heartbeats
			SET entity = $3,
				type = $4,
				category = $5,
				project = $6,
				branch = $7,
				language = $8,
				editor = $9,
				operating_system = $10
			WHERE user_id = $1 AND id = $2`,
			userID, heartbeatID, updated.Entity, updated.Type, nullEmpty(updated.Category), nullEmpty(updated.Project),
			nullEmpty(updated.Branch), nullEmpty(updated.Language), nullEmpty(updated.Editor), nullEmpty(updated.OperatingSystem))
		queuedWrites++
		if queuedWrites >= customRuleBatchWriteLimit {
			if err := flushCustomRuleBatch(ctx, tx, batch, queuedWrites); err != nil {
				return err
			}
			queuedWrites = 0
			batch = &pgx.Batch{}
		}
		if updated.Project != "" {
			trackProjectHeartbeatRange(projectsToUpsert, updated.Project, updated.Time)
		}
		changed++
		return nil
	})
	if err != nil {
		return changed, deleted, err
	}
	if queuedWrites > 0 {
		if err := flushCustomRuleBatch(ctx, tx, batch, queuedWrites); err != nil {
			return changed, deleted, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return changed, deleted, err
	}
	if err := s.upsertProjects(ctx, userID, projectsToUpsert); err != nil {
		return changed, deleted, err
	}
	if changed > 0 || deleted > 0 {
		if err := s.MarkStatsStale(ctx, userID); err != nil {
			return changed, deleted, err
		}
	}
	return changed, deleted, nil
}

func flushCustomRuleBatch(ctx context.Context, tx pgx.Tx, batch *pgx.Batch, queuedWrites int) error {
	results := tx.SendBatch(ctx, batch)
	for i := 0; i < queuedWrites; i++ {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return err
		}
	}
	return results.Close()
}

type projectHeartbeatRange struct {
	first float64
	last  float64
}

func trackProjectHeartbeatRange(projects map[string]projectHeartbeatRange, project string, heartbeatTime float64) {
	current, ok := projects[project]
	if !ok {
		projects[project] = projectHeartbeatRange{first: heartbeatTime, last: heartbeatTime}
		return
	}
	if heartbeatTime < current.first {
		current.first = heartbeatTime
	}
	if heartbeatTime > current.last {
		current.last = heartbeatTime
	}
	projects[project] = current
}

func heartbeatRewriteChanged(before, after services.Heartbeat) bool {
	return before.Entity != after.Entity ||
		before.Type != after.Type ||
		before.Category != after.Category ||
		before.Project != after.Project ||
		before.Branch != after.Branch ||
		before.Language != after.Language ||
		before.Editor != after.Editor ||
		before.OperatingSystem != after.OperatingSystem
}

func (s *Store) customRuleDestinations(ctx context.Context, ruleID uuid.UUID) ([]services.CustomRuleDestination, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT destination, destination_value
		FROM custom_rule_destinations
		WHERE rule_id = $1
		ORDER BY id ASC`, ruleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var destinations []services.CustomRuleDestination
	for rows.Next() {
		var destination services.CustomRuleDestination
		if err := rows.Scan(&destination.Destination, &destination.DestinationValue); err != nil {
			return nil, err
		}
		destinations = append(destinations, destination)
	}
	return destinations, rows.Err()
}

func (s *Store) customRuleDestinationsByRule(ctx context.Context, ruleIDs []uuid.UUID) (map[uuid.UUID][]services.CustomRuleDestination, error) {
	destinations := make(map[uuid.UUID][]services.CustomRuleDestination, len(ruleIDs))
	if len(ruleIDs) == 0 {
		return destinations, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT rule_id, destination, destination_value
		FROM custom_rule_destinations
		WHERE rule_id = ANY($1)
		ORDER BY rule_id ASC, id ASC`, ruleIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ruleID uuid.UUID
		var destination services.CustomRuleDestination
		if err := rows.Scan(&ruleID, &destination.Destination, &destination.DestinationValue); err != nil {
			return nil, err
		}
		destinations[ruleID] = append(destinations[ruleID], destination)
	}
	return destinations, rows.Err()
}

func (s *Store) UpsertStatsCache(ctx context.Context, userID uuid.UUID, rangeName string, stats services.Stats) error {
	payload, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO stats_cache (user_id, range, data, is_up_to_date, percent_calculated, computed_at)
		VALUES ($1, $2, $3, true, 100, now())
		ON CONFLICT (user_id, range) DO UPDATE SET
			data = EXCLUDED.data,
			is_up_to_date = true,
			percent_calculated = 100,
			computed_at = now()`, userID, rangeName, payload)
	return err
}

func (s *Store) UpsertProjectStatsCache(ctx context.Context, userID uuid.UUID, project, rangeName string, stats services.Stats) error {
	payload, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO project_stats_cache (user_id, project, range, data, is_up_to_date, percent_calculated, computed_at)
		VALUES ($1, $2, $3, $4, true, 100, now())
		ON CONFLICT (user_id, project, range) DO UPDATE SET
			data = EXCLUDED.data,
			is_up_to_date = true,
			percent_calculated = 100,
			computed_at = now()`, userID, project, rangeName, payload)
	return err
}

func (s *Store) StatsCache(ctx context.Context, userID uuid.UUID, rangeName string) (services.Stats, bool, error) {
	var data []byte
	var upToDate bool
	err := s.Pool.QueryRow(ctx, `SELECT data, is_up_to_date FROM stats_cache WHERE user_id = $1 AND range = $2`, userID, rangeName).Scan(&data, &upToDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return services.Stats{}, false, nil
	}
	if err != nil {
		return services.Stats{}, false, err
	}
	var stats services.Stats
	if err := json.Unmarshal(data, &stats); err != nil {
		return services.Stats{}, false, err
	}
	stats.IsUpToDate = upToDate
	if !upToDate {
		stats.PercentCalculated = int(math.Min(float64(stats.PercentCalculated), 99))
	}
	return stats, true, nil
}

func (s *Store) ProjectStatsCache(ctx context.Context, userID uuid.UUID, project, rangeName string) (services.Stats, bool, error) {
	var data []byte
	var upToDate bool
	err := s.Pool.QueryRow(ctx, `SELECT data, is_up_to_date FROM project_stats_cache WHERE user_id = $1 AND project = $2 AND range = $3`, userID, project, rangeName).Scan(&data, &upToDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return services.Stats{}, false, nil
	}
	if err != nil {
		return services.Stats{}, false, err
	}
	var stats services.Stats
	if err := json.Unmarshal(data, &stats); err != nil {
		return services.Stats{}, false, err
	}
	stats.IsUpToDate = upToDate
	if !upToDate {
		stats.PercentCalculated = int(math.Min(float64(stats.PercentCalculated), 99))
	}
	return stats, true, nil
}

func (s *Store) MarkStatsStale(ctx context.Context, userID uuid.UUID) error {
	return s.markStatsStaleForUsers(ctx, []uuid.UUID{userID})
}

func (s *Store) markStatsStaleForUsers(ctx context.Context, userIDs []uuid.UUID) error {
	if len(userIDs) == 0 {
		return nil
	}
	_, err := s.Pool.Exec(ctx, `UPDATE stats_cache SET is_up_to_date = false, percent_calculated = 0 WHERE user_id = ANY($1)`, userIDs)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `UPDATE project_stats_cache SET is_up_to_date = false, percent_calculated = 0 WHERE user_id = ANY($1)`, userIDs)
	return err
}

type oauthTokenWriter interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func insertOAuthTokenPair(ctx context.Context, writer oauthTokenWriter, user User, app OAuthApp, scopes []string, ttl time.Duration) (OAuthTokenResult, error) {
	return insertOAuthToken(ctx, writer, user, app, scopes, ttl, true)
}

func insertOAuthAccessToken(ctx context.Context, writer oauthTokenWriter, user User, app OAuthApp, scopes []string, ttl time.Duration) (OAuthTokenResult, error) {
	return insertOAuthToken(ctx, writer, user, app, scopes, ttl, false)
}

func insertOAuthToken(ctx context.Context, writer oauthTokenWriter, user User, app OAuthApp, scopes []string, ttl time.Duration, includeRefresh bool) (OAuthTokenResult, error) {
	if ttl <= 0 {
		ttl = time.Hour
	}
	var err error
	scopes, err = normalizeOAuthScopes(scopes)
	if err != nil {
		return OAuthTokenResult{}, err
	}
	access, accessFingerprint, accessHash, err := auth.GenerateOAuthBearerToken()
	if err != nil {
		return OAuthTokenResult{}, err
	}
	var refresh string
	var refreshFingerprint any
	var refreshHash any
	if includeRefresh {
		refreshToken, fingerprint, hash, err := auth.GenerateOAuthRefreshToken()
		if err != nil {
			return OAuthTokenResult{}, err
		}
		refresh = refreshToken
		refreshFingerprint = fingerprint
		refreshHash = hash
	}
	expiresAt := time.Now().Add(ttl).UTC()
	row := writer.QueryRow(ctx, `
		INSERT INTO oauth_tokens (app_id, user_id, access_token_hash, access_token_fingerprint, refresh_token_hash, refresh_token_fingerprint, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING expires_at`,
		app.ID, user.ID, accessHash, accessFingerprint, refreshHash, refreshFingerprint, scopes, expiresAt)
	if err := row.Scan(&expiresAt); err != nil {
		return OAuthTokenResult{}, err
	}
	if _, err := writer.Exec(ctx, `
		WITH ranked AS (
			SELECT id, row_number() OVER (ORDER BY created_at DESC, id DESC) AS rn
			FROM oauth_tokens
			WHERE app_id = $1 AND user_id = $2 AND revoked_at IS NULL
		)
		UPDATE oauth_tokens
		SET revoked_at = now()
		WHERE id IN (SELECT id FROM ranked WHERE rn > 8)`,
		app.ID, user.ID); err != nil {
		return OAuthTokenResult{}, err
	}
	return OAuthTokenResult{
		User:         user,
		App:          app,
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresAt:    expiresAt,
		ExpiresIn:    int(time.Until(expiresAt).Seconds()),
		Scopes:       scopes,
	}, nil
}

func (s *Store) upsertMachine(ctx context.Context, userID uuid.UUID, name string) (*uuid.UUID, error) {
	if name == "" {
		return nil, nil
	}
	return upsertMachineTx(ctx, s.Pool, userID, name)
}

type machineUpserter interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func upsertMachineTx(ctx context.Context, q machineUpserter, userID uuid.UUID, name string) (*uuid.UUID, error) {
	if name == "" {
		return nil, nil
	}
	var id uuid.UUID
	err := q.QueryRow(ctx, `
			INSERT INTO machine_names (user_id, name, last_seen_at)
			VALUES ($1, $2, now())
			ON CONFLICT (user_id, name) DO UPDATE SET last_seen_at = now()
		RETURNING id`, userID, name).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (s *Store) upsertProject(ctx context.Context, userID uuid.UUID, name string, heartbeatTime float64) error {
	return upsertProjectTx(ctx, s.Pool, userID, name, heartbeatTime)
}

func (s *Store) upsertProjects(ctx context.Context, userID uuid.UUID, projects map[string]projectHeartbeatRange) error {
	if len(projects) == 0 {
		return nil
	}
	names := make([]string, 0, len(projects))
	firstSeen := make([]time.Time, 0, len(projects))
	lastSeen := make([]time.Time, 0, len(projects))
	for name, heartbeatRange := range projects {
		names = append(names, name)
		firstSeen = append(firstSeen, time.Unix(int64(heartbeatRange.first), 0).UTC())
		lastSeen = append(lastSeen, time.Unix(int64(heartbeatRange.last), 0).UTC())
	}
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO projects (user_id, name, first_heartbeat_at, last_heartbeat_at)
		SELECT $1, input.name, input.first_seen, input.last_seen
		FROM unnest($2::text[], $3::timestamptz[], $4::timestamptz[]) AS input(name, first_seen, last_seen)
		ON CONFLICT (user_id, name) DO UPDATE SET
			last_heartbeat_at = GREATEST(projects.last_heartbeat_at, EXCLUDED.last_heartbeat_at),
			first_heartbeat_at = LEAST(projects.first_heartbeat_at, EXCLUDED.first_heartbeat_at)`,
		userID, names, firstSeen, lastSeen)
	return err
}

type projectUpserter interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func upsertProjectTx(ctx context.Context, q projectUpserter, userID uuid.UUID, name string, heartbeatTime float64) error {
	timestamp := time.Unix(int64(heartbeatTime), 0).UTC()
	_, err := q.Exec(ctx, `
			INSERT INTO projects (user_id, name, first_heartbeat_at, last_heartbeat_at)
			VALUES ($1, $2, $3, $3)
			ON CONFLICT (user_id, name) DO UPDATE SET
			last_heartbeat_at = GREATEST(projects.last_heartbeat_at, EXCLUDED.last_heartbeat_at),
			first_heartbeat_at = LEAST(projects.first_heartbeat_at, EXCLUDED.first_heartbeat_at)`,
		userID, name, timestamp)
	return err
}

func scanUser(row pgx.Row) (User, error) {
	var user User
	err := row.Scan(userScanDestinations(&user)...)
	return user, err
}

func userScanDestinations(user *User) []any {
	return []any{&user.ID, &user.GitHubID, &user.GitHubUsername, &user.Email, &user.FullName, &user.AvatarURL, &user.Timezone, &user.TimeoutMinutes, &user.WritesOnly, &user.IsHireable, &user.HasPublicProfile, &user.Country, &user.HeartbeatRetentionDays, &user.PublicUsername, &user.PublicDisplayName, &user.PublicGitHubLink, &user.PublicShowTotalTime, &user.PublicShowProjects, &user.PublicProjectVisibility, &user.PublicShowLanguages, &user.PublicShowEditors, &user.PublicShowMachines, &user.PublicShowOS, &user.PublicShowCategories, &user.PublicShowAI, &user.PublicShowSummaries, &user.PublicProfile}
}

type apiKeyScanner interface {
	Scan(dest ...any) error
}

func scanAPIKey(row apiKeyScanner) (APIKey, error) {
	var key APIKey
	err := row.Scan(&key.ID, &key.UserID, &key.Name, &key.Fingerprint, &key.Scopes, &key.LastUsedAt, &key.CreatedAt)
	return key, err
}

func scanGoal(row pgx.Row) (Goal, error) {
	var goal Goal
	err := row.Scan(&goal.ID, &goal.Title, &goal.CustomTitle, &goal.Delta, &goal.Seconds, &goal.Languages, &goal.Editors,
		&goal.Projects, &goal.IgnoreDays, &goal.IgnoreZeroDays, &goal.ImproveByPercent, &goal.IsEnabled, &goal.IsInverse,
		&goal.IsSnoozed, &goal.SnoozeUntil, &goal.CreatedAt, &goal.ModifiedAt)
	return goal, err
}

func scanGoalEvaluation(row pgx.Row) (GoalEvaluation, error) {
	var evaluation GoalEvaluation
	err := row.Scan(&evaluation.ID, &evaluation.UserID, &evaluation.GoalID, &evaluation.PeriodStart, &evaluation.PeriodEnd, &evaluation.ActualSeconds, &evaluation.TargetSeconds, &evaluation.Percent, &evaluation.IsComplete, &evaluation.EvaluatedAt)
	return evaluation, err
}

func scanExternalDuration(row pgx.Row) (ExternalDuration, error) {
	var duration ExternalDuration
	err := row.Scan(&duration.ID, &duration.ExternalID, &duration.Provider, &duration.Entity, &duration.Type, &duration.Category,
		&duration.StartTime, &duration.EndTime, &duration.Project, &duration.Branch, &duration.Language, &duration.Meta, &duration.CreatedAt)
	return duration, err
}

func scanExternalDurationWithUserID(row pgx.Row) (uuid.UUID, ExternalDuration, error) {
	var userID uuid.UUID
	var duration ExternalDuration
	err := row.Scan(&userID, &duration.ID, &duration.ExternalID, &duration.Provider, &duration.Entity, &duration.Type, &duration.Category,
		&duration.StartTime, &duration.EndTime, &duration.Project, &duration.Branch, &duration.Language, &duration.Meta, &duration.CreatedAt)
	return userID, duration, err
}

func scanLeaderboard(row pgx.Row) (Leaderboard, error) {
	var board Leaderboard
	err := row.Scan(&board.ID, &board.Name, &board.TimeRange, &board.CreatedAt, &board.ModifiedAt)
	return board, err
}

func scanDataDump(row pgx.Row) (DataDump, error) {
	var dump DataDump
	err := row.Scan(&dump.ID, &dump.Type, &dump.Status, &dump.PercentComplete, &dump.DownloadURL, &dump.IsProcessing, &dump.IsStuck, &dump.HasFailed, &dump.ExpiresAt, &dump.CreatedAt)
	return dump, err
}

func scanCustomRule(row pgx.Row) (CustomRule, error) {
	var rule CustomRule
	err := row.Scan(&rule.ID, &rule.Action, &rule.Source, &rule.Operation, &rule.SourceValue, &rule.Priority, &rule.CreatedAt, &rule.ModifiedAt)
	return rule, err
}

func scanCustomRulesProgress(row pgx.Row) (CustomRulesProgress, error) {
	var progress CustomRulesProgress
	err := row.Scan(&progress.Status, &progress.PercentComplete, &progress.Total, &progress.Changed, &progress.Deleted, &progress.Error, &progress.StartedAt, &progress.CompletedAt, &progress.ModifiedAt)
	return progress, err
}

func scanOAuthApp(row pgx.Row) (OAuthApp, error) {
	var app OAuthApp
	err := row.Scan(&app.ID, &app.UserID, &app.Name, &app.ClientID, &app.ClientSecretFingerprint, &app.RedirectURIs, &app.Scopes, &app.CreatedAt, &app.ModifiedAt)
	return app, err
}

func scanOAuthAppWithHash(row pgx.Row) (OAuthApp, string, error) {
	var app OAuthApp
	var hash string
	err := row.Scan(&app.ID, &app.UserID, &app.Name, &app.ClientID, &app.ClientSecretFingerprint, &app.RedirectURIs, &app.Scopes, &app.CreatedAt, &app.ModifiedAt, &hash)
	return app, hash, err
}

func scanShareToken(row pgx.Row) (ShareToken, error) {
	var token ShareToken
	err := row.Scan(&token.ID, &token.UserID, &token.Name, &token.Fingerprint, &token.LastUsedAt, &token.CreatedAt)
	return token, err
}

func scanAICostSetting(row pgx.Row) (AICostSetting, error) {
	var setting AICostSetting
	err := row.Scan(&setting.Agent, &setting.InputCostPerMillionCents, &setting.OutputCostPerMillionCents, &setting.ModifiedAt)
	return setting, err
}

const heartbeatSelectColumns = `id, entity, type, coalesce(category, ''), time, coalesce(project, ''), coalesce(branch, ''),
	coalesce(language, ''), coalesce(machine_name, ''), coalesce(plugin, ''), coalesce(plugin_version, ''),
	coalesce(editor, ''), coalesce(editor_version, ''), coalesce(operating_system, ''), coalesce(architecture, ''),
	coalesce(dependencies, ''), lines, line_number, cursor_pos, is_write, ai_line_changes,
	human_line_changes, coalesce(ai_session, ''), ai_input_tokens, ai_output_tokens, ai_prompt_length,
	coalesce(ai_subscription_plan, ''), coalesce(ai_model, ''), coalesce(ai_provider, ''), coalesce(ai_agent, ''),
	coalesce(ai_agent_version, ''), coalesce(ai_agent_complexity, ''), coalesce(commit_hash, ''),
	coalesce(metadata, '{}'::jsonb), coalesce(raw_payload, '{}'::jsonb)`

func scanHeartbeat(row pgx.Row) (services.Heartbeat, error) {
	return scanHeartbeatFields(row)
}

func scanHeartbeatWithUserID(row pgx.Row) (uuid.UUID, services.Heartbeat, error) {
	var userID uuid.UUID
	heartbeat, err := scanHeartbeatFields(row, &userID)
	return userID, heartbeat, err
}

func scanHeartbeatFields(row pgx.Row, leading ...any) (services.Heartbeat, error) {
	var heartbeat services.Heartbeat
	var id uuid.UUID
	var metadataRaw, rawPayloadRaw json.RawMessage
	destinations := append(leading, &id, &heartbeat.Entity, &heartbeat.Type, &heartbeat.Category, &heartbeat.Time,
		&heartbeat.Project, &heartbeat.Branch, &heartbeat.Language, &heartbeat.MachineName,
		&heartbeat.Plugin, &heartbeat.PluginVersion, &heartbeat.Editor, &heartbeat.EditorVersion,
		&heartbeat.OperatingSystem, &heartbeat.Architecture, &heartbeat.Dependencies, &heartbeat.Lines,
		&heartbeat.LineNumber, &heartbeat.CursorPosition, &heartbeat.IsWrite, &heartbeat.AILineChanges,
		&heartbeat.HumanLineChanges, &heartbeat.AISession, &heartbeat.AIInputTokens, &heartbeat.AIOutputTokens,
		&heartbeat.AIPromptLength, &heartbeat.AISubscriptionPlan, &heartbeat.AIModel, &heartbeat.AIProvider,
		&heartbeat.AIAgent, &heartbeat.AIAgentVersion, &heartbeat.AIAgentComplexity, &heartbeat.CommitHash,
		&metadataRaw, &rawPayloadRaw)
	err := row.Scan(destinations...)
	if err != nil {
		return heartbeat, err
	}
	if heartbeat.Metadata, err = jsonMapFromRaw(metadataRaw); err != nil {
		return heartbeat, err
	}
	if heartbeat.RawPayload, err = jsonMapFromRaw(rawPayloadRaw); err != nil {
		return heartbeat, err
	}
	heartbeat.ID = id.String()
	heartbeat.Revision = heartbeat.CommitHash
	return heartbeat, err
}

func nullEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func jsonMapArg(value map[string]any) string {
	if value == nil {
		value = map[string]any{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return `{}`
	}
	return string(data)
}

func jsonMapFromRaw(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	value := map[string]any{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if len(value) == 0 {
		return nil, nil
	}
	return value, nil
}

func randomHex(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func randomPrefixedToken(prefix string, n int) (string, error) {
	value, err := randomHex(n)
	if err != nil {
		return "", err
	}
	return prefix + value, nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func normalizeGoalInput(input GoalInput) GoalInput {
	if strings.TrimSpace(input.Title) == "" {
		input.Title = "Coding goal"
	}
	input.CustomTitle = strings.TrimSpace(input.CustomTitle)
	if input.Delta != "week" {
		input.Delta = "day"
	}
	if input.Seconds <= 0 {
		input.Seconds = 3600
	}
	input.Languages = normalizeStringList(input.Languages)
	input.Editors = normalizeStringList(input.Editors)
	input.Projects = normalizeStringList(input.Projects)
	input.IgnoreDays = normalizeWeekdayList(input.IgnoreDays)
	if input.IsEnabled == nil {
		enabled := true
		input.IsEnabled = &enabled
	}
	if !input.IsSnoozed {
		input.SnoozeUntil = nil
	}
	return input
}

func ValidateGoalInput(input GoalInput) error {
	delta := strings.TrimSpace(input.Delta)
	if delta != "" && delta != "day" && delta != "week" {
		return fmt.Errorf("delta must be day or week")
	}
	if input.Seconds < 0 {
		return fmt.Errorf("seconds cannot be negative")
	}
	if input.ImproveByPercent != nil && (*input.ImproveByPercent < 0 || math.IsNaN(*input.ImproveByPercent) || math.IsInf(*input.ImproveByPercent, 0)) {
		return fmt.Errorf("improve_by_percent must be a non-negative number")
	}
	for _, day := range input.IgnoreDays {
		day = strings.TrimSpace(day)
		if day == "" {
			continue
		}
		if !isWeekdayAlias(day) {
			return fmt.Errorf("ignore_days contains invalid weekday %q", day)
		}
	}
	return nil
}

func NormalizeUserSettings(input UserSettingsInput) UserSettingsInput {
	input.Timezone = strings.TrimSpace(input.Timezone)
	if input.Timezone == "" {
		input.Timezone = "UTC"
	}
	input.Country = strings.ToUpper(strings.TrimSpace(input.Country))
	input.PublicUsername = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(input.PublicUsername), "@"))
	input.PublicDisplayName = strings.TrimSpace(input.PublicDisplayName)
	input.PublicProjectVisibility = strings.TrimSpace(input.PublicProjectVisibility)
	if input.PublicProjectVisibility == "" {
		input.PublicProjectVisibility = "public_repos"
	}
	input.PublicProfile = NormalizePublicProfile(input.PublicProfile)
	return input
}

// ProfileLayouts are the public-page themes a user can choose from.
var ProfileLayouts = []string{"terminal", "spotlight", "rail"}

// PublicProfileDefaultRanges are the visitor-selectable stats windows that can
// be saved as a public profile's initial range.
var PublicProfileDefaultRanges = []string{"last_7_days", "last_30_days", "last_6_months", "last_year", "all_time"}

// ProfileVisibilityFields are the per-field privacy keys. The accepted values
// are "public" and "private" today; the set is open so org/team scopes can be
// added later without a migration.
var ProfileVisibilityFields = []string{"bio", "location", "website", "twitter", "linkedin", "mastodon", "pronouns", "company", "role", "hireable", "email"}

func NormalizePublicProfile(profile PublicProfile) PublicProfile {
	profile.Bio = strings.TrimSpace(profile.Bio)
	profile.Location = strings.TrimSpace(profile.Location)
	profile.WebsiteURL = strings.TrimSpace(profile.WebsiteURL)
	profile.TwitterUsername = strings.TrimPrefix(strings.TrimSpace(profile.TwitterUsername), "@")
	profile.LinkedInURL = strings.TrimSpace(profile.LinkedInURL)
	profile.MastodonURL = strings.TrimSpace(profile.MastodonURL)
	profile.Pronouns = strings.TrimSpace(profile.Pronouns)
	profile.Company = strings.TrimSpace(profile.Company)
	profile.Role = strings.TrimSpace(profile.Role)
	profile.Layout = strings.ToLower(strings.TrimSpace(profile.Layout))
	if !containsString(ProfileLayouts, profile.Layout) {
		profile.Layout = "terminal"
	}
	profile.DefaultRange = strings.ToLower(strings.TrimSpace(profile.DefaultRange))
	if profile.DefaultRange == "" {
		profile.DefaultRange = "last_7_days"
	}
	if len(profile.Visibility) == 0 {
		profile.Visibility = nil
	} else {
		cleaned := map[string]string{}
		for key, value := range profile.Visibility {
			key = strings.TrimSpace(key)
			value = strings.ToLower(strings.TrimSpace(value))
			// Drop "public" entries: absent means public, so we only persist
			// explicit overrides and keep the blob small.
			if key == "" || value == "" || value == "public" {
				continue
			}
			cleaned[key] = value
		}
		if len(cleaned) == 0 {
			cleaned = nil
		}
		profile.Visibility = cleaned
	}
	return profile
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func ValidateUserSettings(input UserSettingsInput) error {
	input = NormalizeUserSettings(input)
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return fmt.Errorf("timezone must be a valid IANA timezone")
	}
	if input.TimeoutMinutes < 0 {
		return fmt.Errorf("timeout_minutes cannot be negative")
	}
	if input.HeartbeatRetentionDays < 0 {
		return fmt.Errorf("heartbeat_retention_days cannot be negative")
	}
	if input.Country != "" && !validCountryCode(input.Country) {
		return fmt.Errorf("country must be a two-letter ISO country code")
	}
	if input.PublicUsername != "" && !validPublicUsername(input.PublicUsername) {
		return fmt.Errorf("public_username must be 3-39 URL-safe characters")
	}
	if input.PublicProjectVisibility != "none" && input.PublicProjectVisibility != "public_repos" && input.PublicProjectVisibility != "all" {
		return fmt.Errorf("public_project_visibility must be none, public_repos, or all")
	}
	if err := ValidatePublicProfile(input.PublicProfile); err != nil {
		return err
	}
	return nil
}

func ValidatePublicProfile(profile PublicProfile) error {
	profile = NormalizePublicProfile(profile)
	if len(profile.Bio) > 1000 {
		return fmt.Errorf("bio must be at most 1000 characters")
	}
	for label, value := range map[string]string{"location": profile.Location, "pronouns": profile.Pronouns, "company": profile.Company, "role": profile.Role} {
		if len(value) > 200 {
			return fmt.Errorf("%s must be at most 200 characters", label)
		}
	}
	for label, value := range map[string]string{"website_url": profile.WebsiteURL, "linkedin_url": profile.LinkedInURL, "mastodon_url": profile.MastodonURL} {
		if value == "" {
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("%s must be an absolute http or https URL", label)
		}
	}
	if profile.TwitterUsername != "" && !validTwitterUsername(profile.TwitterUsername) {
		return fmt.Errorf("twitter_username must be 1-15 letters, numbers, or underscores")
	}
	if !containsString(ProfileLayouts, profile.Layout) {
		return fmt.Errorf("layout must be one of terminal, spotlight, rail")
	}
	if !containsString(PublicProfileDefaultRanges, profile.DefaultRange) {
		return fmt.Errorf("default_range must be one of last_7_days, last_30_days, last_6_months, last_year, all_time")
	}
	for key, value := range profile.Visibility {
		if value != "private" {
			return fmt.Errorf("visibility for %q must be public or private", key)
		}
	}
	return nil
}

func validTwitterUsername(value string) bool {
	if len(value) < 1 || len(value) > 15 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}

func validPublicUsername(value string) bool {
	if len(value) < 3 || len(value) > 39 {
		return false
	}
	for i, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if !valid {
			return false
		}
		if (i == 0 || i == len(value)-1) && (r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func validCountryCode(value string) bool {
	if len(value) != 2 {
		return false
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func normalizeWeekdayList(values []string) []string {
	normalized := normalizeStringList(values)
	out := make([]string, 0, len(normalized))
	for _, value := range normalized {
		switch strings.ToLower(value) {
		case "sun", "sunday":
			out = append(out, "sunday")
		case "mon", "monday":
			out = append(out, "monday")
		case "tue", "tues", "tuesday":
			out = append(out, "tuesday")
		case "wed", "wednesday":
			out = append(out, "wednesday")
		case "thu", "thurs", "thursday":
			out = append(out, "thursday")
		case "fri", "friday":
			out = append(out, "friday")
		case "sat", "saturday":
			out = append(out, "saturday")
		}
	}
	return out
}

func isWeekdayAlias(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sun", "sunday", "mon", "monday", "tue", "tues", "tuesday", "wed", "wednesday", "thu", "thurs", "thursday", "fri", "friday", "sat", "saturday":
		return true
	default:
		return false
	}
}

func normalizeStringList(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func normalizeAPIKeyScopes(scopes []string) ([]string, error) {
	scopes = normalizeStringList(scopes)
	if len(scopes) == 0 {
		return DefaultAPIKeyScopes(), nil
	}
	for _, scope := range scopes {
		if !validOAuthScope(scope) {
			return nil, fmt.Errorf("%w: %s", ErrInvalidOAuthScope, scope)
		}
	}
	return scopes, nil
}

func ValidateOAuthAppInput(input OAuthAppInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("%w: OAuth app name is required", ErrInvalidResourceName)
	}
	redirectURIs := normalizeStringList(input.RedirectURIs)
	if len(redirectURIs) == 0 {
		return errors.New("at least one redirect URI is required")
	}
	for _, value := range redirectURIs {
		parsed, err := url.Parse(strings.TrimSpace(value))
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return errors.New("redirect URIs must be absolute URLs")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.New("redirect URIs must use http or https")
		}
	}
	_, err := normalizeOAuthScopes(input.Scopes)
	return err
}

func normalizeOAuthScopes(scopes []string) ([]string, error) {
	scopes = normalizeStringList(scopes)
	if len(scopes) == 0 {
		return []string{"read_stats", "read_summaries", "write_heartbeats"}, nil
	}
	for _, scope := range scopes {
		if !validOAuthScope(scope) {
			return nil, fmt.Errorf("%w: %s", ErrInvalidOAuthScope, scope)
		}
	}
	return scopes, nil
}

func validOAuthScope(scope string) bool {
	switch scope {
	case "read_summaries",
		"read_summaries.categories",
		"read_summaries.dependencies",
		"read_summaries.editors",
		"read_summaries.languages",
		"read_summaries.machines",
		"read_summaries.operating_systems",
		"read_summaries.projects",
		"read_stats",
		"read_stats.best_day",
		"read_stats.categories",
		"read_stats.dependencies",
		"read_stats.editors",
		"read_stats.languages",
		"read_stats.machines",
		"read_stats.operating_systems",
		"read_stats.projects",
		"read_goals",
		"read_private_leaderboards",
		"write_private_leaderboards",
		"read_heartbeats",
		"write_heartbeats",
		"email":
		return true
	default:
		return false
	}
}
