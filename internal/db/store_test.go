package db

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestUserScanDestinationsIncludesCountryAndRetention(t *testing.T) {
	var user User

	destinations := userScanDestinations(&user)

	if len(destinations) != 26 {
		t.Fatalf("expected 26 user scan destinations including privacy fields, got %d", len(destinations))
	}
	user.Country = "US"
	if user.Country != "US" {
		t.Fatal("expected country field to remain part of user model")
	}
	user.HeartbeatRetentionDays = 365
	if user.HeartbeatRetentionDays != 365 {
		t.Fatal("expected heartbeat retention field to remain part of user model")
	}
	user.PublicUsername = "keith"
	user.PublicProjectVisibility = "public_repos"
	user.PublicShowAI = true
	if user.PublicUsername != "keith" || user.PublicProjectVisibility != "public_repos" || !user.PublicShowAI {
		t.Fatal("expected public profile privacy fields to remain part of user model")
	}
}

func TestSQLCGetUserQueryIncludesCurrentProfileColumns(t *testing.T) {
	sourceBytes, err := os.ReadFile("queries/phase1.sql")
	if err != nil {
		t.Fatalf("could not read sqlc query file: %v", err)
	}
	source := string(sourceBytes)
	for _, column := range []string{"has_public_profile", "country", "heartbeat_retention_days", "public_username", "public_project_visibility", "public_show_ai"} {
		if !strings.Contains(source, column) {
			t.Fatalf("sqlc GetUser query must include current profile column %q", column)
		}
	}
}

func TestDefaultAPIKeyScopesIncludeLocalAccountScopes(t *testing.T) {
	scopes := map[string]bool{}
	for _, scope := range DefaultAPIKeyScopes() {
		scopes[scope] = true
	}
	for _, scope := range []string{"read_stats", "read_summaries", "read_heartbeats", "write_heartbeats", "read_goals", "read_private_leaderboards", "write_private_leaderboards", "email"} {
		if !scopes[scope] {
			t.Fatalf("expected default API key scopes to include %s", scope)
		}
	}
}

func TestNormalizeAPIKeyScopesDefaultsToFullLocalScopes(t *testing.T) {
	scopes, err := normalizeAPIKeyScopes(nil)
	if err != nil {
		t.Fatalf("normalizeAPIKeyScopes returned error: %v", err)
	}

	if len(scopes) != len(DefaultAPIKeyScopes()) {
		t.Fatalf("expected %d default scopes, got %d", len(DefaultAPIKeyScopes()), len(scopes))
	}
}

func TestNormalizeAPIKeyScopesRejectsUnknownScope(t *testing.T) {
	if _, err := normalizeAPIKeyScopes([]string{"read_stats", "delete_everything"}); !errors.Is(err, ErrInvalidOAuthScope) {
		t.Fatalf("expected ErrInvalidOAuthScope, got %v", err)
	}
}

func TestValidateAPIKeyNameRejectsBlankName(t *testing.T) {
	if err := ValidateAPIKeyName("   "); err == nil {
		t.Fatal("expected blank API key name to be rejected")
	}
}

func TestValidateAPIKeyNameAllowsNamedKey(t *testing.T) {
	if err := ValidateAPIKeyName("Local WakaTime"); err != nil {
		t.Fatalf("expected named API key to be valid, got %v", err)
	}
}

func TestValidateShareTokenNameRejectsBlankName(t *testing.T) {
	if err := ValidateShareTokenName("   "); err == nil {
		t.Fatal("expected blank share token name to be rejected")
	}
}

func TestValidateShareTokenNameAllowsNamedToken(t *testing.T) {
	if err := ValidateShareTokenName("Portfolio embed"); err != nil {
		t.Fatalf("expected named share token to be valid, got %v", err)
	}
}

func TestValidateOAuthAppInputRejectsBlankName(t *testing.T) {
	err := ValidateOAuthAppInput(OAuthAppInput{Name: "   ", RedirectURIs: []string{"https://example.com/callback"}})
	if err == nil {
		t.Fatal("expected blank OAuth app name to be rejected")
	}
	if !errors.Is(err, ErrInvalidResourceName) {
		t.Fatalf("expected ErrInvalidResourceName, got %v", err)
	}
}

func TestProjectModelExposesPublicMetadata(t *testing.T) {
	project := Project{HasPublicURL: true, Badge: "https://example.com/badges/project.svg"}
	if !project.HasPublicURL {
		t.Fatal("expected project public URL flag to be exposed")
	}
	if project.Badge == "" {
		t.Fatal("expected project badge URL to be exposed")
	}
}

func TestNormalizeOAuthScopesDefaultsToUsefulLocalScopes(t *testing.T) {
	scopes, err := normalizeOAuthScopes(nil)
	if err != nil {
		t.Fatalf("normalizeOAuthScopes returned error: %v", err)
	}

	expected := []string{"read_stats", "read_summaries", "write_heartbeats"}
	if len(scopes) != len(expected) {
		t.Fatalf("expected %d scopes, got %d", len(expected), len(scopes))
	}
	for i, scope := range expected {
		if scopes[i] != scope {
			t.Fatalf("scope %d: expected %q, got %q", i, scope, scopes[i])
		}
	}
}

func TestNormalizeOAuthScopesRejectsUnknownScope(t *testing.T) {
	if _, err := normalizeOAuthScopes([]string{"read_stats", "delete_everything"}); !errors.Is(err, ErrInvalidOAuthScope) {
		t.Fatalf("expected ErrInvalidOAuthScope, got %v", err)
	}
}

func TestValidateAICostSettingsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		settings []AICostSetting
	}{
		{name: "too many", settings: make([]AICostSetting, 51)},
		{name: "missing agent", settings: []AICostSetting{{Agent: "", InputCostPerMillionCents: 1, OutputCostPerMillionCents: 1}}},
		{name: "negative input", settings: []AICostSetting{{Agent: "Codex", InputCostPerMillionCents: -1, OutputCostPerMillionCents: 1}}},
		{name: "negative output", settings: []AICostSetting{{Agent: "Codex", InputCostPerMillionCents: 1, OutputCostPerMillionCents: -1}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateAICostSettings(tt.settings); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateAICostSettingsAllowsNonnegativeCosts(t *testing.T) {
	err := ValidateAICostSettings([]AICostSetting{{Agent: "Codex", InputCostPerMillionCents: 0, OutputCostPerMillionCents: 400}})
	if err != nil {
		t.Fatalf("expected valid AI cost settings, got %v", err)
	}
}

func TestValidateOAuthAppInputRejectsInvalidRedirectURIs(t *testing.T) {
	tests := []struct {
		name         string
		redirectURIs []string
	}{
		{name: "missing", redirectURIs: nil},
		{name: "relative", redirectURIs: []string{"/oauth/callback"}},
		{name: "unsupported scheme", redirectURIs: []string{"file:///tmp/callback"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateOAuthAppInput(OAuthAppInput{Name: "App", RedirectURIs: tt.redirectURIs}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateOAuthAppInputAllowsHTTPRedirectURIs(t *testing.T) {
	if err := ValidateOAuthAppInput(OAuthAppInput{Name: "App", RedirectURIs: []string{"http://localhost:8080/callback", "https://example.com/oauth/callback"}}); err != nil {
		t.Fatalf("expected HTTP redirect URIs to be valid, got %v", err)
	}
}

func TestCreateOAuthAppValidatesInputBeforeInsert(t *testing.T) {
	sourceBytes, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("could not read store.go: %v", err)
	}
	body := functionSource(string(sourceBytes), "CreateOAuthApp")
	if !strings.Contains(body, "ValidateOAuthAppInput") {
		t.Fatal("CreateOAuthApp must validate OAuth app input before insert")
	}
}

func TestCreateAPIKeyValidatesNameBeforeInsert(t *testing.T) {
	sourceBytes, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("could not read store.go: %v", err)
	}
	body := functionSource(string(sourceBytes), "CreateAPIKeyWithScopes")
	if !strings.Contains(body, "ValidateAPIKeyName") {
		t.Fatal("CreateAPIKeyWithScopes must validate name before insert")
	}
}

func TestCreateShareTokenValidatesNameBeforeInsert(t *testing.T) {
	sourceBytes, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("could not read store.go: %v", err)
	}
	body := functionSource(string(sourceBytes), "CreateShareToken")
	if !strings.Contains(body, "ValidateShareTokenName") {
		t.Fatal("CreateShareToken must validate name before insert")
	}
}

func TestOAuthRefreshTokenQueriesRequireUnexpiredTokens(t *testing.T) {
	sourceBytes, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("could not read store.go: %v", err)
	}
	source := string(sourceBytes)
	for _, functionName := range []string{"OAuthRefreshTokenUserID", "RefreshOAuthToken"} {
		body := functionSource(source, functionName)
		if !strings.Contains(body, "t.expires_at > now()") {
			t.Fatalf("%s must reject expired refresh tokens", functionName)
		}
	}
}

func TestRevokeOAuthTokenScopesLookupToAuthenticatedClient(t *testing.T) {
	sourceBytes, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("could not read store.go: %v", err)
	}
	body := functionSource(string(sourceBytes), "RevokeOAuthToken")
	for _, want := range []string{
		"JOIN oauth_apps a ON a.id = t.app_id",
		"a.client_id = $2",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("RevokeOAuthToken must scope token lookup to the authenticated OAuth client; missing %q", want)
		}
	}
}

func TestOAuthImplicitTokensDoNotCreateRefreshTokens(t *testing.T) {
	sourceBytes, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("could not read store.go: %v", err)
	}
	body := functionSource(string(sourceBytes), "CreateOAuthImplicitToken")
	if strings.Contains(body, "insertOAuthTokenPair") {
		t.Fatal("implicit OAuth tokens must use an access-token-only insert path")
	}
	if !strings.Contains(body, "insertOAuthAccessToken") {
		t.Fatal("implicit OAuth tokens must call insertOAuthAccessToken")
	}
}

func TestOAuthTokenMigrationAllowsRefreshTokenNulls(t *testing.T) {
	migrationBytes, err := os.ReadFile("migrations/0010_oauth_implicit_refresh_nullable.sql")
	if err != nil {
		t.Fatalf("could not read OAuth nullable refresh migration: %v", err)
	}
	migration := string(migrationBytes)
	for _, statement := range []string{
		"ALTER COLUMN refresh_token_hash DROP NOT NULL",
		"ALTER COLUMN refresh_token_fingerprint DROP NOT NULL",
	} {
		if !strings.Contains(migration, statement) {
			t.Fatalf("expected migration to include %q", statement)
		}
	}
}

func TestDataDumpTypeMigrationIsIdempotent(t *testing.T) {
	migrationBytes, err := os.ReadFile("migrations/0011_data_dump_type_check.sql")
	if err != nil {
		t.Fatalf("could not read data dump type migration: %v", err)
	}
	migration := string(migrationBytes)
	for _, statement := range []string{
		"IF NOT EXISTS",
		"data_dumps_type_check",
		"CHECK (type IN ('daily', 'heartbeats'))",
	} {
		if !strings.Contains(migration, statement) {
			t.Fatalf("expected migration to include %q", statement)
		}
	}
}

func TestDataDumpBaseMigrationDefaultsToQueuedState(t *testing.T) {
	migrationBytes, err := os.ReadFile("migrations/0001_phase1.sql")
	if err != nil {
		t.Fatalf("could not read base migration: %v", err)
	}
	migration := string(migrationBytes)
	for _, statement := range []string{
		"status text NOT NULL DEFAULT 'Pending'",
		"percent_complete double precision NOT NULL DEFAULT 0",
		"is_processing boolean NOT NULL DEFAULT true",
	} {
		if !strings.Contains(migration, statement) {
			t.Fatalf("expected data_dumps base migration to include %q", statement)
		}
	}
}

func TestDataDumpQueuedDefaultsMigrationIsIdempotent(t *testing.T) {
	migrationBytes, err := os.ReadFile("migrations/0022_data_dump_queued_defaults.sql")
	if err != nil {
		t.Fatalf("could not read data dump queued defaults migration: %v", err)
	}
	migration := string(migrationBytes)
	for _, statement := range []string{
		"ALTER COLUMN status SET DEFAULT 'Pending'",
		"ALTER COLUMN percent_complete SET DEFAULT 0",
		"ALTER COLUMN is_processing SET DEFAULT true",
	} {
		if !strings.Contains(migration, statement) {
			t.Fatalf("expected data dump queued defaults migration to include %q", statement)
		}
	}
}

func TestCreateDataDumpRejectsUnsupportedTypeBeforeInsert(t *testing.T) {
	sourceBytes, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("could not read store source: %v", err)
	}
	body := functionSource(string(sourceBytes), "CreateDataDump")
	if !strings.Contains(body, "ValidateDataDumpType") {
		t.Fatal("CreateDataDump must validate dump type before inserting")
	}
	if strings.Contains(body, `if dumpType != "daily"`) {
		t.Fatal("CreateDataDump must not silently coerce unsupported dump types to heartbeats")
	}
}

func TestGoalInputChecksMigrationIsIdempotent(t *testing.T) {
	migrationBytes, err := os.ReadFile("migrations/0012_goal_input_checks.sql")
	if err != nil {
		t.Fatalf("could not read goal input checks migration: %v", err)
	}
	migration := string(migrationBytes)
	for _, statement := range []string{
		"IF NOT EXISTS",
		"goals_delta_check",
		"goals_seconds_check",
		"goals_improve_by_percent_check",
		"CHECK (delta IN ('day', 'week'))",
		"CHECK (seconds >= 0)",
		"CHECK (improve_by_percent IS NULL OR improve_by_percent >= 0)",
	} {
		if !strings.Contains(migration, statement) {
			t.Fatalf("expected migration to include %q", statement)
		}
	}
}

func TestLeaderboardTimeRangeMigrationIsIdempotent(t *testing.T) {
	migrationBytes, err := os.ReadFile("migrations/0013_leaderboard_time_range_check.sql")
	if err != nil {
		t.Fatalf("could not read leaderboard time range migration: %v", err)
	}
	migration := string(migrationBytes)
	for _, statement := range []string{
		"IF NOT EXISTS",
		"leaderboards_time_range_check",
		"last_7_days|last_30_days|last_6_months|last_year|all_time",
		"[0-9]{4}",
		"[0-9]{4}-[0-9]{2}",
	} {
		if !strings.Contains(migration, statement) {
			t.Fatalf("expected migration to include %q", statement)
		}
	}
}

func TestUserSettingsChecksMigrationIsIdempotent(t *testing.T) {
	migrationBytes, err := os.ReadFile("migrations/0014_user_settings_checks.sql")
	if err != nil {
		t.Fatalf("could not read user settings checks migration: %v", err)
	}
	migration := string(migrationBytes)
	for _, statement := range []string{
		"IF NOT EXISTS",
		"users_timeout_minutes_check",
		"users_heartbeat_retention_days_check",
		"users_country_check",
		"CHECK (timeout_minutes >= 0)",
		"CHECK (heartbeat_retention_days >= 0)",
		"CHECK (country IS NULL OR country ~ '^[A-Z]{2}$')",
	} {
		if !strings.Contains(migration, statement) {
			t.Fatalf("expected migration to include %q", statement)
		}
	}
}

func TestOAuthAppRedirectURIsMigrationIsIdempotent(t *testing.T) {
	migrationBytes, err := os.ReadFile("migrations/0015_oauth_app_redirect_uris_check.sql")
	if err != nil {
		t.Fatalf("could not read OAuth app redirect URIs migration: %v", err)
	}
	migration := string(migrationBytes)
	for _, statement := range []string{
		"IF NOT EXISTS",
		"oauth_apps_redirect_uris_check",
		"CHECK (cardinality(redirect_uris) > 0)",
	} {
		if !strings.Contains(migration, statement) {
			t.Fatalf("expected migration to include %q", statement)
		}
	}
}

func TestUpsertExternalDurationValidatesInputBeforeInsert(t *testing.T) {
	sourceBytes, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("could not read store.go: %v", err)
	}
	body := functionSource(string(sourceBytes), "UpsertExternalDuration")
	if !strings.Contains(body, "services.ValidateExternalDuration") {
		t.Fatal("UpsertExternalDuration must validate external duration input before insert")
	}
}

func TestExternalDurationChecksMigrationIsIdempotent(t *testing.T) {
	migrationBytes, err := os.ReadFile("migrations/0016_external_duration_checks.sql")
	if err != nil {
		t.Fatalf("could not read external duration checks migration: %v", err)
	}
	migration := string(migrationBytes)
	for _, statement := range []string{
		"IF NOT EXISTS",
		"external_durations_required_text_check",
		"external_durations_time_check",
		"btrim(external_id) <> ''",
		"btrim(provider) <> ''",
		"btrim(entity) <> ''",
		"btrim(type) <> ''",
		"CHECK (start_time > 0 AND end_time > start_time)",
	} {
		if !strings.Contains(migration, statement) {
			t.Fatalf("expected migration to include %q", statement)
		}
	}
}

func TestReplaceCustomRulesValidatesInputBeforeInsert(t *testing.T) {
	sourceBytes, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("could not read store.go: %v", err)
	}
	body := functionSource(string(sourceBytes), "ReplaceCustomRules")
	if !strings.Contains(body, "services.ValidateCustomRules") {
		t.Fatal("ReplaceCustomRules must validate custom rule input before replacing rows")
	}
}

func TestCustomRuleChecksMigrationIsIdempotent(t *testing.T) {
	migrationBytes, err := os.ReadFile("migrations/0017_custom_rule_checks.sql")
	if err != nil {
		t.Fatalf("could not read custom rule checks migration: %v", err)
	}
	migration := string(migrationBytes)
	for _, statement := range []string{
		"IF NOT EXISTS",
		"custom_rules_action_check",
		"custom_rules_operation_check",
		"custom_rules_required_text_check",
		"custom_rule_destinations_required_text_check",
		"CHECK (action IN ('change', 'delete'))",
		"CHECK (operation IN ('equals', 'contains', 'starts_with', 'ends_with', 'regex', 'matches'))",
		"CHECK (priority > 0)",
		"btrim(destination) <> ''",
		"btrim(destination_value) <> ''",
	} {
		if !strings.Contains(migration, statement) {
			t.Fatalf("expected migration to include %q", statement)
		}
	}
}

func TestCustomRuleFieldChecksMigrationIsIdempotent(t *testing.T) {
	migrationBytes, err := os.ReadFile("migrations/0021_custom_rule_field_checks.sql")
	if err != nil {
		t.Fatalf("could not read custom rule field checks migration: %v", err)
	}
	migration := string(migrationBytes)
	for _, statement := range []string{
		"IF NOT EXISTS",
		"custom_rules_source_check",
		"custom_rule_destinations_destination_check",
		"lower(btrim(source)) IN ('entity', 'type', 'category', 'project', 'branch', 'language', 'editor', 'operating_system')",
		"lower(btrim(destination)) IN ('entity', 'type', 'category', 'project', 'branch', 'language', 'editor', 'operating_system')",
	} {
		if !strings.Contains(migration, statement) {
			t.Fatalf("expected migration to include %q", statement)
		}
	}
}

func TestReplaceAICostSettingsValidatesInputBeforeReplace(t *testing.T) {
	sourceBytes, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("could not read store.go: %v", err)
	}
	body := functionSource(string(sourceBytes), "ReplaceAICostSettings")
	if !strings.Contains(body, "ValidateAICostSettings") {
		t.Fatal("ReplaceAICostSettings must validate input before replacing rows")
	}
}

func TestAICostSettingChecksMigrationIsIdempotent(t *testing.T) {
	migrationBytes, err := os.ReadFile("migrations/0018_ai_cost_setting_checks.sql")
	if err != nil {
		t.Fatalf("could not read AI cost setting checks migration: %v", err)
	}
	migration := string(migrationBytes)
	for _, statement := range []string{
		"IF NOT EXISTS",
		"ai_cost_settings_agent_check",
		"ai_cost_settings_costs_check",
		"btrim(agent) <> ''",
		"CHECK (input_cost_per_million_cents >= 0 AND output_cost_per_million_cents >= 0)",
	} {
		if !strings.Contains(migration, statement) {
			t.Fatalf("expected migration to include %q", statement)
		}
	}
}

func TestNamedAccountResourceChecksMigrationIsIdempotent(t *testing.T) {
	migrationBytes, err := os.ReadFile("migrations/0019_named_account_resource_checks.sql")
	if err != nil {
		t.Fatalf("could not read named account resource checks migration: %v", err)
	}
	migration := string(migrationBytes)
	for _, statement := range []string{
		"IF NOT EXISTS",
		"api_keys_name_check",
		"share_tokens_name_check",
		"CHECK (btrim(name) <> '')",
	} {
		if !strings.Contains(migration, statement) {
			t.Fatalf("expected migration to include %q", statement)
		}
	}
}

func TestOAuthAndLeaderboardNameChecksMigrationIsIdempotent(t *testing.T) {
	migrationBytes, err := os.ReadFile("migrations/0020_oauth_and_leaderboard_name_checks.sql")
	if err != nil {
		t.Fatalf("could not read migration: %v", err)
	}
	migration := string(migrationBytes)
	for _, statement := range []string{
		"IF NOT EXISTS",
		"oauth_apps_name_check",
		"leaderboards_name_check",
		"CHECK (btrim(name) <> '')",
	} {
		if !strings.Contains(migration, statement) {
			t.Fatalf("expected migration to contain %q", statement)
		}
	}
}

func functionSource(source, name string) string {
	start := strings.Index(source, "func (s *Store) "+name)
	if start < 0 {
		return ""
	}
	next := strings.Index(source[start+1:], "\nfunc ")
	if next < 0 {
		return source[start:]
	}
	return source[start : start+1+next]
}

func TestNormalizeGoalInputDefaultsEnabledWhenOmitted(t *testing.T) {
	input := normalizeGoalInput(GoalInput{Title: "Daily", Seconds: 60})

	if input.IsEnabled == nil {
		t.Fatal("expected is_enabled pointer to be set")
	}
	if !*input.IsEnabled {
		t.Fatal("expected omitted is_enabled to default to true")
	}
}

func TestNormalizeGoalInputPreservesExplicitDisabled(t *testing.T) {
	disabled := false
	input := normalizeGoalInput(GoalInput{Title: "Paused", Seconds: 60, IsEnabled: &disabled})

	if input.IsEnabled == nil {
		t.Fatal("expected is_enabled pointer to be set")
	}
	if *input.IsEnabled {
		t.Fatal("expected explicit is_enabled=false to be preserved")
	}
}

func TestValidateGoalInputRejectsInvalidFields(t *testing.T) {
	negativeImprove := -1.0
	tests := []struct {
		name  string
		input GoalInput
	}{
		{name: "unsupported delta", input: GoalInput{Delta: "month", Seconds: 60}},
		{name: "negative seconds", input: GoalInput{Delta: "day", Seconds: -1}},
		{name: "invalid weekday", input: GoalInput{Delta: "week", Seconds: 60, IgnoreDays: []string{"funday"}}},
		{name: "negative improve percent", input: GoalInput{Delta: "day", Seconds: 60, ImproveByPercent: &negativeImprove}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateGoalInput(tt.input); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateGoalInputAllowsDefaultsAndAliases(t *testing.T) {
	percent := 25.0
	input := GoalInput{
		Title:            "Improve",
		Seconds:          0,
		IgnoreDays:       []string{"mon", "Friday"},
		ImproveByPercent: &percent,
	}

	if err := ValidateGoalInput(input); err != nil {
		t.Fatalf("expected valid goal input, got %v", err)
	}
}

func TestLeaderboardStoreMethodsValidateRangeBeforeNormalizing(t *testing.T) {
	sourceBytes, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("could not read store.go: %v", err)
	}
	source := string(sourceBytes)
	for _, functionName := range []string{"CreateLeaderboard", "UpdateLeaderboard"} {
		body := functionSource(source, functionName)
		if !strings.Contains(body, "services.ValidateLeaderboardInput") {
			t.Fatalf("%s must validate leaderboard input before normalizing", functionName)
		}
	}
}

func TestUpdateUserMarksStatsStale(t *testing.T) {
	sourceBytes, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("could not read store.go: %v", err)
	}
	body := functionSource(string(sourceBytes), "UpdateUser")
	if !strings.Contains(body, "s.MarkStatsStale(ctx, id)") {
		t.Fatal("UpdateUser must mark cached stats stale after settings changes")
	}
}

func TestValidateUserSettingsRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name  string
		input UserSettingsInput
	}{
		{name: "invalid timezone", input: UserSettingsInput{Timezone: "Not/AZone", TimeoutMinutes: 15}},
		{name: "negative timeout", input: UserSettingsInput{Timezone: "UTC", TimeoutMinutes: -1}},
		{name: "negative retention", input: UserSettingsInput{Timezone: "UTC", TimeoutMinutes: 15, HeartbeatRetentionDays: -1}},
		{name: "invalid country", input: UserSettingsInput{Timezone: "UTC", TimeoutMinutes: 15, Country: "USA"}},
		{name: "invalid public username", input: UserSettingsInput{Timezone: "UTC", TimeoutMinutes: 15, PublicUsername: "-keith"}},
		{name: "invalid project visibility", input: UserSettingsInput{Timezone: "UTC", TimeoutMinutes: 15, PublicProjectVisibility: "friends"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateUserSettings(tt.input); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestNormalizeUserSettingsDefaultsTimezoneAndPreservesZeroTimeout(t *testing.T) {
	input := NormalizeUserSettings(UserSettingsInput{
		Timezone:               "",
		TimeoutMinutes:         0,
		Country:                " us ",
		HeartbeatRetentionDays: 30,
	})

	if input.Timezone != "UTC" {
		t.Fatalf("expected default timezone UTC, got %q", input.Timezone)
	}
	if input.TimeoutMinutes != 0 {
		t.Fatalf("expected zero timeout to remain valid, got %d", input.TimeoutMinutes)
	}
	if input.Country != "US" {
		t.Fatalf("expected country US, got %q", input.Country)
	}
	if input.HeartbeatRetentionDays != 30 {
		t.Fatalf("expected retention 30, got %d", input.HeartbeatRetentionDays)
	}
}

func TestCustomRulesProgressAbortedStatusIsTerminal(t *testing.T) {
	if !customRulesProgressIsAborted(CustomRulesProgress{Status: "Aborted"}) {
		t.Fatal("expected Aborted custom rules progress to be treated as aborted")
	}
	if customRulesProgressIsAborted(CustomRulesProgress{Status: "Completed"}) {
		t.Fatal("expected Completed custom rules progress not to be treated as aborted")
	}
}
