package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/keithah/stint/internal/aicostbake"
	apimw "github.com/keithah/stint/internal/api/middleware"
	"github.com/keithah/stint/internal/auth"
	"github.com/keithah/stint/internal/cache"
	"github.com/keithah/stint/internal/config"
	"github.com/keithah/stint/internal/db"
	dumpfiles "github.com/keithah/stint/internal/dumps"
	"github.com/keithah/stint/internal/importer"
	"github.com/keithah/stint/internal/jobs"
	"github.com/keithah/stint/internal/pricing"
	"github.com/keithah/stint/internal/pricingrefresh"
	"github.com/keithah/stint/internal/services"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

const sessionCookieName = "stint_session"
const sessionJWTCookieName = "stint_api_jwt"
const githubOAuthStateCookieName = "stint_github_oauth_state"
const oauthAccessTokenTTL = 365 * 24 * time.Hour
const oauthImplicitAccessTokenTTL = 12 * time.Hour
const leaderboardCacheTTL = time.Hour
const sessionJWTTTL = 30 * 24 * time.Hour
const githubOAuthStateTTL = 10 * time.Minute

type Server struct {
	Config           config.Config
	Store            *db.Store
	OAuth            *oauth2.Config
	Limiter          apimw.RateLimiter
	FallbackLimiter  apimw.RateLimiter
	StatusCache      cache.StatusCache
	LeaderboardCache cache.LeaderboardCache
	Jobs             jobs.Client
	Pricing          *pricing.Engine
}

const (
	heartbeatIngestionRateLimit = 1000
	authenticatedReadRateLimit  = 60
	oauthTokenCreationRateLimit = 10
)

var (
	errRegistrationClosed      = errors.New("registration is disabled")
	errMaxUsersReached         = errors.New("maximum user count reached")
	errPublicSummariesDisabled = errors.New("public summaries are disabled")
)

func NewRouter(cfg config.Config, store *db.Store) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     compactOrigins("http://localhost:3000", cfg.BaseURL, cfg.WebBaseURL),
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
		AllowCredentials: true,
	}))

	oauthConfig := &oauth2.Config{
		ClientID:     cfg.GitHubClientID,
		ClientSecret: cfg.GitHubClientSecret,
		Endpoint:     github.Endpoint,
		Scopes:       []string{"read:user", "user:email"},
		RedirectURL:  cfg.BaseURL + "/auth/github/callback",
	}
	limiter := apimw.RateLimiter(apimw.NewMemoryRateLimiter())
	if cfg.RedisURL != "" {
		if redisLimiter, err := apimw.NewRedisRateLimiter(cfg.RedisURL); err == nil {
			limiter = redisLimiter
		}
	}
	statusCache := cache.StatusCache(cache.NewMemoryStatusCache())
	if cfg.RedisURL != "" {
		if redisStatusCache, err := cache.NewRedisStatusCache(cfg.RedisURL); err == nil {
			statusCache = redisStatusCache
		}
	}
	leaderboardCache := cache.LeaderboardCache(cache.NewMemoryLeaderboardCache())
	if cfg.RedisURL != "" {
		if redisLeaderboardCache, err := cache.NewRedisLeaderboardCache(cfg.RedisURL); err == nil {
			leaderboardCache = redisLeaderboardCache
		}
	}
	jobClient := jobs.Client(jobs.NoopClient{})
	if cfg.RedisURL != "" {
		if asynqClient, err := jobs.NewAsynqClient(cfg.RedisURL); err == nil {
			jobClient = asynqClient
		}
	}
	pricingEngine, err := pricing.NewFromBundled()
	if err != nil {
		e.Logger.Errorf("usage pricing engine unavailable, AI usage will be reported as unpriced: %v", err)
		pricingEngine = nil
	}
	server := &Server{Config: cfg, Store: store, OAuth: oauthConfig, Limiter: limiter, FallbackLimiter: apimw.NewMemoryRateLimiter(), StatusCache: statusCache, LeaderboardCache: leaderboardCache, Jobs: jobClient, Pricing: pricingEngine}
	// Keep the API's pricing engine in sync with the weekly refresh so the AI
	// cost meter reflects the latest upstream prices without a redeploy.
	if pricingEngine != nil && store != nil {
		go pricingrefresh.Refresher{Store: store, Engine: pricingEngine}.Run(context.Background(), 30*time.Minute)
	}

	e.GET("/healthz", server.health)
	e.GET("/healthz/ingestion", server.ingestionHealth)
	e.GET("/auth/github/login", server.githubLogin)
	e.GET("/auth/github/callback", server.githubCallback)
	e.POST("/auth/logout", server.logout)
	e.GET("/oauth/authorize", server.oauthAuthorize)
	e.POST("/oauth/authorize", server.oauthAuthorizePost)
	e.POST("/oauth/token", server.oauthToken, server.rateLimitOAuthToken(oauthTokenCreationRateLimit, time.Hour))
	e.POST("/oauth/revoke", server.oauthRevoke)

	api := e.Group("/api/v1")
	api.GET("/meta", server.meta)
	api.GET("/docs", server.openAPIDocs)
	api.GET("/leaders", server.publicLeaders, server.rateLimitIP("public-read", 60, time.Minute))
	api.GET("/editors", server.editors)
	api.GET("/program_languages", server.programLanguages)
	api.GET("/users/:user", server.publicUserProfile, server.rateLimitIP("public-user", 60, time.Minute))
	api.GET("/users/:user/summaries", server.publicUserSummaries, server.rateLimitIP("public-user-summaries", 60, time.Minute))
	api.GET("/users/:user/stats", server.publicUserStats, server.rateLimitIP("public-user-stats", 60, time.Minute))
	api.GET("/users/:user/stats/:range", server.publicUserStats, server.rateLimitIP("public-user-stats", 60, time.Minute))
	api.GET("/users/:user/share/:token/stats", server.publicShareStats, server.rateLimitIP("share-stats", 120, time.Minute))
	api.GET("/users/:user/share/:token/summaries", server.publicShareSummaries, server.rateLimitIP("share-summaries", 120, time.Minute))
	api.GET("/share/:token/stats", server.publicShareStatsByToken, server.rateLimitIP("share-stats", 120, time.Minute))
	api.GET("/share/:token/summaries", server.publicShareSummariesByToken, server.rateLimitIP("share-summaries", 120, time.Minute))
	api.POST("/dev/seed-key", server.devSeed)
	api.POST("/dev/jobs/heartbeats-purge", server.devHeartbeatsPurge)
	api.POST("/dev/jobs/leaderboard-update", server.devLeaderboardUpdate)
	api.POST("/dev/jobs/goals-evaluate", server.devGoalsEvaluate)
	readLimit := server.rateLimitAuthenticatedRead(authenticatedReadRateLimit, time.Minute)
	api.GET("/auth/me", server.currentUser, server.requireUser, readLimit)

	current := api.Group("/users/current", server.requireUser)
	current.GET("", server.currentUser, readLimit)
	current.PUT("", server.updateCurrentUser, requireLocalAccountAccess)
	current.DELETE("", server.deleteCurrentUser, requireLocalAccountAccess)
	current.GET("/heartbeats", server.listHeartbeats, requireScope(scopeReadHeartbeats), readLimit)
	current.POST("/heartbeats", server.createHeartbeat, requireScope(scopeWriteHeartbeats), server.rateLimitUser("heartbeats", heartbeatIngestionRateLimit, time.Minute))
	current.POST("/heartbeats.bulk", server.createHeartbeatsBulk, requireScope(scopeWriteHeartbeats), server.rateLimitUser("heartbeats", heartbeatIngestionRateLimit, time.Minute))
	current.DELETE("/heartbeats.bulk", server.deleteHeartbeatsBulk, requireScope(scopeWriteHeartbeats))
	current.POST("/usage_events.bulk", server.createUsageEventsBulk, requireScope(scopeWriteHeartbeats), server.rateLimitUser("usage_events", heartbeatIngestionRateLimit, time.Minute))
	current.GET("/usage_events", server.listUsageEvents, requireScope(scopeReadStats), readLimit)
	current.GET("/usage_events/summary", server.usageEventsSummary, requireScope(scopeReadStats), readLimit)
	current.GET("/usage_events/blocks", server.usageEventsBlocks, requireScope(scopeReadStats), readLimit)
	current.GET("/custom_pricing", server.listCustomPricing, requireScope(scopeReadStats), readLimit)
	current.PUT("/custom_pricing", server.upsertCustomPricing, requireLocalAccountAccess)
	current.DELETE("/custom_pricing/:model", server.deleteCustomPricing, requireLocalAccountAccess)
	current.GET("/pricing/sources", server.listPricingSources, requireScope(scopeReadStats), readLimit)
	current.GET("/pricing/models", server.listPricingModels, requireScope(scopeReadStats), readLimit)
	current.GET("/billing_prefs", server.listBillingPrefs, requireScope(scopeReadStats), readLimit)
	current.PUT("/billing_prefs", server.upsertBillingPref, requireLocalAccountAccess)
	current.DELETE("/billing_prefs/:agent", server.deleteBillingPref, requireLocalAccountAccess)
	current.POST("/file_experts", server.fileExperts, requireScope(scopeReadStats), readLimit)
	current.GET("/durations", server.durations, requireSummarySliceScope, readLimit)
	current.GET("/summaries", server.summaries, requireSummaryScope, readLimit)
	current.GET("/stats", server.allStats, requireScope(scopeReadStats), readLimit)
	current.GET("/stats/last_7_days", server.last7DaysStats, requireScope(scopeReadStats), readLimit)
	current.GET("/stats/:range", server.statsForRange, requireScope(scopeReadStats), readLimit)
	current.GET("/status_bar/today", server.statusBarToday, requireScope(scopeReadStats), readLimit)
	current.GET("/statusbar/today", server.statusBarTodayWakaTime, requireScope(scopeReadStats), readLimit)
	current.GET("/all_time_since_today", server.allTimeSinceToday, requireScope(scopeReadStats), readLimit)
	current.GET("/projects", server.listProjects, requireScope(scopeReadStatsProjects), readLimit)
	current.GET("/projects/:project/commits", server.projectCommits, requireScope(scopeReadHeartbeats), readLimit)
	current.GET("/projects/:project/commits/:hash", server.projectCommit, requireScope(scopeReadHeartbeats), readLimit)
	current.GET("/projects/:project", server.projectDetail, requireScope(scopeReadStatsProjects), readLimit)
	current.GET("/machine_names", server.listMachineNames, requireScope(scopeReadStatsMachines), readLimit)
	current.GET("/user_agents", server.listUserAgents, requireScope(scopeReadStatsEditors), readLimit)
	current.GET("/insights/:insight_type/:range", server.insight, requireInsightScope, readLimit)
	current.GET("/goals", server.listGoals, requireScope(scopeReadGoals), readLimit)
	current.POST("/goals", server.createGoal, requireScope(scopeReadGoals))
	current.GET("/goals/:goal", server.getGoal, requireScope(scopeReadGoals), readLimit)
	current.PUT("/goals/:goal", server.updateGoal, requireScope(scopeReadGoals))
	current.DELETE("/goals/:goal", server.deleteGoal, requireScope(scopeReadGoals))
	current.GET("/external_durations", server.listExternalDurations, requireScope(scopeReadSummaries), readLimit)
	current.POST("/external_durations", server.createExternalDuration, requireScope(scopeWriteHeartbeats))
	current.POST("/external_durations.bulk", server.createExternalDurationsBulk, requireScope(scopeWriteHeartbeats))
	current.DELETE("/external_durations.bulk", server.deleteExternalDurationsBulk, requireScope(scopeWriteHeartbeats))
	current.GET("/leaderboards", server.listLeaderboards, requireScope(scopeReadPrivateLeaderboards), readLimit)
	current.POST("/leaderboards", server.createLeaderboard, requireScope(scopeWritePrivateLeaderboards))
	current.GET("/leaderboards/:board", server.getLeaderboard, requireScope(scopeReadPrivateLeaderboards), readLimit)
	current.PUT("/leaderboards/:board", server.updateLeaderboard, requireScope(scopeWritePrivateLeaderboards))
	current.DELETE("/leaderboards/:board", server.deleteLeaderboard, requireScope(scopeWritePrivateLeaderboards))
	current.POST("/leaderboards/:board/members", server.addLeaderboardMember, requireScope(scopeWritePrivateLeaderboards))
	current.DELETE("/leaderboards/:board/members/:user", server.removeLeaderboardMember, requireScope(scopeWritePrivateLeaderboards))
	current.GET("/data_dumps", server.listDataDumps, requireScope(scopeReadHeartbeats), readLimit)
	current.POST("/data_dumps", server.createDataDump, requireScope(scopeReadHeartbeats))
	current.GET("/data_dumps/:dump/download", server.downloadDataDump, requireScope(scopeReadHeartbeats), readLimit)
	current.GET("/custom_rules", server.listCustomRules, requireScope(scopeReadStats), readLimit)
	current.PUT("/custom_rules", server.replaceCustomRules, requireLocalAccountAccess)
	current.DELETE("/custom_rules/:rule_id", server.deleteCustomRule, requireLocalAccountAccess)
	current.GET("/custom_rules_progress", server.customRulesProgress, requireScope(scopeReadStats), readLimit)
	current.DELETE("/custom_rules_progress", server.abortCustomRulesProgress, requireLocalAccountAccess)
	current.GET("/share_tokens", server.listShareTokens, requireLocalAccountAccess, readLimit)
	current.POST("/share_tokens", server.createShareToken, requireLocalAccountAccess)
	current.DELETE("/share_tokens/:id", server.deleteShareToken, requireLocalAccountAccess)
	current.POST("/imports/wakatime", server.importWakaTimeDump, requireScope(scopeWriteHeartbeats))
	current.GET("/ai_costs", server.listAICosts, requireScope(scopeReadStats), readLimit)
	current.PUT("/ai_costs", server.replaceAICosts, requireLocalAccountAccess)

	keys := api.Group("/api_keys", server.requireUser, requireLocalAccountAccess)
	keys.GET("", server.listAPIKeys, readLimit)
	keys.POST("", server.createAPIKey)
	keys.DELETE("/:id", server.revokeAPIKey)

	oauthApps := api.Group("/oauth/apps", server.requireUser, requireLocalAccountAccess)
	oauthApps.GET("", server.listOAuthApps, readLimit)
	oauthApps.POST("", server.createOAuthApp)
	oauthApps.DELETE("/:id", server.deleteOAuthApp)

	return e
}

func (s *Server) health(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.Pool.Ping(ctx); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "errors": []string{err.Error()}})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

// ingestionHealth reports global heartbeat-ingestion freshness for external
// monitors. It is intentionally separate from /healthz: a healthy server with
// no recent heartbeats is still live, but a monitor can alert on a stalled
// feed by watching seconds_since_last_heartbeat during expected active hours.
func (s *Server) ingestionHealth(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
	defer cancel()
	now := time.Now()
	stats, err := s.Store.IngestionStats(ctx, float64(now.Unix()))
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "errors": []string{err.Error()}})
	}
	resp := map[string]any{
		"ok":              true,
		"count_last_hour": stats.CountLastHour,
		"count_last_24h":  stats.CountLast24h,
	}
	if stats.LastHeartbeatTime > 0 {
		resp["last_heartbeat_at"] = int64(stats.LastHeartbeatTime)
		resp["seconds_since_last_heartbeat"] = now.Unix() - int64(stats.LastHeartbeatTime)
	}
	return c.JSON(http.StatusOK, resp)
}

func (s *Server) meta(c echo.Context) error {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "stint"
	}
	baseURL := strings.TrimRight(s.Config.BaseURL, "/")
	return c.JSON(http.StatusOK, map[string]any{
		"data": map[string]any{
			"api_url":  baseURL + "/api/v1",
			"base_url": baseURL,
			"hostname": hostname,
			"ip":       clientIP(c.Request()),
			"version":  "phase1",
		},
	})
}

func clientIP(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		for _, candidate := range strings.Split(r.Header.Get(header), ",") {
			if ip := strings.TrimSpace(candidate); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func (s *Server) openAPIDocs(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"openapi": "3.1.0",
		"info": map[string]string{
			"title":   "Stint API",
			"version": "0.1.0",
		},
		"servers": []map[string]string{{"url": s.Config.BaseURL}},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"BearerAuth":       map[string]string{"type": "http", "scheme": "bearer"},
				"BasicAuth":        map[string]string{"type": "http", "scheme": "basic"},
				"OAuthClientBasic": map[string]string{"type": "http", "scheme": "basic"},
				"ApiKeyQuery":      map[string]string{"type": "apiKey", "in": "query", "name": "api_key"},
				"SessionCookie":    map[string]string{"type": "apiKey", "in": "cookie", "name": sessionCookieName},
			},
			"schemas": openAPISchemas(),
		},
		"paths": openAPIPaths(),
	})
}

type openAPIOperation struct {
	Method             string
	Summary            string
	Tag                string
	Status             int
	RequireAuth        bool
	RateLimited        bool
	SecuritySchemes    []string
	Request            string
	Response           string
	AltResponses       map[int]string
	Parameters         []map[string]any
	FormFields         []string
	MultipartFileField string
}

func openAPIPaths() map[string]any {
	paths := map[string]any{}
	add := func(path string, operations ...openAPIOperation) {
		methods := map[string]any{}
		for _, operation := range operations {
			status := operation.Status
			if status == 0 {
				status = http.StatusOK
			}
			responses := map[string]any{
				fmt.Sprintf("%d", status): openAPIResponse(http.StatusText(status), operation.Response),
			}
			for altStatus, schema := range operation.AltResponses {
				responses[fmt.Sprintf("%d", altStatus)] = openAPIResponse(http.StatusText(altStatus), schema)
			}
			body := map[string]any{
				"summary":   operation.Summary,
				"tags":      []string{operation.Tag},
				"responses": responses,
			}
			requestContent := map[string]any{}
			if operation.Request != "" {
				requestContent["application/json"] = map[string]any{"schema": openAPIRef(operation.Request)}
			}
			if operation.MultipartFileField != "" {
				requestContent["multipart/form-data"] = openAPIMultipartFileContent(operation.MultipartFileField)
			}
			if len(requestContent) > 0 {
				body["requestBody"] = map[string]any{"required": true, "content": requestContent}
			}
			if len(operation.FormFields) > 0 {
				body["requestBody"] = openAPIFormRequestBody(operation.FormFields)
			}
			parameters := openAPIPathParameters(path)
			if len(operation.Parameters) > 0 {
				parameters = append(parameters, operation.Parameters...)
			}
			if len(parameters) > 0 {
				body["parameters"] = parameters
			}
			if operation.RequireAuth {
				body["security"] = []map[string][]string{
					{"BearerAuth": {}},
					{"BasicAuth": {}},
					{"ApiKeyQuery": {}},
					{"SessionCookie": {}},
				}
				responses["401"] = openAPIResponse("Unauthorized", "Error")
			}
			if operation.RequireAuth || operation.RateLimited {
				responses["429"] = openAPIRateLimitResponse()
			}
			if len(operation.SecuritySchemes) > 0 {
				security := make([]map[string][]string, 0, len(operation.SecuritySchemes))
				for _, scheme := range operation.SecuritySchemes {
					security = append(security, map[string][]string{scheme: {}})
				}
				body["security"] = security
			}
			methods[strings.ToLower(operation.Method)] = body
		}
		paths[path] = methods
	}
	get := func(summary, tag string, auth bool) openAPIOperation {
		return openAPIOperation{Method: http.MethodGet, Summary: summary, Tag: tag, RequireAuth: auth}
	}
	post := func(summary, tag string, status int, auth bool) openAPIOperation {
		return openAPIOperation{Method: http.MethodPost, Summary: summary, Tag: tag, Status: status, RequireAuth: auth}
	}
	put := func(summary, tag string, auth bool) openAPIOperation {
		return openAPIOperation{Method: http.MethodPut, Summary: summary, Tag: tag, RequireAuth: auth}
	}
	del := func(summary, tag string, status int, auth bool) openAPIOperation {
		return openAPIOperation{Method: http.MethodDelete, Summary: summary, Tag: tag, Status: status, RequireAuth: auth}
	}
	withResponse := func(operation openAPIOperation, schema string) openAPIOperation {
		operation.Response = schema
		return operation
	}
	withQueryParams := func(operation openAPIOperation, names ...string) openAPIOperation {
		for _, name := range names {
			operation.Parameters = append(operation.Parameters, openAPIQueryParameter(name))
		}
		return operation
	}
	withFormFields := func(operation openAPIOperation, names ...string) openAPIOperation {
		operation.FormFields = append(operation.FormFields, names...)
		return operation
	}
	withMultipartFile := func(operation openAPIOperation, name string) openAPIOperation {
		operation.MultipartFileField = name
		return operation
	}
	withAcceptedResponse := func(operation openAPIOperation, schema string) openAPIOperation {
		if operation.AltResponses == nil {
			operation.AltResponses = map[int]string{}
		}
		operation.AltResponses[http.StatusAccepted] = schema
		return operation
	}
	withRateLimit := func(operation openAPIOperation) openAPIOperation {
		operation.RateLimited = true
		return operation
	}
	withJSON := func(operation openAPIOperation, request, response string) openAPIOperation {
		operation.Request = request
		operation.Response = response
		return operation
	}
	withSecuritySchemes := func(operation openAPIOperation, schemes ...string) openAPIOperation {
		operation.SecuritySchemes = append(operation.SecuritySchemes, schemes...)
		return operation
	}

	add("/api/v1/meta", withResponse(get("Get server metadata", "meta", false), "MetaResponse"))
	add("/api/v1/docs", get("Get OpenAPI document", "meta", false))
	add("/healthz", get("Get service health", "meta", false))
	add("/healthz/ingestion", get("Get heartbeat ingestion freshness", "meta", false))
	add("/auth/github/login", openAPIOperation{Method: http.MethodGet, Summary: "Start GitHub login", Tag: "auth", Status: http.StatusFound})
	add("/auth/github/callback", withQueryParams(openAPIOperation{Method: http.MethodGet, Summary: "Complete GitHub login", Tag: "auth", Status: http.StatusFound}, "code", "state"))
	add("/auth/logout", withResponse(post("Clear browser session", "auth", http.StatusOK, false), "LogoutResponse"))
	add("/api/v1/leaders", withRateLimit(withQueryParams(withResponse(get("Get public leaderboard", "leaderboards", false), "PublicLeaderboardResponse"), "language", "country")))
	add("/api/v1/editors", withResponse(get("List known editors", "metadata", false), "EditorListResponse"))
	add("/api/v1/program_languages", withResponse(get("List known programming languages", "metadata", false), "ProgramLanguageListResponse"))
	add("/api/v1/users/{user}", withRateLimit(withResponse(get("Get public user profile", "users", false), "PublicUserResponse")))
	add("/api/v1/users/{user}/summaries", withRateLimit(withQueryParams(withResponse(get("Get public user summaries", "summaries", false), "PublicSummaryResponse"), "start", "end")))
	add("/api/v1/users/{user}/stats", withRateLimit(withResponse(get("Get public user stats", "stats", false), "PublicStatsResponse")))
	add("/api/v1/users/{user}/stats/{range}", withRateLimit(withResponse(get("Get public user stats for range", "stats", false), "PublicStatsResponse")))
	add("/api/v1/auth/me", withResponse(get("Get authenticated user", "auth", true), "UserResponse"))
	add("/api/v1/api_keys",
		withResponse(get("List API keys", "auth", true), "APIKeyListResponse"),
		withJSON(post("Create API key", "auth", http.StatusCreated, true), "APIKeyCreateRequest", "APIKeyCreateResponse"),
	)
	add("/api/v1/api_keys/{id}", del("Revoke API key", "auth", http.StatusNoContent, true))
	add("/api/v1/oauth/apps",
		withResponse(get("List OAuth applications", "oauth", true), "OAuthAppListResponse"),
		withJSON(post("Create OAuth application", "oauth", http.StatusCreated, true), "OAuthAppCreateRequest", "OAuthAppResponse"),
	)
	add("/api/v1/oauth/apps/{id}", del("Delete OAuth application", "oauth", http.StatusNoContent, true))
	add("/api/v1/users/current",
		withResponse(get("Get current user profile", "users", true), "UserResponse"),
		withJSON(put("Update current user profile", "users", true), "UserUpdateRequest", "UserResponse"),
		withJSON(del("Delete current user account", "users", http.StatusOK, true), "AccountDeleteRequest", "UserDeleteResponse"),
	)
	add("/api/v1/users/current/heartbeats",
		withQueryParams(withResponse(get("List heartbeats for a day", "heartbeats", true), "HeartbeatListResponse"), "date"),
		withJSON(post("Create heartbeat", "heartbeats", http.StatusAccepted, true), "Heartbeat", "HeartbeatResponse"),
	)
	add("/api/v1/users/current/heartbeats.bulk",
		withJSON(post("Create heartbeats in bulk", "heartbeats", http.StatusAccepted, true), "HeartbeatBulkRequest", "HeartbeatBulkResponse"),
		withJSON(del("Delete heartbeats in bulk", "heartbeats", http.StatusOK, true), "HeartbeatBulkDeleteRequest", "DeletedCountResponse"),
	)
	add("/api/v1/users/current/usage_events.bulk", withRateLimit(post("Ingest AI usage events in bulk", "usage", http.StatusOK, true)))
	add("/api/v1/users/current/usage_events", withQueryParams(get("Export AI usage events", "usage", true), "start", "end"))
	add("/api/v1/users/current/usage_events/summary", withQueryParams(get("Get AI usage cost summary", "usage", true), "range", "start", "end", "cost_mode", "agent"))
	add("/api/v1/users/current/usage_events/blocks", withQueryParams(get("Get 5-hour usage blocks and burn rate", "usage", true), "range", "start", "end", "cost_mode", "agent"))
	add("/api/v1/users/current/custom_pricing",
		get("List custom AI pricing overrides", "ai", true),
		put("Upsert a custom AI pricing override", "ai", true),
	)
	add("/api/v1/users/current/custom_pricing/{model}", del("Delete a custom AI pricing override", "ai", http.StatusOK, true))
	add("/api/v1/users/current/pricing/sources", get("List AI price sources and freshness", "ai", true))
	add("/api/v1/users/current/pricing/models", get("List resolved per-model AI prices", "ai", true))
	add("/api/v1/users/current/billing_prefs",
		get("List per-agent billing-mode overrides", "ai", true),
		put("Upsert a per-agent billing-mode override", "ai", true),
	)
	add("/api/v1/users/current/billing_prefs/{agent}", del("Delete a per-agent billing-mode override", "ai", http.StatusOK, true))
	add("/api/v1/users/current/file_experts", withJSON(post("Get file experts", "heartbeats", http.StatusOK, true), "FileExpertsRequest", "FileExpertsResponse"))
	add("/api/v1/users/current/durations", withQueryParams(withResponse(get("Get durations for a day", "stats", true), "DurationResponse"), "date", "slice_by"))
	add("/api/v1/users/current/summaries", withQueryParams(withResponse(get("Get summaries for a date range", "stats", true), "SummaryResponse"), "start", "end"))
	add("/api/v1/users/current/stats", withAcceptedResponse(withResponse(get("Get all stats ranges", "stats", true), "StatsRangesResponse"), "StatsRangesResponse"))
	add("/api/v1/users/current/stats/last_7_days", withAcceptedResponse(withResponse(get("Get last 7 days stats", "stats", true), "StatsResponse"), "StatsResponse"))
	add("/api/v1/users/current/stats/{range}", withAcceptedResponse(withResponse(get("Get stats for a range", "stats", true), "StatsResponse"), "StatsResponse"))
	add("/api/v1/users/current/status_bar/today", withResponse(get("Get status bar stats", "stats", true), "StatusBarResponse"))
	add("/api/v1/users/current/statusbar/today", withResponse(get("Get status bar stats", "stats", true), "WakaTimeStatusBarResponse"))
	add("/api/v1/users/current/all_time_since_today", withResponse(get("Get all-time stats", "stats", true), "AllTimeResponse"))
	add("/api/v1/users/current/projects", withResponse(get("List projects", "projects", true), "ProjectListResponse"))
	add("/api/v1/users/current/projects/{project}/commits", withQueryParams(withResponse(get("List project commits", "projects", true), "ProjectCommitListResponse"), "branch", "page"))
	add("/api/v1/users/current/projects/{project}/commits/{hash}", withResponse(get("Get project commit", "projects", true), "ProjectCommitResponse"))
	add("/api/v1/users/current/projects/{project}", withQueryParams(withResponse(get("Get project detail", "projects", true), "ProjectDetailResponse"), "range"))
	add("/api/v1/users/current/machine_names", withResponse(get("List machine names", "metadata", true), "MachineNameListResponse"))
	add("/api/v1/users/current/user_agents", withResponse(get("List user agents", "metadata", true), "UserAgentListResponse"))
	add("/api/v1/users/current/insights/{insight_type}/{range}", withResponse(get("Get insight slice", "insights", true), "InsightResponse"))
	add("/api/v1/users/current/goals",
		withResponse(get("List goals", "goals", true), "GoalListResponse"),
		withJSON(post("Create goal", "goals", http.StatusCreated, true), "GoalInput", "GoalResponse"),
	)
	add("/api/v1/users/current/goals/{goal}",
		withResponse(get("Get goal", "goals", true), "WakaTimeGoalResponse"),
		withJSON(put("Update goal", "goals", true), "GoalInput", "GoalResponse"),
		del("Delete goal", "goals", http.StatusNoContent, true),
	)
	add("/api/v1/users/current/external_durations",
		withResponse(get("List external durations", "external durations", true), "ExternalDurationListResponse"),
		withJSON(post("Create external duration", "external durations", http.StatusCreated, true), "ExternalDuration", "ExternalDurationResponse"),
	)
	add("/api/v1/users/current/external_durations.bulk",
		withJSON(post("Create external durations in bulk", "external durations", http.StatusAccepted, true), "ExternalDurationBulkRequest", "ExternalDurationBulkResponse"),
		withJSON(del("Delete external durations in bulk", "external durations", http.StatusOK, true), "IDListRequest", "DeletedCountResponse"),
	)
	add("/api/v1/users/current/leaderboards",
		withResponse(get("List private leaderboards", "leaderboards", true), "LeaderboardListResponse"),
		withJSON(post("Create private leaderboard", "leaderboards", http.StatusCreated, true), "LeaderboardInput", "LeaderboardResponse"),
	)
	add("/api/v1/users/current/leaderboards/{board}",
		withResponse(get("Get private leaderboard", "leaderboards", true), "LeaderboardEntriesResponse"),
		withJSON(put("Update private leaderboard", "leaderboards", true), "LeaderboardInput", "LeaderboardResponse"),
		del("Delete private leaderboard", "leaderboards", http.StatusNoContent, true),
	)
	add("/api/v1/users/current/leaderboards/{board}/members", withJSON(post("Add private leaderboard member", "leaderboards", http.StatusCreated, true), "LeaderboardMemberRequest", "LeaderboardMemberResponse"))
	add("/api/v1/users/current/leaderboards/{board}/members/{user}", del("Remove private leaderboard member", "leaderboards", http.StatusNoContent, true))
	add("/api/v1/users/current/data_dumps",
		withResponse(get("List data dumps", "data dumps", true), "DataDumpListResponse"),
		withAcceptedResponse(withJSON(post("Create data dump", "data dumps", http.StatusCreated, true), "DataDumpRequest", "DataDumpResponse"), "DataDumpResponse"),
	)
	add("/api/v1/users/current/data_dumps/{dump}/download", withResponse(get("Download data dump", "data dumps", true), "DataDumpDownloadResponse"))
	add("/api/v1/users/current/custom_rules",
		withResponse(get("List custom rules", "custom rules", true), "CustomRuleListResponse"),
		withJSON(put("Replace custom rules", "custom rules", true), "CustomRulesRequest", "CustomRuleListResponse"),
	)
	add("/api/v1/users/current/custom_rules/{rule_id}", del("Delete custom rule", "custom rules", http.StatusNoContent, true))
	add("/api/v1/users/current/custom_rules_progress",
		withResponse(get("Get custom rule application progress", "custom rules", true), "CustomRuleProgressResponse"),
		withResponse(del("Abort custom rule application progress", "custom rules", http.StatusOK, true), "CustomRuleProgressResponse"),
	)
	add("/api/v1/users/current/share_tokens",
		withResponse(get("List share tokens", "sharing", true), "ShareTokenListResponse"),
		withJSON(post("Create share token", "sharing", http.StatusCreated, true), "ShareTokenCreateRequest", "ShareTokenResponse"),
	)
	add("/api/v1/users/current/share_tokens/{id}", del("Delete share token", "sharing", http.StatusNoContent, true))
	add("/api/v1/users/current/imports/wakatime", withMultipartFile(withJSON(post("Import WakaTime dump", "imports", http.StatusAccepted, true), "WakaTimeImportRequest", "WakaTimeImportResponse"), "file"))
	add("/api/v1/users/current/ai_costs",
		withResponse(get("List AI cost settings", "ai", true), "AICostSettingsResponse"),
		withJSON(put("Replace AI cost settings", "ai", true), "AICostSettingsRequest", "AICostSettingsResponse"),
	)
	add("/api/v1/users/{user}/share/{token}/stats", withRateLimit(withQueryParams(withResponse(get("Get shared stats", "sharing", false), "PublicStatsResponse"), "range", "callback")))
	add("/api/v1/users/{user}/share/{token}/summaries", withRateLimit(withQueryParams(withResponse(get("Get shared summaries", "sharing", false), "PublicSummaryResponse"), "start", "end", "callback")))
	add("/api/v1/share/{token}/stats", withRateLimit(withQueryParams(withResponse(get("Get shared stats by token", "sharing", false), "PublicStatsResponse"), "range", "callback")))
	add("/api/v1/share/{token}/summaries", withRateLimit(withQueryParams(withResponse(get("Get shared summaries by token", "sharing", false), "PublicSummaryResponse"), "start", "end", "callback")))
	add("/oauth/authorize",
		withQueryParams(get("Render OAuth authorization prompt", "oauth", true), "response_type", "client_id", "redirect_uri", "scope", "state"),
		withFormFields(post("Approve or deny OAuth authorization", "oauth", http.StatusFound, true), "response_type", "client_id", "redirect_uri", "scope", "state", "decision"),
	)
	add("/oauth/token", withRateLimit(withSecuritySchemes(withResponse(withFormFields(post("Exchange OAuth token", "oauth", http.StatusOK, false), "grant_type", "code", "redirect_uri", "refresh_token", "client_id", "client_secret"), "OAuthTokenResponse"), "OAuthClientBasic")))
	add("/oauth/revoke", withSecuritySchemes(withResponse(withFormFields(post("Revoke OAuth token", "oauth", http.StatusOK, false), "token", "client_id", "client_secret"), "OAuthRevokeResponse"), "OAuthClientBasic"))
	return paths
}

func openAPIResponse(description, schema string) map[string]any {
	response := map[string]any{"description": description}
	if schema != "" {
		response["content"] = map[string]any{
			"application/json": map[string]any{"schema": openAPIRef(schema)},
		}
	}
	return response
}

func openAPIRateLimitResponse() map[string]any {
	response := openAPIResponse("Too Many Requests", "Error")
	response["headers"] = map[string]any{
		"Retry-After": map[string]any{
			"description": "Seconds to wait before retrying the request.",
			"schema": map[string]any{
				"type":    "integer",
				"minimum": 1,
			},
		},
	}
	return response
}

func openAPIRef(schema string) map[string]string {
	return map[string]string{"$ref": "#/components/schemas/" + schema}
}

func openAPIFormRequestBody(fields []string) map[string]any {
	properties := map[string]any{}
	for _, field := range fields {
		properties[field] = map[string]any{
			"type":        "string",
			"description": openAPIQueryParameterDescription(field),
		}
	}
	return map[string]any{
		"required": true,
		"content": map[string]any{
			"application/x-www-form-urlencoded": map[string]any{
				"schema": map[string]any{
					"type":       "object",
					"properties": properties,
				},
			},
		},
	}
}

func openAPIMultipartFileContent(field string) map[string]any {
	return map[string]any{
		"schema": map[string]any{
			"type":     "object",
			"required": []string{field},
			"properties": map[string]any{
				field: map[string]any{
					"type":   "string",
					"format": "binary",
				},
			},
		},
	}
}

func openAPIQueryParameter(name string) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "query",
		"required":    false,
		"description": openAPIQueryParameterDescription(name),
		"schema":      map[string]string{"type": "string"},
	}
}

func openAPIPathParameters(path string) []map[string]any {
	var parameters []map[string]any
	remaining := path
	seen := map[string]bool{}
	for {
		start := strings.Index(remaining, "{")
		if start == -1 {
			break
		}
		end := strings.Index(remaining[start+1:], "}")
		if end == -1 {
			break
		}
		name := remaining[start+1 : start+1+end]
		if name != "" && !seen[name] {
			parameters = append(parameters, map[string]any{
				"name":        name,
				"in":          "path",
				"required":    true,
				"description": openAPIPathParameterDescription(path, name),
				"schema":      openAPIPathParameterSchema(path, name),
			})
			seen[name] = true
		}
		remaining = remaining[start+end+2:]
	}
	return parameters
}

func openAPIPathParameterSchema(path, name string) map[string]any {
	schema := map[string]any{"type": "string"}
	if name == "insight_type" && strings.Contains(path, "/insights/") {
		schema["enum"] = supportedInsightTypes()
	}
	return schema
}

func openAPIPathParameterDescription(path, name string) string {
	if name == "token" && strings.Contains(path, "/share/") {
		return "Share token."
	}
	if name == "insight_type" && strings.Contains(path, "/insights/") {
		return "Insight breakdown type."
	}
	return openAPIQueryParameterDescription(name)
}

func openAPIQueryParameterDescription(name string) string {
	switch name {
	case "callback":
		return "Optional JSONP callback function name."
	case "language":
		return "Optional language filter."
	case "country":
		return "Optional ISO country code filter."
	case "range":
		return "Optional stats range."
	case "start":
		return "Start date in YYYY-MM-DD format."
	case "end":
		return "End date in YYYY-MM-DD format."
	case "date":
		return "Date in YYYY-MM-DD format."
	case "slice_by":
		return "Duration grouping field."
	case "branch":
		return "Optional branch filter."
	case "page":
		return "Optional one-based page number."
	case "response_type":
		return "OAuth response type: code or token."
	case "client_id":
		return "OAuth client identifier."
	case "client_secret":
		return "OAuth client secret."
	case "redirect_uri":
		return "Registered OAuth redirect URI."
	case "scope":
		return "Space-delimited OAuth scopes."
	case "state":
		return "Opaque OAuth state returned to the client."
	case "decision":
		return "OAuth consent decision."
	case "grant_type":
		return "OAuth grant type."
	case "code":
		return "OAuth authorization code."
	case "refresh_token":
		return "OAuth refresh token."
	case "token":
		return "OAuth token to revoke."
	case "user":
		return "GitHub username or local user identifier."
	case "project":
		return "Project name."
	case "hash":
		return "Commit hash."
	case "goal":
		return "Goal identifier."
	case "board":
		return "Leaderboard identifier."
	case "id":
		return "Resource identifier."
	case "rule_id":
		return "Custom rule identifier."
	case "dump":
		return "Data dump identifier."
	default:
		return "Optional query parameter."
	}
}

func openAPISchemas() map[string]any {
	stringSchema := map[string]string{"type": "string"}
	integerSchema := map[string]string{"type": "integer"}
	numberSchema := map[string]string{"type": "number"}
	booleanSchema := map[string]string{"type": "boolean"}
	stringArraySchema := map[string]any{"type": "array", "items": stringSchema}
	sliceTotalArray := map[string]any{"type": "array", "items": openAPIRef("SliceTotal")}
	return map[string]any{
		"Error": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"error":  stringSchema,
				"errors": map[string]any{"type": "array", "items": stringSchema},
			},
		},
		"ServerMeta": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"api_url":  stringSchema,
				"base_url": stringSchema,
				"hostname": stringSchema,
				"ip":       stringSchema,
				"version":  stringSchema,
			},
		},
		"MetaResponse": map[string]any{
			"type":       "object",
			"properties": map[string]any{"data": openAPIRef("ServerMeta")},
		},
		"User": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":                       stringSchema,
				"github_id":                integerSchema,
				"github_username":          stringSchema,
				"email":                    stringSchema,
				"full_name":                stringSchema,
				"avatar_url":               stringSchema,
				"country":                  stringSchema,
				"timezone":                 stringSchema,
				"timeout_minutes":          integerSchema,
				"writes_only":              booleanSchema,
				"is_hireable":              booleanSchema,
				"has_public_profile":       booleanSchema,
				"heartbeat_retention_days": integerSchema,
			},
		},
		"UserResponse": map[string]any{
			"type":       "object",
			"properties": map[string]any{"data": openAPIRef("User")},
		},
		"UserDeleteResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{
					"type":       "object",
					"properties": map[string]any{"deleted": booleanSchema},
				},
			},
		},
		"PublicUser": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":              stringSchema,
				"username":        stringSchema,
				"name":            stringSchema,
				"github_username": stringSchema,
				"github_url":      stringSchema,
				"avatar_url":      stringSchema,
				"permissions": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"total_time":         booleanSchema,
						"projects":           booleanSchema,
						"project_visibility": stringSchema,
						"languages":          booleanSchema,
						"editors":            booleanSchema,
						"machines":           booleanSchema,
						"operating_systems":  booleanSchema,
						"categories":         booleanSchema,
						"ai":                 booleanSchema,
						"summaries":          booleanSchema,
						"github":             booleanSchema,
					},
				},
			},
		},
		"PublicUserResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": openAPIRef("PublicUser"),
			},
		},
		"PublicStatsResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": openAPIRef("Stats"),
				"user": openAPIRef("PublicUser"),
			},
		},
		"PublicSummaryResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("SummaryDay")},
				"user": openAPIRef("PublicUser"),
			},
		},
		"DeletedCountResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{
					"type":       "object",
					"properties": map[string]any{"deleted": integerSchema},
				},
			},
		},
		"Heartbeat": map[string]any{
			"type":     "object",
			"required": []string{"entity", "time"},
			"properties": map[string]any{
				"entity":            stringSchema,
				"type":              stringSchema,
				"category":          stringSchema,
				"time":              numberSchema,
				"project":           stringSchema,
				"alternate_project": stringSchema,
				"branch":            stringSchema,
				"commit_hash":       stringSchema,
				"revision":          stringSchema,
				"language":          stringSchema,
				"dependencies": map[string]any{
					"oneOf": []any{
						stringSchema,
						map[string]any{"type": "array", "items": stringSchema},
					},
				},
				"machine_name":         stringSchema,
				"plugin":               stringSchema,
				"plugin_version":       stringSchema,
				"editor":               stringSchema,
				"editor_version":       stringSchema,
				"operating_system":     stringSchema,
				"architecture":         stringSchema,
				"is_write":             booleanSchema,
				"lines":                integerSchema,
				"lineno":               integerSchema,
				"cursorpos":            integerSchema,
				"ai_line_changes":      integerSchema,
				"human_line_changes":   integerSchema,
				"ai_session":           stringSchema,
				"ai_input_tokens":      integerSchema,
				"ai_output_tokens":     integerSchema,
				"ai_prompt_length":     integerSchema,
				"ai_subscription_plan": stringSchema,
				"ai_model":             stringSchema,
				"ai_model_name":        stringSchema,
				"model_name":           stringSchema,
				"llm_model":            stringSchema,
				"model":                stringSchema,
				"ai_provider":          stringSchema,
				"provider":             stringSchema,
				"llm_provider":         stringSchema,
				"ai_agent":             stringSchema,
				"ai_agent_name":        stringSchema,
				"ai_agent_version":     stringSchema,
				"ai_agent_complexity":  stringSchema,
				"metadata": map[string]any{
					"type":                 "object",
					"additionalProperties": true,
				},
				"raw_payload": map[string]any{
					"type":                 "object",
					"additionalProperties": true,
					"readOnly":             true,
				},
			},
		},
		"HeartbeatResponse": map[string]any{
			"type":       "object",
			"properties": map[string]any{"data": openAPIRef("Heartbeat")},
		},
		"HeartbeatListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("Heartbeat")},
			},
		},
		"HeartbeatBulkRequest": map[string]any{
			"type":     "array",
			"maxItems": 25,
			"items":    openAPIRef("Heartbeat"),
		},
		"HeartbeatBulkResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"responses": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "array",
						"minItems": 2,
						"maxItems": 2,
						"items":    map[string]any{},
					},
				},
			},
		},
		"EditorMetadata": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":    stringSchema,
				"key":     stringSchema,
				"version": stringSchema,
			},
		},
		"EditorListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("EditorMetadata")},
			},
		},
		"ProgramLanguage": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":  stringSchema,
				"color": stringSchema,
			},
		},
		"ProgramLanguageListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("ProgramLanguage")},
			},
		},
		"APIKeyCreateRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "minLength": 1},
				"scopes": stringArraySchema,
			},
		},
		"APIKey": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":           stringSchema,
				"user_id":      stringSchema,
				"name":         stringSchema,
				"fingerprint":  stringSchema,
				"scopes":       stringArraySchema,
				"last_used_at": stringSchema,
				"created_at":   stringSchema,
			},
		},
		"APIKeyCreateResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"key":     openAPIRef("APIKey"),
						"api_key": stringSchema,
					},
				},
			},
		},
		"APIKeyListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("APIKey")},
			},
		},
		"OAuthAppCreateRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "minLength": 1},
				"redirect_uris": map[string]any{
					"type":     "array",
					"minItems": 1,
					"items": map[string]string{
						"type":   "string",
						"format": "uri",
					},
				},
				"scopes": stringArraySchema,
			},
		},
		"OAuthApp": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":                        stringSchema,
				"user_id":                   stringSchema,
				"name":                      stringSchema,
				"client_id":                 stringSchema,
				"client_secret":             stringSchema,
				"client_secret_fingerprint": stringSchema,
				"redirect_uris":             stringArraySchema,
				"scopes":                    stringArraySchema,
				"created_at":                stringSchema,
				"modified_at":               stringSchema,
			},
		},
		"OAuthAppResponse": map[string]any{
			"type":       "object",
			"properties": map[string]any{"data": openAPIRef("OAuthApp")},
		},
		"OAuthAppListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("OAuthApp")},
			},
		},
		"UserUpdateRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"timezone": map[string]any{
					"type":    "string",
					"pattern": "^(UTC|[A-Za-z_]+(/[A-Za-z0-9_+\\-]+)+)$",
				},
				"timeout_minutes": map[string]any{
					"type":    "integer",
					"minimum": 0,
				},
				"writes_only":                   booleanSchema,
				"has_public_profile":            booleanSchema,
				"country":                       map[string]any{"type": "string", "pattern": "^[A-Za-z]{2}$"},
				"heartbeat_retention_days":      map[string]any{"type": "integer", "minimum": 0},
				"public_username":               map[string]any{"type": "string", "pattern": "^[A-Za-z0-9][A-Za-z0-9_-]{1,37}[A-Za-z0-9]$"},
				"public_display_name":           stringSchema,
				"public_github_link_enabled":    booleanSchema,
				"public_show_total_time":        booleanSchema,
				"public_show_projects":          booleanSchema,
				"public_project_visibility":     map[string]any{"type": "string", "enum": []string{"none", "public_repos", "all"}},
				"public_show_languages":         booleanSchema,
				"public_show_editors":           booleanSchema,
				"public_show_machines":          booleanSchema,
				"public_show_operating_systems": booleanSchema,
				"public_show_categories":        booleanSchema,
				"public_show_ai":                booleanSchema,
				"public_show_summaries":         booleanSchema,
			},
		},
		"AccountDeleteRequest": map[string]any{
			"type":       "object",
			"required":   []string{"confirmation"},
			"properties": map[string]any{"confirmation": stringSchema},
		},
		"HeartbeatBulkDeleteRequest": map[string]any{
			"type":     "object",
			"required": []string{"date", "ids"},
			"properties": map[string]any{
				"date": stringSchema,
				"ids":  stringArraySchema,
			},
		},
		"FileExpertsRequest": map[string]any{
			"type":     "object",
			"required": []string{"entity"},
			"properties": map[string]any{
				"entity":  stringSchema,
				"project": stringSchema,
			},
		},
		"FileExpertsResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"total": map[string]any{"type": "object"},
							"user":  map[string]any{"type": "object"},
						},
					},
				},
			},
		},
		"DurationRow": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":     stringSchema,
				"project":  stringSchema,
				"language": stringSchema,
				"time":     numberSchema,
				"duration": numberSchema,
			},
		},
		"DurationResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("DurationRow")},
			},
		},
		"SummaryRange": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date":  stringSchema,
				"start": stringSchema,
				"end":   stringSchema,
			},
		},
		"SummaryTotal": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"total_seconds": integerSchema,
				"text":          stringSchema,
			},
		},
		"SummaryDay": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"range":             openAPIRef("SummaryRange"),
				"grand_total":       openAPIRef("SummaryTotal"),
				"projects":          sliceTotalArray,
				"languages":         sliceTotalArray,
				"categories":        sliceTotalArray,
				"dependencies":      sliceTotalArray,
				"editors":           sliceTotalArray,
				"machines":          sliceTotalArray,
				"operating_systems": sliceTotalArray,
			},
		},
		"SummaryResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("SummaryDay")},
			},
		},
		"StatusBarResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cached": booleanSchema,
				"data": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"total_seconds":      integerSchema,
						"grand_total_text":   stringSchema,
						"project":            stringSchema,
						"project_seconds":    integerSchema,
						"project_text":       stringSchema,
						"language":           stringSchema,
						"language_seconds":   integerSchema,
						"language_text":      stringSchema,
						"range":              stringSchema,
						"percent_calculated": integerSchema,
					},
				},
			},
		},
		"MachineName": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":           stringSchema,
				"name":         stringSchema,
				"value":        stringSchema,
				"timezone":     stringSchema,
				"last_seen_at": stringSchema,
				"created_at":   stringSchema,
			},
		},
		"MachineNameListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("MachineName")},
			},
		},
		"UserAgent": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":                   stringSchema,
				"value":                stringSchema,
				"editor":               stringSchema,
				"ai_model":             stringSchema,
				"ai_provider":          stringSchema,
				"ai_agent":             stringSchema,
				"ai_agent_version":     stringSchema,
				"ai_agent_complexity":  stringSchema,
				"version":              stringSchema,
				"os":                   stringSchema,
				"last_seen_at":         stringSchema,
				"is_browser_extension": booleanSchema,
				"is_desktop_app":       booleanSchema,
				"created_at":           stringSchema,
			},
		},
		"UserAgentListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("UserAgent")},
			},
		},
		"GoalInput": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":        stringSchema,
				"custom_title": stringSchema,
				"delta": map[string]any{
					"type": "string",
					"enum": []string{"day", "week"},
				},
				"seconds": map[string]any{
					"type":    "integer",
					"minimum": 0,
				},
				"languages":        stringArraySchema,
				"editors":          stringArraySchema,
				"projects":         stringArraySchema,
				"ignore_days":      stringArraySchema,
				"ignore_zero_days": booleanSchema,
				"improve_by_percent": map[string]any{
					"type":    "number",
					"minimum": 0,
				},
				"is_enabled":   booleanSchema,
				"is_inverse":   booleanSchema,
				"is_snoozed":   booleanSchema,
				"snooze_until": stringSchema,
			},
		},
		"Goal": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":                 stringSchema,
				"title":              stringSchema,
				"custom_title":       stringSchema,
				"delta":              stringSchema,
				"seconds":            integerSchema,
				"languages":          stringArraySchema,
				"editors":            stringArraySchema,
				"projects":           stringArraySchema,
				"ignore_days":        stringArraySchema,
				"ignore_zero_days":   booleanSchema,
				"improve_by_percent": numberSchema,
				"is_enabled":         booleanSchema,
				"is_inverse":         booleanSchema,
				"is_snoozed":         booleanSchema,
				"snooze_until":       stringSchema,
				"created_at":         stringSchema,
				"modified_at":        stringSchema,
			},
		},
		"GoalProgress": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"goal":                  openAPIRef("Goal"),
				"actual_seconds":        integerSchema,
				"target_seconds":        integerSchema,
				"percent":               integerSchema,
				"is_complete":           booleanSchema,
				"human_readable_actual": stringSchema,
				"human_readable_target": stringSchema,
				"remaining_seconds":     integerSchema,
				"is_snoozed":            booleanSchema,
				"is_ignored":            booleanSchema,
			},
		},
		"GoalResponse": map[string]any{
			"type":       "object",
			"properties": map[string]any{"data": openAPIRef("Goal")},
		},
		"GoalListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("GoalProgress")},
			},
		},
		"WakaTimeGoalResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cached_at": stringSchema,
				"data":      map[string]any{"type": "object"},
			},
		},
		"ExternalDuration": map[string]any{
			"type":     "object",
			"required": []string{"external_id", "provider", "entity", "type", "start_time", "end_time"},
			"properties": map[string]any{
				"id":          stringSchema,
				"external_id": stringSchema,
				"provider":    stringSchema,
				"entity":      stringSchema,
				"type":        stringSchema,
				"category":    stringSchema,
				"start_time":  map[string]any{"type": "number", "exclusiveMinimum": 0},
				"end_time":    map[string]any{"type": "number", "exclusiveMinimum": 0},
				"project":     stringSchema,
				"branch":      stringSchema,
				"language":    stringSchema,
				"meta":        stringSchema,
			},
		},
		"ExternalDurationBulkRequest": map[string]any{
			"type":     "array",
			"maxItems": 1000,
			"items":    openAPIRef("ExternalDuration"),
		},
		"ExternalDurationResponse": map[string]any{
			"type":       "object",
			"properties": map[string]any{"data": openAPIRef("ExternalDuration")},
		},
		"ExternalDurationListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("ExternalDuration")},
			},
		},
		"ExternalDurationBulkResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"responses": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"status": integerSchema,
							"data":   openAPIRef("ExternalDuration"),
							"error":  stringSchema,
						},
					},
				},
			},
		},
		"IDListRequest": map[string]any{
			"type":       "object",
			"required":   []string{"ids"},
			"properties": map[string]any{"ids": stringArraySchema},
		},
		"Leaderboard": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":          stringSchema,
				"name":        stringSchema,
				"time_range":  stringSchema,
				"created_at":  stringSchema,
				"modified_at": stringSchema,
			},
		},
		"LeaderboardEntry": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"user_id":       stringSchema,
				"username":      stringSchema,
				"display_name":  stringSchema,
				"avatar_url":    stringSchema,
				"country":       stringSchema,
				"total_seconds": integerSchema,
				"text":          stringSchema,
				"rank":          integerSchema,
			},
		},
		"PublicLeaderboardMeta": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cached":     booleanSchema,
				"range":      stringSchema,
				"language":   stringSchema,
				"country":    stringSchema,
				"updated_at": stringSchema,
			},
		},
		"PublicLeaderboardResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("LeaderboardEntry")},
				"meta": openAPIRef("PublicLeaderboardMeta"),
			},
		},
		"LeaderboardMember": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"user_id":   stringSchema,
				"username":  stringSchema,
				"full_name": stringSchema,
				"role":      stringSchema,
			},
		},
		"LeaderboardResponse": map[string]any{
			"type":       "object",
			"properties": map[string]any{"data": openAPIRef("Leaderboard")},
		},
		"LeaderboardListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("Leaderboard")},
			},
		},
		"LeaderboardEntriesResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data":    map[string]any{"type": "array", "items": openAPIRef("LeaderboardEntry")},
				"board":   openAPIRef("Leaderboard"),
				"members": map[string]any{"type": "array", "items": openAPIRef("LeaderboardMember")},
			},
		},
		"LeaderboardInput": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "minLength": 1},
				"time_range": map[string]any{
					"type":    "string",
					"pattern": "^(last_7_days|last_30_days|last_6_months|last_year|all_time|[0-9]{4}|[0-9]{4}-[0-9]{2})$",
				},
			},
		},
		"LeaderboardMemberRequest": map[string]any{
			"type":     "object",
			"required": []string{"username"},
			"properties": map[string]any{
				"username": stringSchema,
			},
		},
		"LeaderboardMemberResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"user_id":   stringSchema,
						"username":  stringSchema,
						"full_name": stringSchema,
					},
				},
			},
		},
		"CustomRuleDestination": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"destination":       map[string]any{"type": "string", "enum": customRuleFieldValues()},
				"destination_value": stringSchema,
			},
		},
		"CustomRule": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":           stringSchema,
				"action":       map[string]any{"type": "string", "enum": []string{"change", "delete"}},
				"source":       map[string]any{"type": "string", "enum": customRuleFieldValues()},
				"operation":    map[string]any{"type": "string", "enum": []string{"equals", "contains", "starts_with", "ends_with", "regex", "matches"}},
				"source_value": stringSchema,
				"priority":     integerSchema,
				"destinations": map[string]any{"type": "array", "items": openAPIRef("CustomRuleDestination")},
			},
		},
		"CustomRulesRequest": map[string]any{
			"type":  "array",
			"items": openAPIRef("CustomRule"),
		},
		"CustomRuleListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("CustomRule")},
			},
		},
		"CustomRuleProgress": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":           stringSchema,
				"percent_complete": integerSchema,
				"total":            integerSchema,
				"changed":          integerSchema,
				"deleted":          integerSchema,
				"error":            stringSchema,
				"started_at":       stringSchema,
				"completed_at":     stringSchema,
				"modified_at":      stringSchema,
			},
		},
		"CustomRuleProgressResponse": map[string]any{
			"type":       "object",
			"properties": map[string]any{"data": openAPIRef("CustomRuleProgress")},
		},
		"ShareToken": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":           stringSchema,
				"name":         stringSchema,
				"token":        stringSchema,
				"fingerprint":  stringSchema,
				"last_used_at": stringSchema,
				"created_at":   stringSchema,
			},
		},
		"ShareTokenResponse": map[string]any{
			"type":       "object",
			"properties": map[string]any{"data": openAPIRef("ShareToken")},
		},
		"ShareTokenListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("ShareToken")},
			},
		},
		"ShareTokenCreateRequest": map[string]any{
			"type":       "object",
			"properties": map[string]any{"name": map[string]any{"type": "string", "minLength": 1}},
		},
		"WakaTimeImportRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data":       map[string]any{"type": "array", "items": openAPIRef("Heartbeat")},
				"heartbeats": map[string]any{"type": "array", "items": openAPIRef("Heartbeat")},
			},
		},
		"WakaTimeImportResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"status":     stringSchema,
						"inserted":   integerSchema,
						"duplicates": integerSchema,
						"invalid":    integerSchema,
						"total":      integerSchema,
					},
				},
			},
		},
		"OAuthTokenResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"access_token":  stringSchema,
				"refresh_token": stringSchema,
				"token_type":    stringSchema,
				"expires_in":    integerSchema,
				"scope":         stringSchema,
			},
		},
		"OAuthRevokeResponse": map[string]any{
			"type":       "object",
			"properties": map[string]any{"revoked": booleanSchema},
		},
		"LogoutResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{
					"type":       "object",
					"properties": map[string]any{"ok": booleanSchema},
				},
			},
		},
		"AICostSetting": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agent":                         stringSchema,
				"input_cost_per_million_cents":  map[string]any{"type": "integer", "minimum": 0},
				"output_cost_per_million_cents": map[string]any{"type": "integer", "minimum": 0},
			},
		},
		"AICostSettingsRequest": map[string]any{
			"type":  "array",
			"items": openAPIRef("AICostSetting"),
		},
		"AICostSettingsResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("AICostSetting")},
			},
		},
		"SliceTotal": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":          stringSchema,
				"total_seconds": integerSchema,
				"text":          stringSchema,
			},
		},
		"DailyStat": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date":          stringSchema,
				"total_seconds": integerSchema,
				"text":          stringSchema,
				"projects":      sliceTotalArray,
			},
		},
		"HourlyStat": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"hour":          integerSchema,
				"label":         stringSchema,
				"total_seconds": integerSchema,
				"text":          stringSchema,
				"projects":      sliceTotalArray,
				"languages":     sliceTotalArray,
			},
		},
		"AIStat": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":                 stringSchema,
				"ai_seconds":           integerSchema,
				"ai_line_changes":      integerSchema,
				"human_line_changes":   integerSchema,
				"ai_input_tokens":      integerSchema,
				"ai_output_tokens":     integerSchema,
				"ai_prompt_length":     integerSchema,
				"session_count":        integerSchema,
				"estimated_cost_cents": integerSchema,
			},
		},
		"AICostPeriod": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agent":         stringSchema,
				"daily_cents":   integerSchema,
				"weekly_cents":  integerSchema,
				"monthly_cents": integerSchema,
				"total_cents":   integerSchema,
			},
		},
		"AIMetrics": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ai_line_changes":         integerSchema,
				"human_line_changes":      integerSchema,
				"ai_percentage":           integerSchema,
				"human_review_percentage": integerSchema,
				"follow_up_edits":         integerSchema,
				"ai_input_tokens":         integerSchema,
				"ai_output_tokens":        integerSchema,
				"prompt_count":            integerSchema,
				"average_prompt_length":   integerSchema,
				"median_prompt_length":    integerSchema,
				"session_count":           integerSchema,
				"estimated_cost_cents":    integerSchema,
				"agents":                  map[string]any{"type": "array", "items": openAPIRef("AIStat")},
				"days":                    map[string]any{"type": "array", "items": openAPIRef("AIStat")},
				"costs":                   map[string]any{"type": "array", "items": openAPIRef("AICostPeriod")},
			},
		},
		"Stats": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"range":                        stringSchema,
				"total_seconds":                integerSchema,
				"human_readable_total":         stringSchema,
				"daily_average_seconds":        integerSchema,
				"human_readable_daily_average": stringSchema,
				"best_day":                     openAPIRef("DailyStat"),
				"days":                         map[string]any{"type": "array", "items": openAPIRef("DailyStat")},
				"hourly":                       map[string]any{"type": "array", "items": openAPIRef("HourlyStat")},
				"projects":                     sliceTotalArray,
				"languages":                    sliceTotalArray,
				"editors":                      sliceTotalArray,
				"operating_systems":            sliceTotalArray,
				"machines":                     sliceTotalArray,
				"categories":                   sliceTotalArray,
				"branches":                     sliceTotalArray,
				"dependencies":                 sliceTotalArray,
				"ai":                           openAPIRef("AIMetrics"),
				"project_ai":                   map[string]any{"type": "array", "items": openAPIRef("AIStat")},
				"is_up_to_date":                booleanSchema,
				"percent_calculated":           integerSchema,
			},
		},
		"StatsResponse": map[string]any{
			"type":       "object",
			"properties": map[string]any{"data": openAPIRef("Stats")},
		},
		"StatsRangesResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{
					"type":                 "object",
					"additionalProperties": openAPIRef("Stats"),
				},
			},
		},
		"WakaTimeStatusBarResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cached_at":          stringSchema,
				"data":               map[string]any{"type": "object"},
				"has_team_features":  booleanSchema,
				"percent_calculated": integerSchema,
			},
		},
		"AllTimeResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"total_seconds": integerSchema,
						"text":          stringSchema,
						"stats":         openAPIRef("Stats"),
					},
				},
			},
		},
		"InsightResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{},
			},
		},
		"Project": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":                 stringSchema,
				"name":               stringSchema,
				"color":              stringSchema,
				"has_public_url":     booleanSchema,
				"badge":              stringSchema,
				"first_heartbeat_at": stringSchema,
				"last_heartbeat_at":  stringSchema,
				"created_at":         stringSchema,
			},
		},
		"ProjectListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("Project")},
			},
		},
		"ProjectDetail": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project": openAPIRef("Project"),
				"stats":   openAPIRef("Stats"),
			},
		},
		"ProjectDetailResponse": map[string]any{
			"type":       "object",
			"properties": map[string]any{"data": openAPIRef("ProjectDetail")},
		},
		"ProjectCommitProject": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":             stringSchema,
				"name":           stringSchema,
				"color":          stringSchema,
				"has_public_url": booleanSchema,
				"badge":          stringSchema,
				"privacy":        stringSchema,
				"repository":     stringSchema,
			},
		},
		"CommitSummary": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":                                stringSchema,
				"hash":                              stringSchema,
				"truncated_hash":                    stringSchema,
				"branch":                            stringSchema,
				"ref":                               stringSchema,
				"total_seconds":                     integerSchema,
				"human_readable_total":              stringSchema,
				"human_readable_total_with_seconds": stringSchema,
				"created_at":                        stringSchema,
				"author_date":                       stringSchema,
				"committer_date":                    stringSchema,
				"html_url":                          stringSchema,
				"url":                               stringSchema,
			},
		},
		"ProjectCommitListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"commits":       map[string]any{"type": "array", "items": openAPIRef("CommitSummary")},
				"branch":        stringSchema,
				"page":          integerSchema,
				"next_page":     integerSchema,
				"next_page_url": stringSchema,
				"prev_page":     integerSchema,
				"prev_page_url": stringSchema,
				"total":         integerSchema,
				"total_pages":   integerSchema,
				"status":        stringSchema,
				"project":       openAPIRef("ProjectCommitProject"),
			},
		},
		"ProjectCommitResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"commit":  openAPIRef("CommitSummary"),
				"branch":  stringSchema,
				"project": openAPIRef("ProjectCommitProject"),
				"status":  stringSchema,
			},
		},
		"DataDumpRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type": map[string]any{
					"type": "string",
					"enum": []string{"heartbeats", "daily"},
				},
			},
		},
		"DataDump": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":               stringSchema,
				"type":             stringSchema,
				"status":           stringSchema,
				"percent_complete": numberSchema,
				"download_url":     stringSchema,
				"is_processing":    booleanSchema,
				"is_stuck":         booleanSchema,
				"has_failed":       booleanSchema,
				"expires_at":       stringSchema,
				"created_at":       stringSchema,
			},
		},
		"DataDumpResponse": map[string]any{
			"type":       "object",
			"properties": map[string]any{"data": openAPIRef("DataDump")},
		},
		"DataDumpListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{"type": "array", "items": openAPIRef("DataDump")},
			},
		},
		"DataDumpDownloadResponse": map[string]any{
			"type": "array",
			"items": map[string]any{
				"oneOf": []any{openAPIRef("Heartbeat"), openAPIRef("SummaryDay")},
			},
		},
	}
}

func customRuleFieldValues() []string {
	return []string{"entity", "type", "category", "project", "branch", "language", "editor", "operating_system"}
}

func (s *Server) editors(c echo.Context) error {
	if s.Store != nil {
		editors, err := s.Store.ListEditors(c.Request().Context())
		if err == nil && len(editors) > 0 {
			return c.JSON(http.StatusOK, dataArray(editors))
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"data": services.KnownEditors()})
}

func (s *Server) programLanguages(c echo.Context) error {
	if s.Store != nil {
		languages, err := s.Store.ListProgramLanguages(c.Request().Context())
		if err == nil && len(languages) > 0 {
			return c.JSON(http.StatusOK, dataArray(languages))
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"data": services.KnownProgramLanguages()})
}

func (s *Server) publicLeaders(c echo.Context) error {
	if !s.Config.EnablePublicLeaderboard {
		return c.JSON(http.StatusNotFound, errorBody("public leaderboard is disabled"))
	}
	const rangeName = "last_7_days"
	language := strings.TrimSpace(c.QueryParam("language"))
	country := strings.ToUpper(strings.TrimSpace(c.QueryParam("country")))
	cacheKey := leaderboardCacheKey(rangeName, language, country)
	if s.LeaderboardCache != nil {
		entries, ok, err := s.LeaderboardCache.Get(c.Request().Context(), cacheKey)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
		}
		if ok {
			return c.JSON(http.StatusOK, map[string]any{"data": nonNilSlice(entries), "meta": leaderboardMeta(true, rangeName, language, country)})
		}
	}
	entries, err := s.refreshLeaderboardCache(c.Request().Context(), rangeName, language, country)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, map[string]any{"data": nonNilSlice(entries), "meta": leaderboardMeta(false, rangeName, language, country)})
}

func (s *Server) githubLogin(c echo.Context) error {
	if s.Config.GitHubClientID == "" || s.Config.GitHubClientSecret == "" {
		return c.JSON(http.StatusServiceUnavailable, errorBody("GitHub OAuth is not configured; use DEV_SEED_ENABLED for local setup"))
	}
	state, signedState, err := newGitHubOAuthState(s.Config.SessionSecret)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	c.SetCookie(&http.Cookie{
		Name:     githubOAuthStateCookieName,
		Value:    signedState,
		Path:     "/auth/github/callback",
		MaxAge:   int(githubOAuthStateTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return c.Redirect(http.StatusFound, s.OAuth.AuthCodeURL(state, oauth2.AccessTypeOnline))
}

func newGitHubOAuthState(secret string) (string, string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", "", err
	}
	state := base64.RawURLEncoding.EncodeToString(random)
	return state, signedGitHubOAuthState(state, secret), nil
}

func signedGitHubOAuthState(state, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(state))
	return state + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func verifyGitHubOAuthState(state, signedState, secret string) bool {
	if state == "" || signedState == "" {
		return false
	}
	expected := signedGitHubOAuthState(state, secret)
	return hmac.Equal([]byte(signedState), []byte(expected))
}

type githubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func primaryGitHubEmail(profileEmail string, emails []githubEmail) string {
	profileEmail = strings.TrimSpace(profileEmail)
	if profileEmail != "" {
		return profileEmail
	}
	for _, email := range emails {
		if email.Primary && email.Verified && strings.TrimSpace(email.Email) != "" {
			return strings.TrimSpace(email.Email)
		}
	}
	for _, email := range emails {
		if email.Verified && strings.TrimSpace(email.Email) != "" {
			return strings.TrimSpace(email.Email)
		}
	}
	return ""
}

func (s *Server) ensureGitHubRegistrationAllowed(ctx context.Context, githubID int64) error {
	if s.Store == nil {
		return nil
	}
	if _, err := s.Store.UserByGitHubID(ctx, githubID); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if !s.Config.EnableRegistration {
		return errRegistrationClosed
	}
	if s.Config.MaxUsers <= 0 {
		return nil
	}
	count, err := s.Store.CountUsers(ctx)
	if err != nil {
		return err
	}
	if !registrationAllowsNewUser(true, s.Config.MaxUsers, count) {
		return errMaxUsersReached
	}
	return nil
}

func registrationAllowsNewUser(enabled bool, maxUsers, currentUsers int) bool {
	if !enabled {
		return false
	}
	return maxUsers <= 0 || currentUsers < maxUsers
}

func (s *Server) githubCallback(c echo.Context) error {
	code := c.QueryParam("code")
	if code == "" {
		return c.JSON(http.StatusBadRequest, errorBody("missing GitHub OAuth code"))
	}
	stateCookie, err := c.Cookie(githubOAuthStateCookieName)
	if err != nil || !verifyGitHubOAuthState(c.QueryParam("state"), stateCookie.Value, s.Config.SessionSecret) {
		return c.JSON(http.StatusBadRequest, errorBody("invalid GitHub OAuth state"))
	}
	c.SetCookie(&http.Cookie{
		Name:     githubOAuthStateCookieName,
		Value:    "",
		Path:     "/auth/github/callback",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	token, err := s.OAuth.Exchange(c.Request().Context(), code)
	if err != nil {
		return c.JSON(http.StatusBadGateway, errorBody("could not exchange GitHub OAuth code"))
	}

	client := s.OAuth.Client(c.Request().Context(), token)
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return c.JSON(http.StatusBadGateway, errorBody("could not fetch GitHub user"))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return c.JSON(http.StatusBadGateway, errorBody("GitHub user request failed"))
	}

	var gh githubUser
	if err := json.NewDecoder(resp.Body).Decode(&gh); err != nil {
		return c.JSON(http.StatusBadGateway, errorBody("could not decode GitHub user"))
	}
	emails := []githubEmail{}
	emailResp, err := client.Get("https://api.github.com/user/emails")
	if err == nil {
		defer emailResp.Body.Close()
		if emailResp.StatusCode < 300 {
			_ = json.NewDecoder(emailResp.Body).Decode(&emails)
		}
	}
	if err := s.ensureGitHubRegistrationAllowed(c.Request().Context(), gh.ID); err != nil {
		if errors.Is(err, errRegistrationClosed) || errors.Is(err, errMaxUsersReached) {
			return c.JSON(http.StatusForbidden, errorBody(err.Error()))
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	user, err := s.Store.UpsertGitHubUser(c.Request().Context(), db.GitHubProfile{
		ID: gh.ID, Username: gh.Login, Email: primaryGitHubEmail(gh.Email, emails), FullName: gh.Name, AvatarURL: gh.AvatarURL,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	if err := s.setSessionCookie(c, user.ID); err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.Redirect(http.StatusFound, s.frontendURL("/dashboard"))
}

func (s *Server) logout(c echo.Context) error {
	if token, ok := sessionTokenFromCookie(c); ok && s.Store != nil {
		if err := s.Store.DeleteSession(c.Request().Context(), token); err != nil {
			clearSessionCookies(c)
			return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
		}
	}
	clearSessionCookies(c)
	return c.JSON(http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

func (s *Server) oauthAuthorize(c echo.Context) error {
	user, ok := s.sessionUser(c)
	if !ok {
		return c.Redirect(http.StatusFound, s.frontendURL("/login"))
	}
	params, app, err := s.validOAuthAuthorizationRequest(c)
	if err != nil {
		return c.HTML(http.StatusBadRequest, oauthErrorHTML(err.Error()))
	}
	scopeText := strings.Join(params.Scopes, " ")
	body := fmt.Sprintf(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Authorize %s</title></head>
<body style="margin:0;background:#09090b;color:#f4f4f5;font-family:ui-sans-serif,system-ui,sans-serif">
<main style="min-height:100vh;display:grid;place-items:center;padding:24px">
<section style="width:min(520px,100%%);border:1px solid #27272a;background:#111113;padding:28px;border-radius:8px">
<p style="color:#a1a1aa;margin:0 0 8px">Signed in as %s</p>
<h1 style="font-size:24px;margin:0 0 12px">Authorize %s</h1>
<p style="color:#d4d4d8;line-height:1.5">%s is requesting access to: <strong>%s</strong></p>
<form method="post" action="/oauth/authorize" style="display:flex;gap:12px;margin-top:24px">
<input type="hidden" name="response_type" value="%s">
<input type="hidden" name="client_id" value="%s">
<input type="hidden" name="redirect_uri" value="%s">
<input type="hidden" name="scope" value="%s">
<input type="hidden" name="state" value="%s">
<button name="decision" value="allow" style="background:#a3e635;color:#111113;border:0;border-radius:6px;padding:10px 16px;font-weight:700">Authorize</button>
<button name="decision" value="deny" style="background:transparent;color:#f4f4f5;border:1px solid #3f3f46;border-radius:6px;padding:10px 16px">Deny</button>
</form>
</section>
</main>
</body>
</html>`,
		html.EscapeString(app.Name),
		html.EscapeString(user.GitHubUsername),
		html.EscapeString(app.Name),
		html.EscapeString(app.Name),
		html.EscapeString(scopeText),
		html.EscapeString(params.ResponseType),
		html.EscapeString(params.ClientID),
		html.EscapeString(params.RedirectURI),
		html.EscapeString(scopeText),
		html.EscapeString(params.State))
	return c.HTML(http.StatusOK, body)
}

func (s *Server) oauthAuthorizePost(c echo.Context) error {
	user, ok := s.sessionUser(c)
	if !ok {
		return c.Redirect(http.StatusFound, s.frontendURL("/login"))
	}
	params, app, err := s.validOAuthAuthorizationRequest(c)
	if err != nil {
		return c.HTML(http.StatusBadRequest, oauthErrorHTML(err.Error()))
	}
	if c.FormValue("decision") != "allow" {
		return redirectWithOAuthParams(c, params.RedirectURI, map[string]string{"error": "access_denied", "state": params.State})
	}
	if params.ResponseType == "token" {
		if err := s.enforceOAuthTokenUserRateLimit(c, user.ID); err != nil {
			return err
		}
		result, err := s.Store.CreateOAuthImplicitToken(c.Request().Context(), user, app, params.Scopes, oauthImplicitAccessTokenTTL)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, oauthError("server_error"))
		}
		return redirectWithOAuthFragment(c, params.RedirectURI, oauthTokenFragment(result, params.State))
	}
	code, err := s.Store.CreateOAuthAuthorizationCode(c.Request().Context(), user.ID, app.ID, params.RedirectURI, params.Scopes)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, oauthError("server_error"))
	}
	return redirectWithOAuthParams(c, params.RedirectURI, map[string]string{"code": code, "state": params.State})
}

func (s *Server) oauthToken(c echo.Context) error {
	clientID, secret := oauthClientCredentials(c.Request())
	if clientID == "" || secret == "" {
		return c.JSON(http.StatusUnauthorized, oauthError("invalid_client"))
	}
	app, err := s.Store.VerifyOAuthAppSecret(c.Request().Context(), clientID, secret)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, oauthError("invalid_client"))
	}

	var result db.OAuthTokenResult
	switch c.FormValue("grant_type") {
	case "authorization_code":
		code := strings.TrimSpace(c.FormValue("code"))
		redirectURI := strings.TrimSpace(c.FormValue("redirect_uri"))
		if code == "" || redirectURI == "" {
			return c.JSON(http.StatusBadRequest, oauthError("invalid_request"))
		}
		userID, err := s.Store.OAuthAuthorizationCodeUserID(c.Request().Context(), app.ClientID, code, redirectURI)
		if err != nil {
			return c.JSON(http.StatusBadRequest, oauthError("invalid_grant"))
		}
		if err := s.enforceOAuthTokenUserRateLimit(c, userID); err != nil {
			return err
		}
		result, err = s.Store.ExchangeOAuthAuthorizationCode(c.Request().Context(), app.ClientID, code, redirectURI, oauthAccessTokenTTL)
	case "refresh_token":
		refreshToken := strings.TrimSpace(c.FormValue("refresh_token"))
		if refreshToken == "" {
			return c.JSON(http.StatusBadRequest, oauthError("invalid_request"))
		}
		userID, err := s.Store.OAuthRefreshTokenUserID(c.Request().Context(), app.ClientID, refreshToken)
		if err != nil {
			return c.JSON(http.StatusBadRequest, oauthError("invalid_grant"))
		}
		if err := s.enforceOAuthTokenUserRateLimit(c, userID); err != nil {
			return err
		}
		result, err = s.Store.RefreshOAuthToken(c.Request().Context(), app.ClientID, refreshToken, oauthAccessTokenTTL)
	default:
		return c.JSON(http.StatusBadRequest, oauthError("unsupported_grant_type"))
	}
	if err != nil {
		return c.JSON(http.StatusBadRequest, oauthError("invalid_grant"))
	}
	return c.JSON(http.StatusOK, oauthTokenBody(result))
}

func (s *Server) oauthRevoke(c echo.Context) error {
	clientID, secret := oauthClientCredentials(c.Request())
	if clientID == "" || secret == "" {
		return c.JSON(http.StatusUnauthorized, oauthError("invalid_client"))
	}
	if _, err := s.Store.VerifyOAuthAppSecret(c.Request().Context(), clientID, secret); err != nil {
		return c.JSON(http.StatusUnauthorized, oauthError("invalid_client"))
	}
	token := strings.TrimSpace(c.FormValue("token"))
	if token == "" {
		return c.JSON(http.StatusBadRequest, oauthError("invalid_request"))
	}
	if err := s.Store.RevokeOAuthToken(c.Request().Context(), clientID, token); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusInternalServerError, oauthError("server_error"))
	}
	return c.JSON(http.StatusOK, map[string]any{"revoked": true})
}

func (s *Server) devSeed(c echo.Context) error {
	if !s.Config.DevSeedEnabled {
		return c.JSON(http.StatusNotFound, errorBody("dev seed is disabled"))
	}
	githubID := int64(1)
	if rawID := strings.TrimSpace(c.QueryParam("github_id")); rawID != "" {
		parsed, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || parsed <= 0 {
			return c.JSON(http.StatusBadRequest, errorBody("github_id must be a positive integer"))
		}
		githubID = parsed
	}
	username := strings.TrimSpace(c.QueryParam("username"))
	if username == "" {
		username = "local-dev"
	}
	if err := s.ensureGitHubRegistrationAllowed(c.Request().Context(), githubID); err != nil {
		if errors.Is(err, errRegistrationClosed) || errors.Is(err, errMaxUsersReached) {
			return c.JSON(http.StatusForbidden, errorBody(err.Error()))
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	user, err := s.Store.UpsertGitHubUser(c.Request().Context(), db.GitHubProfile{
		ID: githubID, Username: username, Email: username + "@example.com", FullName: "Local Developer",
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	apiKey, raw, err := s.Store.CreateAPIKey(c.Request().Context(), user.ID, "Local WakaTime")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	if err := s.setSessionCookie(c, user.ID); err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	accessToken, err := s.sessionJWT(user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusCreated, map[string]any{"data": map[string]any{"user": user, "api_key": raw, "access_token": accessToken, "token_type": "Bearer", "key": apiKey}})
}

func (s *Server) devHeartbeatsPurge(c echo.Context) error {
	if !s.Config.DevSeedEnabled {
		return c.JSON(http.StatusNotFound, errorBody("dev jobs are disabled"))
	}
	retentionDays := s.Config.HeartbeatRetentionDays
	if value := strings.TrimSpace(c.QueryParam("retention_days")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody("retention_days must be an integer"))
		}
		retentionDays = parsed
	}
	if err := s.enqueueHeartbeatsPurge(c.Request().Context(), retentionDays); err != nil {
		if errors.Is(err, jobs.ErrQueueUnavailable) {
			deleted, err := s.runHeartbeatsPurge(c.Request().Context(), retentionDays)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
			}
			return c.JSON(http.StatusOK, map[string]any{"data": map[string]any{"queued": false, "deleted": deleted}})
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusAccepted, map[string]any{"data": map[string]any{"queued": true}})
}

func (s *Server) devLeaderboardUpdate(c echo.Context) error {
	if !s.Config.DevSeedEnabled {
		return c.JSON(http.StatusNotFound, errorBody("dev jobs are disabled"))
	}
	rangeName := strings.TrimSpace(c.QueryParam("range"))
	if rangeName == "" {
		rangeName = "last_7_days"
	}
	if _, err := services.WindowForRange(time.Now(), rangeName); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	if err := s.enqueueLeaderboardUpdate(c.Request().Context(), rangeName); err != nil {
		if errors.Is(err, jobs.ErrQueueUnavailable) {
			entries, err := s.refreshLeaderboardCache(c.Request().Context(), rangeName, "", "")
			if err != nil {
				return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
			}
			return c.JSON(http.StatusOK, map[string]any{"data": map[string]any{"queued": false, "range": rangeName, "entries": len(entries)}})
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusAccepted, map[string]any{"data": map[string]any{"queued": true, "range": rangeName}})
}

func (s *Server) devGoalsEvaluate(c echo.Context) error {
	if !s.Config.DevSeedEnabled {
		return c.JSON(http.StatusNotFound, errorBody("dev jobs are disabled"))
	}
	now := time.Time{}
	if value := strings.TrimSpace(c.QueryParam("now_unix")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed <= 0 {
			return c.JSON(http.StatusBadRequest, errorBody("now_unix must be a positive unix timestamp"))
		}
		now = time.Unix(parsed, 0).UTC()
	}
	if err := s.enqueueGoalsEvaluate(c.Request().Context(), now); err != nil {
		if errors.Is(err, jobs.ErrQueueUnavailable) {
			evaluated, err := s.runGoalsEvaluate(c.Request().Context(), now)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
			}
			return c.JSON(http.StatusOK, map[string]any{"data": map[string]any{"queued": false, "evaluated": evaluated}})
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusAccepted, map[string]any{"data": map[string]any{"queued": true}})
}

func (s *Server) currentUser(c echo.Context) error {
	user := userFromContext(c)
	authInfo, _ := c.Get("auth").(authContext)
	return c.JSON(http.StatusOK, map[string]any{"data": currentUserResponse(user, authInfo)})
}

func currentUserResponse(user db.User, authInfo authContext) db.User {
	if authInfo.Kind == "session" || authInfo.HasScope("email") {
		return user
	}
	user.Email = ""
	return user
}

func (s *Server) updateCurrentUser(c echo.Context) error {
	user := userFromContext(c)
	var payload struct {
		Timezone                string           `json:"timezone"`
		TimeoutMinutes          int              `json:"timeout_minutes"`
		WritesOnly              bool             `json:"writes_only"`
		HasPublicProfile        bool             `json:"has_public_profile"`
		Country                 string           `json:"country"`
		HeartbeatRetentionDays  int              `json:"heartbeat_retention_days"`
		PublicUsername          string           `json:"public_username"`
		PublicDisplayName       string           `json:"public_display_name"`
		PublicGitHubLink        bool             `json:"public_github_link_enabled"`
		PublicShowTotalTime     bool             `json:"public_show_total_time"`
		PublicShowProjects      bool             `json:"public_show_projects"`
		PublicProjectVisibility string           `json:"public_project_visibility"`
		PublicShowLanguages     bool             `json:"public_show_languages"`
		PublicShowEditors       bool             `json:"public_show_editors"`
		PublicShowMachines      bool             `json:"public_show_machines"`
		PublicShowOS            bool             `json:"public_show_operating_systems"`
		PublicShowCategories    bool             `json:"public_show_categories"`
		PublicShowAI            bool             `json:"public_show_ai"`
		PublicShowSummaries     bool             `json:"public_show_summaries"`
		PublicProfile           db.PublicProfile `json:"public_profile"`
	}
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	input := db.UserSettingsInput{
		Timezone:                payload.Timezone,
		TimeoutMinutes:          payload.TimeoutMinutes,
		WritesOnly:              payload.WritesOnly,
		HasPublicProfile:        payload.HasPublicProfile,
		Country:                 payload.Country,
		HeartbeatRetentionDays:  payload.HeartbeatRetentionDays,
		PublicUsername:          payload.PublicUsername,
		PublicDisplayName:       payload.PublicDisplayName,
		PublicGitHubLink:        payload.PublicGitHubLink,
		PublicShowTotalTime:     payload.PublicShowTotalTime,
		PublicShowProjects:      payload.PublicShowProjects,
		PublicProjectVisibility: payload.PublicProjectVisibility,
		PublicShowLanguages:     payload.PublicShowLanguages,
		PublicShowEditors:       payload.PublicShowEditors,
		PublicShowMachines:      payload.PublicShowMachines,
		PublicShowOS:            payload.PublicShowOS,
		PublicShowCategories:    payload.PublicShowCategories,
		PublicShowAI:            payload.PublicShowAI,
		PublicShowSummaries:     payload.PublicShowSummaries,
		PublicProfile:           payload.PublicProfile,
	}
	if err := db.ValidateUserSettings(input); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	updated, err := s.Store.UpdateUser(c.Request().Context(), user.ID, input)
	if errors.Is(err, db.ErrDuplicatePublicUsername) {
		return c.JSON(http.StatusConflict, errorBody("public username is already taken"))
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	s.enqueueStatsRecompute(c.Request().Context(), user.ID)
	return c.JSON(http.StatusOK, map[string]any{"data": updated})
}

func (s *Server) deleteCurrentUser(c echo.Context) error {
	user := userFromContext(c)
	var payload struct {
		Confirmation string `json:"confirmation"`
	}
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	if !services.AccountDeletionConfirmed(payload.Confirmation) {
		return c.JSON(http.StatusBadRequest, errorBody("confirmation must be DELETE"))
	}
	if err := s.Store.DeleteUser(c.Request().Context(), user.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, errorBody("user not found"))
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	clearSessionCookies(c)
	return c.JSON(http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}

func (s *Server) createHeartbeat(c echo.Context) error {
	user := userFromContext(c)
	var heartbeat services.Heartbeat
	if err := c.Bind(&heartbeat); err != nil {
		return c.JSON(http.StatusBadRequest, wakaError("invalid heartbeat JSON"))
	}
	prepHeartbeat(&heartbeat, c.Request().UserAgent())
	var deleted bool
	var err error
	heartbeat, deleted, err = s.applyCustomRules(c.Request().Context(), user, heartbeat)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, wakaError(err.Error()))
	}
	if deleted {
		return c.JSON(http.StatusAccepted, map[string]any{"data": heartbeat, "deleted_by_rule": true})
	}
	if err := validateHeartbeat(heartbeat); err != nil {
		return c.JSON(http.StatusBadRequest, wakaError(err.Error()))
	}
	stored, err := s.Store.InsertHeartbeat(c.Request().Context(), user.ID, heartbeat)
	if errors.Is(err, db.ErrDuplicateHeartbeat) {
		return c.JSON(http.StatusAccepted, map[string]any{"data": heartbeat, "duplicate": true})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, wakaError(err.Error()))
	}
	s.enqueueStatsRecompute(c.Request().Context(), user.ID)
	return c.JSON(http.StatusAccepted, map[string]any{"data": stored})
}

func (s *Server) listHeartbeats(c echo.Context) error {
	user := userFromContext(c)
	startDate, endDate, err := dayRangeInLocation(c.QueryParam("date"), userLocation(user), time.Now())
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	heartbeats, err := s.Store.HeartbeatsBetween(c.Request().Context(), user.ID, float64(startDate.Unix()), float64(endDate.Unix()))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(heartbeats))
}

func (s *Server) createHeartbeatsBulk(c echo.Context) error {
	user := userFromContext(c)
	var heartbeats []services.Heartbeat
	if err := c.Bind(&heartbeats); err != nil {
		return c.JSON(http.StatusBadRequest, wakaError("invalid bulk heartbeat JSON"))
	}
	if len(heartbeats) > 25 {
		return c.JSON(http.StatusBadRequest, wakaError("bulk heartbeat limit is 25"))
	}
	responses := make([]any, 0, len(heartbeats))
	for i := range heartbeats {
		heartbeat := heartbeats[i]
		prepHeartbeat(&heartbeat, c.Request().UserAgent())
		var deleted bool
		var err error
		heartbeat, deleted, err = s.applyCustomRules(c.Request().Context(), user, heartbeat)
		if err != nil {
			responses = append(responses, heartbeatBulkError(http.StatusInternalServerError, err.Error()))
			continue
		}
		if deleted {
			responses = append(responses, heartbeatBulkResult(http.StatusAccepted, map[string]any{"data": heartbeat, "deleted_by_rule": true}))
			continue
		}
		if err := validateHeartbeat(heartbeat); err != nil {
			responses = append(responses, heartbeatBulkError(http.StatusBadRequest, err.Error()))
			continue
		}
		stored, err := s.Store.InsertHeartbeat(c.Request().Context(), user.ID, heartbeat)
		switch {
		case errors.Is(err, db.ErrDuplicateHeartbeat):
			responses = append(responses, heartbeatBulkResult(http.StatusAccepted, map[string]any{"data": heartbeat, "duplicate": true}))
		case err != nil:
			responses = append(responses, heartbeatBulkError(http.StatusInternalServerError, err.Error()))
		default:
			s.enqueueStatsRecompute(c.Request().Context(), user.ID)
			responses = append(responses, heartbeatBulkResult(http.StatusCreated, map[string]any{"data": stored}))
		}
	}
	return c.JSON(http.StatusAccepted, map[string]any{"responses": responses})
}

func heartbeatBulkResult(status int, body map[string]any) []any {
	return []any{body, status}
}

func heartbeatBulkError(status int, message string) []any {
	return heartbeatBulkResult(status, map[string]any{"error": message})
}

func (s *Server) deleteHeartbeatsBulk(c echo.Context) error {
	user := userFromContext(c)
	var payload struct {
		Date string   `json:"date"`
		IDs  []string `json:"ids"`
	}
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	start, end, err := dayRange(payload.Date)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	ids := make([]uuid.UUID, 0, len(payload.IDs))
	for _, id := range payload.IDs {
		parsed, err := uuid.Parse(id)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody("invalid heartbeat id"))
		}
		ids = append(ids, parsed)
	}
	deleted, err := s.Store.DeleteHeartbeatsByID(c.Request().Context(), user.ID, ids, start, end)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	if deleted > 0 {
		s.enqueueStatsRecompute(c.Request().Context(), user.ID)
	}
	return c.JSON(http.StatusOK, map[string]any{"data": map[string]any{"deleted": deleted}})
}

func (s *Server) fileExperts(c echo.Context) error {
	user := userFromContext(c)
	var payload struct {
		Entity  string  `json:"entity"`
		Project *string `json:"project"`
	}
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, wakaError("invalid file experts JSON"))
	}
	entity := strings.TrimSpace(payload.Entity)
	if entity == "" {
		return c.JSON(http.StatusBadRequest, wakaError("entity is required"))
	}
	heartbeats, err := s.Store.AllHeartbeats(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
	totalSeconds := fileExpertsTotalSeconds(heartbeats, entity, payload.Project, time.Duration(user.TimeoutMinutes)*time.Minute)
	return c.JSON(http.StatusOK, wakaTimeFileExpertsPayload(user, totalSeconds))
}

func fileExpertsTotalSeconds(heartbeats []services.Heartbeat, entity string, project *string, timeout time.Duration) int {
	filtered := make([]services.Heartbeat, 0, len(heartbeats))
	projectName := ""
	if project != nil {
		projectName = strings.TrimSpace(*project)
	}
	for _, heartbeat := range heartbeats {
		if heartbeat.Entity != entity {
			continue
		}
		if projectName != "" && heartbeat.Project != projectName {
			continue
		}
		filtered = append(filtered, heartbeat)
	}
	totalSeconds := 0
	for _, duration := range services.ComputeDurations(filtered, timeout, "entity") {
		totalSeconds += duration.DurationSeconds
	}
	return totalSeconds
}

func (s *Server) durations(c echo.Context) error {
	user := userFromContext(c)
	startDate, endDate, err := dayRangeInLocation(c.QueryParam("date"), userLocation(user), time.Now())
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	heartbeats, err := s.Store.HeartbeatsBetween(c.Request().Context(), user.ID, float64(startDate.Unix()), float64(endDate.Unix()))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
	externalRows, err := s.Store.ExternalDurationsBetween(c.Request().Context(), user.ID, startDate, endDate)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	sliceBy := c.QueryParam("slice_by")
	if sliceBy == "" {
		sliceBy = "project"
	}
	rows := append(
		services.ComputeDurations(heartbeats, time.Duration(user.TimeoutMinutes)*time.Minute, sliceBy),
		services.ExternalDurationsInWindow(toServiceExternalDurations(externalRows), sliceBy, startDate, endDate)...,
	)
	return c.JSON(http.StatusOK, dataArray(rows))
}

func (s *Server) summaries(c echo.Context) error {
	user := userFromContext(c)
	startDate, endDate, err := dateRangeInLocation(c.QueryParam("start"), c.QueryParam("end"), userLocation(user), time.Now())
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	heartbeats, err := s.Store.HeartbeatsBetween(c.Request().Context(), user.ID, float64(startDate.Unix()), float64(endDate.AddDate(0, 0, 1).Unix()))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
	externalRows, err := s.Store.ExternalDurationsBetween(c.Request().Context(), user.ID, startDate, endDate.AddDate(0, 0, 1))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	external := toServiceExternalDurations(externalRows)
	authInfo, _ := c.Get("auth").(authContext)
	return c.JSON(http.StatusOK, map[string]any{
		"data": summaryRowsForRange(heartbeats, external, startDate, endDate, time.Duration(user.TimeoutMinutes)*time.Minute, summaryFieldsForAuth(authInfo)),
	})
}

func (s *Server) last7DaysStats(c echo.Context) error {
	return s.writeStatsForRange(c, "last_7_days")
}

func (s *Server) allStats(c echo.Context) error {
	user := userFromContext(c)
	data := make(map[string]services.Stats, len(jobs.DefaultStatsRanges()))
	status := http.StatusOK
	for _, rangeName := range jobs.DefaultStatsRanges() {
		stats, err := s.statsForResponse(c.Request().Context(), user, rangeName)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
		}
		if statsResponseStatus(stats) == http.StatusAccepted {
			status = http.StatusAccepted
		}
		data[rangeName] = stats
	}
	return c.JSON(status, map[string]any{"data": data})
}

func (s *Server) statsForRange(c echo.Context) error {
	return s.writeStatsForRange(c, c.Param("range"))
}

func (s *Server) writeStatsForRange(c echo.Context, rangeName string) error {
	user := userFromContext(c)
	if rangeName == "" {
		return c.JSON(http.StatusBadRequest, errorBody("unsupported stats range"))
	}
	if rangeName != "all_time" {
		if _, err := services.WindowForRange(time.Now().In(userLocation(user)), rangeName); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
		}
	}
	stats, err := s.statsForResponse(c.Request().Context(), user, rangeName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(statsResponseStatus(stats), map[string]any{"data": stats})
}

func statsResponseStatus(stats services.Stats) int {
	if !stats.IsUpToDate {
		return http.StatusAccepted
	}
	return http.StatusOK
}

func (s *Server) statsForResponse(ctx context.Context, user db.User, rangeName string) (services.Stats, error) {
	cached, found, err := s.Store.StatsCache(ctx, user.ID, rangeName)
	if err != nil {
		return services.Stats{}, err
	}
	if found {
		if !cached.IsUpToDate {
			s.enqueueStatsRecompute(ctx, user.ID)
		}
		return cached, nil
	}
	return s.computeFreshStatsForRange(ctx, user, rangeName)
}

func (s *Server) computeStatsForRange(ctx context.Context, user db.User, rangeName string) (services.Stats, error) {
	cached, found, err := s.Store.StatsCache(ctx, user.ID, rangeName)
	if err != nil {
		return services.Stats{}, err
	}
	if found && cached.IsUpToDate {
		return cached, nil
	}
	return s.computeFreshStatsForRange(ctx, user, rangeName)
}

func (s *Server) computeFreshStatsForRange(ctx context.Context, user db.User, rangeName string) (services.Stats, error) {
	if rangeName == "all_time" {
		heartbeats, err := s.Store.AllHeartbeats(ctx, user.ID)
		if err != nil {
			return services.Stats{}, err
		}
		heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
		externalRows, err := s.Store.ListExternalDurations(ctx, user.ID)
		if err != nil {
			return services.Stats{}, err
		}
		costs, err := s.Store.AICostRates(ctx, user.ID)
		if err != nil {
			return services.Stats{}, err
		}
		stats := services.ComputeAllTimeStatsWithExternalDurationsAndAICosts(heartbeats, toServiceExternalDurations(externalRows), time.Duration(user.TimeoutMinutes)*time.Minute, costs)
		aicostbake.Bake(ctx, s.Store, s.Pricing, user.ID, userLocation(user), rangeName, &stats)
		if err := s.Store.UpsertStatsCache(ctx, user.ID, rangeName, stats); err != nil {
			return services.Stats{}, err
		}
		return stats, nil
	}

	now := time.Now().In(userLocation(user))
	heartbeats, err := s.Store.HeartbeatsForStatsRange(ctx, user.ID, now, rangeName)
	if err != nil {
		return services.Stats{}, err
	}
	heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
	window, err := services.WindowForRange(now, rangeName)
	if err != nil {
		return services.Stats{}, err
	}
	externalRows, err := s.Store.ExternalDurationsBetween(ctx, user.ID, window.Start, window.End)
	if err != nil {
		return services.Stats{}, err
	}
	costs, err := s.Store.AICostRates(ctx, user.ID)
	if err != nil {
		return services.Stats{}, err
	}
	stats, _, err := services.ComputeStatsForRangeWithExternalDurationsAndAICosts(heartbeats, toServiceExternalDurations(externalRows), now, time.Duration(user.TimeoutMinutes)*time.Minute, rangeName, costs)
	if err != nil {
		return services.Stats{}, err
	}
	aicostbake.Bake(ctx, s.Store, s.Pricing, user.ID, userLocation(user), rangeName, &stats)
	if err := s.Store.UpsertStatsCache(ctx, user.ID, rangeName, stats); err != nil {
		return services.Stats{}, err
	}
	return stats, nil
}

func (s *Server) statusBarToday(c echo.Context) error {
	user := userFromContext(c)
	status, cached, err := s.statusBarTodayForUser(c.Request().Context(), user)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	body := map[string]any{"data": status}
	if cached {
		body["cached"] = true
	}
	return c.JSON(http.StatusOK, body)
}

func (s *Server) statusBarTodayWakaTime(c echo.Context) error {
	user := userFromContext(c)
	status, _, err := s.statusBarTodayForUser(c.Request().Context(), user)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, wakaTimeStatusBarPayload(status, user, time.Now()))
}

func (s *Server) statusBarTodayForUser(ctx context.Context, user db.User) (services.StatusBarStats, bool, error) {
	now := time.Now()
	cacheKey := statusCacheKey(user, now)
	if s.StatusCache != nil {
		if cached, ok, err := s.StatusCache.Get(ctx, cacheKey); err == nil && ok {
			return cached, true, nil
		}
	}
	location := userLocation(user)
	localNow := now.In(location)
	start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 1)
	heartbeats, err := s.Store.HeartbeatsBetween(ctx, user.ID, float64(start.Unix()), float64(end.Unix()))
	if err != nil {
		return services.StatusBarStats{}, false, err
	}
	heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
	externalRows, err := s.Store.ExternalDurationsBetween(ctx, user.ID, start, end)
	if err != nil {
		return services.StatusBarStats{}, false, err
	}
	status := services.ComputeStatusBarForWindowWithExternalDurations(heartbeats, toServiceExternalDurations(externalRows), start, end, time.Duration(user.TimeoutMinutes)*time.Minute)
	if s.StatusCache != nil {
		_ = s.StatusCache.Set(ctx, cacheKey, status, 2*time.Minute)
	}
	return status, false, nil
}

func statusCacheKey(user db.User, now time.Time) string {
	location := userLocation(user)
	localDate := now.In(location).Format("2006-01-02")
	return fmt.Sprintf("%s:timezone:%s:timeout:%d:writes_only:%t:date:%s", user.ID.String(), location.String(), user.TimeoutMinutes, user.WritesOnly, localDate)
}

func wakaTimeStatusBarPayload(status services.StatusBarStats, user db.User, now time.Time) map[string]any {
	location := time.UTC
	if user.Timezone != "" {
		if loaded, err := time.LoadLocation(user.Timezone); err == nil {
			location = loaded
		}
	}
	localNow := now.In(location)
	start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 1).Add(-time.Second)
	data := map[string]any{
		"categories": []map[string]any{
			wakaTimeCounter("Coding", status.TotalSeconds, status.GrandTotalText, 100),
		},
		"grand_total": wakaTimeGrandTotal(status.TotalSeconds, status.GrandTotalText),
		"projects":    []map[string]any{},
		"languages":   []map[string]any{},
		"range": map[string]any{
			"date":     start.Format("2006-01-02"),
			"start":    start.Format(time.RFC3339),
			"end":      end.Format(time.RFC3339),
			"text":     start.Format("Mon Jan 2 2006"),
			"timezone": location.String(),
		},
	}
	if status.Project != "" {
		data["projects"] = []map[string]any{wakaTimeCounter(status.Project, status.ProjectSeconds, status.ProjectText, 100)}
	}
	if status.Language != "" {
		data["languages"] = []map[string]any{wakaTimeCounter(status.Language, status.LanguageSeconds, status.LanguageText, 100)}
	}
	return map[string]any{
		"cached_at":          now.UTC().Format(time.RFC3339),
		"data":               data,
		"has_team_features":  false,
		"percent_calculated": status.PercentCalculated,
	}
}

func wakaTimeGrandTotal(totalSeconds int, text string) map[string]any {
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	return map[string]any{
		"decimal":       fmt.Sprintf("%.2f", float64(totalSeconds)/3600),
		"digital":       fmt.Sprintf("%d:%02d", hours, minutes),
		"hours":         hours,
		"minutes":       minutes,
		"seconds":       seconds,
		"text":          text,
		"total_seconds": float64(totalSeconds),
	}
}

func wakaTimeCounter(name string, totalSeconds int, text string, percent float64) map[string]any {
	counter := wakaTimeGrandTotal(totalSeconds, text)
	counter["digital"] = fmt.Sprintf("%d:%02d:%02d", totalSeconds/3600, (totalSeconds%3600)/60, totalSeconds%60)
	counter["name"] = name
	counter["percent"] = percent
	return counter
}

func wakaTimeGoalPayload(progress services.GoalProgress, user db.User, now time.Time) map[string]any {
	location := time.UTC
	if user.Timezone != "" {
		if loaded, err := time.LoadLocation(user.Timezone); err == nil {
			location = loaded
		}
	}
	localNow := now.In(location)
	start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 1).Add(-time.Second)
	rangePayload := map[string]any{
		"date":     start.Format("2006-01-02"),
		"start":    start.Format(time.RFC3339),
		"end":      end.Format(time.RFC3339),
		"text":     start.Format("Mon Jan 2 2006"),
		"timezone": location.String(),
	}
	status, reason := wakaTimeGoalStatus(progress)
	customTitle := any(nil)
	if progress.Goal.CustomTitle != "" {
		customTitle = progress.Goal.CustomTitle
	}
	modifiedAt := any(nil)
	if progress.Goal.ModifiedAt != "" {
		modifiedAt = progress.Goal.ModifiedAt
	}
	snoozeUntil := any(nil)
	if progress.Goal.SnoozeUntil != "" {
		snoozeUntil = progress.Goal.SnoozeUntil
	}
	return map[string]any{
		"cached_at": now.UTC().Format(time.RFC3339),
		"data": map[string]any{
			"average_status":            status,
			"chart_data":                []map[string]any{wakaTimeGoalChartData(progress, rangePayload, status, reason)},
			"created_at":                progress.Goal.CreatedAt,
			"cumulative_status":         status,
			"custom_title":              customTitle,
			"delta":                     progress.Goal.Delta,
			"editors":                   progress.Goal.Editors,
			"id":                        progress.Goal.ID,
			"ignore_days":               progress.Goal.IgnoreDays,
			"ignore_zero_days":          progress.Goal.IgnoreZeroDays,
			"improve_by_percent":        progress.Goal.ImproveByPercent,
			"is_current_user_owner":     true,
			"is_enabled":                progress.Goal.IsEnabled,
			"is_inverse":                progress.Goal.IsInverse,
			"is_snoozed":                progress.IsSnoozed,
			"is_tweeting":               false,
			"languages":                 progress.Goal.Languages,
			"modified_at":               modifiedAt,
			"owner":                     wakaTimeGoalOwner(user),
			"projects":                  progress.Goal.Projects,
			"range_text":                progress.Goal.Delta,
			"seconds":                   progress.Goal.Seconds,
			"shared_with":               []string{},
			"snooze_until":              snoozeUntil,
			"status":                    status,
			"status_percent_calculated": 100,
			"subscribers":               []map[string]any{},
			"title":                     progress.Goal.Title,
			"type":                      progress.Goal.Delta,
			"stint_progress":            progress,
			"actual_seconds":            progress.ActualSeconds,
			"actual_seconds_text":       progress.HumanReadable,
			"goal_seconds":              progress.TargetSeconds,
			"goal_seconds_text":         progress.TargetReadable,
			"remaining_seconds":         progress.RemainingSeconds,
			"percent":                   progress.Percent,
			"is_complete":               progress.IsComplete,
			"range_status":              status,
			"range_status_reason":       reason,
			"range_status_reason_short": reason,
		},
	}
}

func wakaTimeGoalChartData(progress services.GoalProgress, rangePayload map[string]any, status, reason string) map[string]any {
	return map[string]any{
		"actual_seconds":            float64(progress.ActualSeconds),
		"actual_seconds_text":       progress.HumanReadable,
		"goal_seconds":              progress.TargetSeconds,
		"goal_seconds_text":         progress.TargetReadable,
		"range":                     rangePayload,
		"range_status":              status,
		"range_status_reason":       reason,
		"range_status_reason_short": reason,
		"remaining_seconds":         progress.RemainingSeconds,
		"percent":                   progress.Percent,
		"is_complete":               progress.IsComplete,
		"is_ignored":                progress.IsIgnored,
		"is_snoozed":                progress.IsSnoozed,
	}
}

func wakaTimeGoalStatus(progress services.GoalProgress) (string, string) {
	switch {
	case progress.IsSnoozed:
		return "snoozed", "goal snoozed"
	case progress.IsIgnored:
		return "ignored", "ignored by goal settings"
	case progress.IsComplete:
		return "success", "goal reached"
	default:
		return "pending", ""
	}
}

func wakaTimeGoalOwner(user db.User) map[string]any {
	display := user.FullName
	if display == "" {
		display = user.GitHubUsername
	}
	return map[string]any{
		"display_name": display,
		"email":        nil,
		"full_name":    user.FullName,
		"id":           user.ID.String(),
		"photo":        user.AvatarURL,
		"username":     user.GitHubUsername,
	}
}

func wakaTimeFileExpertsPayload(user db.User, totalSeconds int) map[string]any {
	name := user.FullName
	if name == "" {
		name = user.GitHubUsername
	}
	return map[string]any{
		"data": []map[string]any{
			{
				"total": wakaTimeFileExpertsTotal(totalSeconds),
				"user": map[string]any{
					"id":              user.ID.String(),
					"is_current_user": true,
					"long_name":       name,
					"name":            fileExpertShortName(name),
				},
			},
		},
	}
}

func wakaTimeFileExpertsTotal(totalSeconds int) map[string]any {
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	return map[string]any{
		"decimal":       fmt.Sprintf("%.2f", float64(totalSeconds)/3600),
		"digital":       fmt.Sprintf("%d:%02d", hours, minutes),
		"text":          services.HumanDuration(totalSeconds),
		"total_seconds": float64(totalSeconds),
	}
}

func fileExpertShortName(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return name
	}
	return fields[0]
}

func (s *Server) allTimeSinceToday(c echo.Context) error {
	user := userFromContext(c)
	heartbeats, err := s.Store.AllHeartbeats(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
	externalRows, err := s.Store.ListExternalDurations(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	stats := services.ComputeAllTimeStatsWithExternalDurations(heartbeats, toServiceExternalDurations(externalRows), time.Duration(user.TimeoutMinutes)*time.Minute)
	return c.JSON(http.StatusOK, map[string]any{
		"data": map[string]any{
			"total_seconds": stats.TotalSeconds,
			"text":          stats.HumanReadableTotal,
			"stats":         stats,
		},
	})
}

func (s *Server) listProjects(c echo.Context) error {
	user := userFromContext(c)
	projects, err := s.Store.ListProjects(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(projects))
}

func (s *Server) projectDetail(c echo.Context) error {
	user := userFromContext(c)
	name, err := url.PathUnescape(c.Param("project"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid project name"))
	}
	rangeName, err := projectDetailRange(c.QueryParam("range"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	project, err := s.Store.GetProject(c.Request().Context(), user.ID, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusNotFound, errorBody("project not found"))
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	now := time.Now()
	var heartbeats []services.Heartbeat
	var externalRows []db.ExternalDuration
	if rangeName == "all_time" {
		heartbeats, err = s.Store.AllHeartbeats(c.Request().Context(), user.ID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
		}
		externalRows, err = s.Store.ListExternalDurations(c.Request().Context(), user.ID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
		}
	} else {
		heartbeats, err = s.Store.HeartbeatsForStatsRange(c.Request().Context(), user.ID, now, rangeName)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
		}
		window, err := services.WindowForRange(now, rangeName)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
		}
		externalRows, err = s.Store.ExternalDurationsBetween(c.Request().Context(), user.ID, window.Start, window.End)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
		}
	}
	heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
	filtered := make([]services.Heartbeat, 0, len(heartbeats))
	for _, heartbeat := range heartbeats {
		if heartbeat.Project == project.Name {
			filtered = append(filtered, heartbeat)
		}
	}
	projectExternal := []services.ExternalDuration{}
	for _, duration := range toServiceExternalDurations(externalRows) {
		if duration.Project == project.Name {
			projectExternal = append(projectExternal, duration)
		}
	}
	costs, err := s.Store.AICostRates(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	var stats services.Stats
	if rangeName == "all_time" {
		stats = services.ComputeAllTimeStatsWithExternalDurationsAndAICosts(filtered, projectExternal, time.Duration(user.TimeoutMinutes)*time.Minute, costs)
	} else {
		stats, _, err = services.ComputeStatsForRangeWithExternalDurationsAndAICosts(filtered, projectExternal, now, time.Duration(user.TimeoutMinutes)*time.Minute, rangeName, costs)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"data": map[string]any{"project": project, "stats": stats}})
}

func projectDetailRange(input string) (string, error) {
	rangeName := strings.TrimSpace(input)
	if rangeName == "" {
		return "last_30_days", nil
	}
	if rangeName == "all_time" {
		return rangeName, nil
	}
	if _, err := services.WindowForRange(time.Now(), rangeName); err != nil {
		return "", err
	}
	return rangeName, nil
}

func (s *Server) projectCommits(c echo.Context) error {
	user := userFromContext(c)
	project, commits, err := s.projectCommitRows(c, user)
	if err != nil {
		return err
	}
	page := positiveQueryInt(c, "page", 1)
	const perPage = 100
	total := len(commits)
	totalPages := 0
	if total > 0 {
		totalPages = (total + perPage - 1) / perPage
	}
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	var prevPage any
	var prevPageURL any
	if page > 1 {
		prevPage = page - 1
		prevPageURL = projectCommitPageURL(project.Name, c.QueryParam("branch"), page-1)
	}
	var nextPage any
	var nextPageURL any
	if page < totalPages {
		nextPage = page + 1
		nextPageURL = projectCommitPageURL(project.Name, c.QueryParam("branch"), page+1)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"commits":       commits[start:end],
		"author":        nil,
		"branch":        c.QueryParam("branch"),
		"next_page":     nextPage,
		"next_page_url": nextPageURL,
		"page":          page,
		"prev_page":     prevPage,
		"prev_page_url": prevPageURL,
		"project":       commitProjectPayload(project),
		"status":        "ok",
		"total":         total,
		"total_pages":   totalPages,
	})
}

func projectCommitPageURL(project, branch string, page int) string {
	path := "/api/v1/users/current/projects/" + url.PathEscape(project) + "/commits"
	query := url.Values{}
	if strings.TrimSpace(branch) != "" {
		query.Set("branch", branch)
	}
	if page > 1 {
		query.Set("page", strconv.Itoa(page))
	}
	if encoded := query.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func (s *Server) projectCommit(c echo.Context) error {
	user := userFromContext(c)
	project, commits, err := s.projectCommitRows(c, user)
	if err != nil {
		return err
	}
	hash := c.Param("hash")
	for _, commit := range commits {
		if commit.Hash == hash || commit.TruncatedHash == hash {
			return c.JSON(http.StatusOK, map[string]any{
				"commit":  commit,
				"branch":  commit.Branch,
				"project": commitProjectPayload(project),
				"status":  "ok",
			})
		}
	}
	return c.JSON(http.StatusNotFound, errorBody("commit not found"))
}

func (s *Server) projectCommitRows(c echo.Context, user db.User) (db.Project, []services.CommitSummary, error) {
	name, err := url.PathUnescape(c.Param("project"))
	if err != nil {
		return db.Project{}, nil, c.JSON(http.StatusBadRequest, errorBody("invalid project name"))
	}
	project, err := s.Store.GetProject(c.Request().Context(), user.ID, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Project{}, nil, c.JSON(http.StatusNotFound, errorBody("project not found"))
	}
	if err != nil {
		return db.Project{}, nil, c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	heartbeats, err := s.Store.AllHeartbeats(c.Request().Context(), user.ID)
	if err != nil {
		return db.Project{}, nil, c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
	commits := services.ComputeProjectCommits(heartbeats, project.Name, c.QueryParam("branch"), time.Duration(user.TimeoutMinutes)*time.Minute)
	return project, commits, nil
}

func commitProjectPayload(project db.Project) map[string]any {
	return map[string]any{
		"id":             project.ID.String(),
		"name":           project.Name,
		"color":          project.Color,
		"has_public_url": project.HasPublicURL,
		"badge":          project.Badge,
		"privacy":        "private",
		"repository":     nil,
	}
}

func positiveQueryInt(c echo.Context, name string, fallback int) int {
	value := strings.TrimSpace(c.QueryParam(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func (s *Server) listMachineNames(c echo.Context) error {
	user := userFromContext(c)
	machines, err := s.Store.ListMachineNames(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(machines))
}

func (s *Server) listUserAgents(c echo.Context) error {
	user := userFromContext(c)
	agents, err := s.Store.ListUserAgents(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(agents))
}

func (s *Server) listGoals(c echo.Context) error {
	user := userFromContext(c)
	goals, err := s.Store.ListGoals(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	heartbeats, err := s.Store.AllHeartbeats(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
	externalRows, err := s.Store.ListExternalDurations(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	external := toServiceExternalDurations(externalRows)
	progress := make([]services.GoalProgress, 0, len(goals))
	for _, goal := range goals {
		progress = append(progress, services.ComputeGoalProgressWithExternalDurations(toServiceGoal(goal), heartbeats, external, time.Now(), time.Duration(user.TimeoutMinutes)*time.Minute))
	}
	return c.JSON(http.StatusOK, dataArray(progress))
}

func (s *Server) createGoal(c echo.Context) error {
	user := userFromContext(c)
	var input db.GoalInput
	if err := c.Bind(&input); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	if err := db.ValidateGoalInput(input); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	goal, err := s.Store.CreateGoal(c.Request().Context(), user.ID, input)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusCreated, map[string]any{"data": toServiceGoal(goal)})
}

func (s *Server) getGoal(c echo.Context) error {
	user := userFromContext(c)
	goalID, err := uuid.Parse(c.Param("goal"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid goal id"))
	}
	goal, err := s.Store.GetGoal(c.Request().Context(), user.ID, goalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusNotFound, errorBody("goal not found"))
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	heartbeats, err := s.Store.AllHeartbeats(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
	externalRows, err := s.Store.ListExternalDurations(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	now := time.Now()
	progress := services.ComputeGoalProgressWithExternalDurations(toServiceGoal(goal), heartbeats, toServiceExternalDurations(externalRows), now, time.Duration(user.TimeoutMinutes)*time.Minute)
	return c.JSON(http.StatusOK, wakaTimeGoalPayload(progress, user, now))
}

func (s *Server) updateGoal(c echo.Context) error {
	user := userFromContext(c)
	goalID, err := uuid.Parse(c.Param("goal"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid goal id"))
	}
	var input db.GoalInput
	if err := c.Bind(&input); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	if err := db.ValidateGoalInput(input); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	goal, err := s.Store.UpdateGoal(c.Request().Context(), user.ID, goalID, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusNotFound, errorBody("goal not found"))
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, map[string]any{"data": toServiceGoal(goal)})
}

func (s *Server) deleteGoal(c echo.Context) error {
	user := userFromContext(c)
	goalID, err := uuid.Parse(c.Param("goal"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid goal id"))
	}
	if err := s.Store.DeleteGoal(c.Request().Context(), user.ID, goalID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, errorBody("goal not found"))
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) listExternalDurations(c echo.Context) error {
	user := userFromContext(c)
	durations, err := s.Store.ListExternalDurations(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(durations))
}

func (s *Server) createExternalDuration(c echo.Context) error {
	user := userFromContext(c)
	var input services.ExternalDuration
	if err := c.Bind(&input); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	if err := services.ValidateExternalDuration(input); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	duration, err := s.Store.UpsertExternalDuration(c.Request().Context(), user.ID, input)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	s.enqueueStatsRecompute(c.Request().Context(), user.ID)
	return c.JSON(http.StatusCreated, map[string]any{"data": duration})
}

func (s *Server) createExternalDurationsBulk(c echo.Context) error {
	user := userFromContext(c)
	var inputs []services.ExternalDuration
	if err := c.Bind(&inputs); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	if len(inputs) > 1000 {
		return c.JSON(http.StatusBadRequest, errorBody("bulk external duration limit is 1000"))
	}
	responses := make([]map[string]any, 0, len(inputs))
	for _, input := range inputs {
		if err := services.ValidateExternalDuration(input); err != nil {
			responses = append(responses, map[string]any{"status": http.StatusBadRequest, "error": err.Error()})
			continue
		}
		duration, err := s.Store.UpsertExternalDuration(c.Request().Context(), user.ID, input)
		if err != nil {
			responses = append(responses, map[string]any{"status": http.StatusInternalServerError, "error": err.Error()})
			continue
		}
		s.enqueueStatsRecompute(c.Request().Context(), user.ID)
		responses = append(responses, map[string]any{"status": http.StatusCreated, "data": duration})
	}
	return c.JSON(http.StatusAccepted, map[string]any{"responses": responses})
}

func (s *Server) deleteExternalDurationsBulk(c echo.Context) error {
	user := userFromContext(c)
	var payload struct {
		IDs []string `json:"ids"`
	}
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	ids, err := parseUUIDs(payload.IDs)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	deleted, err := s.Store.DeleteExternalDurations(c.Request().Context(), user.ID, ids)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	if deleted > 0 {
		s.enqueueStatsRecompute(c.Request().Context(), user.ID)
	}
	return c.JSON(http.StatusOK, map[string]any{"data": map[string]any{"deleted": deleted}})
}

func (s *Server) listLeaderboards(c echo.Context) error {
	user := userFromContext(c)
	boards, err := s.Store.ListLeaderboards(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(boards))
}

func (s *Server) createLeaderboard(c echo.Context) error {
	user := userFromContext(c)
	var payload struct {
		Name      string `json:"name"`
		TimeRange string `json:"time_range"`
	}
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	if err := services.ValidateLeaderboardInput(payload.Name, payload.TimeRange); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	board, err := s.Store.CreateLeaderboard(c.Request().Context(), user.ID, payload.Name, payload.TimeRange)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusCreated, map[string]any{"data": board})
}

func (s *Server) getLeaderboard(c echo.Context) error {
	user := userFromContext(c)
	boardID, err := uuid.Parse(c.Param("board"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid leaderboard id"))
	}
	board, err := s.Store.GetLeaderboardForUser(c.Request().Context(), user.ID, boardID)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusNotFound, errorBody("leaderboard not found"))
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	members, err := s.Store.LeaderboardMembers(c.Request().Context(), boardID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	entries, err := s.leaderboardEntries(c.Request().Context(), members, board.TimeRange, "", "")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	memberRows, err := s.Store.LeaderboardMemberRows(c.Request().Context(), boardID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, map[string]any{"data": nonNilSlice(entries), "board": board, "members": nonNilSlice(memberRows)})
}

func (s *Server) updateLeaderboard(c echo.Context) error {
	user := userFromContext(c)
	boardID, err := uuid.Parse(c.Param("board"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid leaderboard id"))
	}
	var payload struct {
		Name      string `json:"name"`
		TimeRange string `json:"time_range"`
	}
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	if err := services.ValidateLeaderboardInput(payload.Name, payload.TimeRange); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	board, err := s.Store.UpdateLeaderboard(c.Request().Context(), user.ID, boardID, payload.Name, payload.TimeRange)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusNotFound, errorBody("leaderboard not found"))
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, map[string]any{"data": board})
}

func (s *Server) deleteLeaderboard(c echo.Context) error {
	user := userFromContext(c)
	boardID, err := uuid.Parse(c.Param("board"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid leaderboard id"))
	}
	if err := s.Store.DeleteLeaderboard(c.Request().Context(), user.ID, boardID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, errorBody("leaderboard not found"))
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) addLeaderboardMember(c echo.Context) error {
	user := userFromContext(c)
	boardID, err := uuid.Parse(c.Param("board"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid leaderboard id"))
	}
	var payload struct {
		Username string `json:"username"`
	}
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	if strings.TrimSpace(payload.Username) == "" {
		return c.JSON(http.StatusBadRequest, errorBody("username is required"))
	}
	member, err := s.Store.UserByGitHubUsername(c.Request().Context(), payload.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusNotFound, errorBody("user not found"))
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	if err := s.Store.AddLeaderboardMember(c.Request().Context(), user.ID, boardID, member.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, errorBody("leaderboard not found"))
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusCreated, map[string]any{
		"data": map[string]any{
			"user_id":   member.ID,
			"username":  member.GitHubUsername,
			"full_name": member.FullName,
		},
	})
}

func (s *Server) removeLeaderboardMember(c echo.Context) error {
	user := userFromContext(c)
	boardID, err := uuid.Parse(c.Param("board"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid leaderboard id"))
	}
	memberID, err := uuid.Parse(c.Param("user"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid user id"))
	}
	if memberID == user.ID {
		return c.JSON(http.StatusBadRequest, errorBody("leaderboard owner cannot be removed"))
	}
	if err := s.Store.RemoveLeaderboardMember(c.Request().Context(), user.ID, boardID, memberID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, errorBody("leaderboard member not found"))
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) listDataDumps(c echo.Context) error {
	user := userFromContext(c)
	dumps, err := s.Store.ListDataDumps(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(dumps))
}

func (s *Server) createDataDump(c echo.Context) error {
	user := userFromContext(c)
	var payload struct {
		Type string `json:"type"`
	}
	_ = c.Bind(&payload)
	dumpType, err := normalizeDataDumpType(payload.Type)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	dump, err := s.Store.CreateDataDump(c.Request().Context(), user.ID, dumpType)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	if err := s.enqueueDataDumpProcess(c.Request().Context(), user.ID, dump.ID); err != nil {
		if errors.Is(err, jobs.ErrQueueUnavailable) {
			dump, err = dumpfiles.GenerateLocal(c.Request().Context(), s.Store, s.Config, user.ID, dump.ID, time.Now().UTC())
			if err != nil {
				return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
			}
			return c.JSON(http.StatusCreated, map[string]any{"data": dump})
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusAccepted, map[string]any{"data": dump})
}

func normalizeDataDumpType(input string) (string, error) {
	dumpType := strings.TrimSpace(input)
	if dumpType == "" {
		return "heartbeats", nil
	}
	switch dumpType {
	case "heartbeats", "daily":
		return dumpType, nil
	default:
		return "", fmt.Errorf("unsupported data dump type %q", dumpType)
	}
}

func (s *Server) downloadDataDump(c echo.Context) error {
	user := userFromContext(c)
	dumpID, err := uuid.Parse(c.Param("dump"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid dump id"))
	}
	dumps, err := s.Store.ListDataDumps(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	var selected db.DataDump
	for _, dump := range dumps {
		if dump.ID == dumpID {
			selected = dump
			break
		}
	}
	if selected.ID == uuid.Nil {
		return c.JSON(http.StatusNotFound, errorBody("data dump not found"))
	}
	if status, message, blocked := dataDumpDownloadError(selected, time.Now().UTC()); blocked {
		return c.JSON(status, errorBody(message))
	}
	if raw, err := dumpfiles.ReadLocalPayload(s.Config, user.ID, selected.ID); err == nil {
		return c.Blob(http.StatusOK, "application/json", raw)
	} else if !errors.Is(err, os.ErrNotExist) {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	if selected.Type == "daily" {
		heartbeats, err := s.Store.AllHeartbeats(c.Request().Context(), user.ID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
		}
		heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
		externalRows, err := s.Store.ListExternalDurations(c.Request().Context(), user.ID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
		}
		external := toServiceExternalDurations(externalRows)
		startDate, endDate := dailyDumpDateRange(heartbeats, external, time.Now().UTC())
		return c.JSON(http.StatusOK, summaryRowsForRange(heartbeats, external, startDate, endDate, time.Duration(user.TimeoutMinutes)*time.Minute, allSummaryFields()))
	}
	heartbeats, err := s.Store.AllHeartbeats(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, heartbeats)
}

func dataDumpDownloadError(dump db.DataDump, now time.Time) (int, string, bool) {
	if dump.Status != "Completed" || dump.IsProcessing {
		return http.StatusConflict, "data dump is still processing", true
	}
	if dump.ExpiresAt != nil && !dump.ExpiresAt.After(now) {
		return http.StatusGone, "data dump expired", true
	}
	return 0, "", false
}

func (s *Server) listCustomRules(c echo.Context) error {
	user := userFromContext(c)
	rules, err := s.Store.ListCustomRules(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(rules))
}

func (s *Server) replaceCustomRules(c echo.Context) error {
	user := userFromContext(c)
	var rules []services.CustomRule
	if err := c.Bind(&rules); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	if err := validateCustomRules(rules); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	stored, err := s.Store.ReplaceCustomRules(c.Request().Context(), user.ID, rules)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	total, err := s.Store.CountHeartbeats(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	if _, err := s.Store.SetCustomRulesProgressQueued(c.Request().Context(), user.ID, total); err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	appliedInline := false
	if err := s.enqueueCustomRulesApply(c.Request().Context(), user.ID); err != nil {
		if errors.Is(err, jobs.ErrQueueUnavailable) {
			if _, err := s.Store.SetCustomRulesProgressProcessing(c.Request().Context(), user.ID, total); err != nil {
				return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
			}
			changed, deleted, err := s.Store.ApplyCustomRulesToHeartbeats(c.Request().Context(), user.ID)
			if err != nil {
				_, _ = s.Store.FailCustomRulesProgress(c.Request().Context(), user.ID, err.Error())
				return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
			}
			if _, err := s.Store.CompleteCustomRulesProgress(c.Request().Context(), user.ID, total, changed, deleted); err != nil {
				return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
			}
			appliedInline = true
		} else {
			return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
		}
	}
	if appliedInline {
		s.enqueueStatsRecompute(c.Request().Context(), user.ID)
	}
	return c.JSON(http.StatusOK, map[string]any{"data": stored})
}

func (s *Server) deleteCustomRule(c echo.Context) error {
	user := userFromContext(c)
	ruleID, err := uuid.Parse(c.Param("rule_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid custom rule id"))
	}
	if err := s.Store.DeleteCustomRule(c.Request().Context(), user.ID, ruleID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, errorBody("custom rule not found"))
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) customRulesProgress(c echo.Context) error {
	user := userFromContext(c)
	progress, err := s.Store.GetCustomRulesProgress(c.Request().Context(), user.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusOK, map[string]any{"data": map[string]any{"status": "NotStarted", "percent_complete": 0, "total": 0, "changed": 0, "deleted": 0}})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, map[string]any{"data": progress})
}

func (s *Server) abortCustomRulesProgress(c echo.Context) error {
	user := userFromContext(c)
	progress, err := s.Store.AbortCustomRulesProgress(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, map[string]any{"data": progress})
}

func (s *Server) listShareTokens(c echo.Context) error {
	user := userFromContext(c)
	tokens, err := s.Store.ListShareTokens(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(tokens))
}

func (s *Server) createShareToken(c echo.Context) error {
	user := userFromContext(c)
	var payload struct {
		Name string `json:"name"`
	}
	_ = c.Bind(&payload)
	token, err := s.Store.CreateShareToken(c.Request().Context(), user.ID, payload.Name)
	if err != nil {
		if errors.Is(err, db.ErrInvalidResourceName) {
			return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusCreated, map[string]any{"data": token})
}

func (s *Server) deleteShareToken(c echo.Context) error {
	user := userFromContext(c)
	tokenID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid share token id"))
	}
	if err := s.Store.DeleteShareToken(c.Request().Context(), user.ID, tokenID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, errorBody("share token not found"))
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) publicUserProfile(c echo.Context) error {
	user, err := s.Store.PublicUserByRef(c.Request().Context(), c.Param("user"))
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusNotFound, errorBody("public user not found"))
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return writePublicPayload(c, map[string]any{"data": publicUser(user)})
}

func (s *Server) publicUserStats(c echo.Context) error {
	user, err := s.Store.PublicUserByRef(c.Request().Context(), c.Param("user"))
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusNotFound, errorBody("public user not found"))
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	rangeName := c.Param("range")
	if rangeName == "" {
		rangeName = strings.TrimSpace(c.QueryParam("range"))
	}
	if rangeName == "" {
		rangeName = "last_7_days"
	}
	stats, err := s.publicStats(c.Request().Context(), user, rangeName)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	return writePublicPayload(c, map[string]any{"data": stats, "user": publicUser(user)})
}

func (s *Server) publicUserSummaries(c echo.Context) error {
	user, err := s.Store.PublicUserByRef(c.Request().Context(), c.Param("user"))
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusNotFound, errorBody("public user not found"))
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	data, err := s.publicSummaries(c.Request().Context(), user, c.QueryParam("start"), c.QueryParam("end"))
	if errors.Is(err, errPublicSummariesDisabled) {
		return c.JSON(http.StatusForbidden, errorBody(err.Error()))
	}
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	return writePublicPayload(c, map[string]any{"data": data, "user": publicUser(user)})
}

func (s *Server) publicShareStats(c echo.Context) error {
	user, err := s.Store.UserByShareToken(c.Request().Context(), c.Param("user"), c.Param("token"))
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusNotFound, errorBody("share token not found"))
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	rangeName := c.QueryParam("range")
	if rangeName == "" {
		rangeName = "last_7_days"
	}
	stats, err := s.publicStats(c.Request().Context(), user, rangeName)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	return writePublicPayload(c, map[string]any{"data": stats, "user": publicUser(user)})
}

func (s *Server) publicShareStatsByToken(c echo.Context) error {
	user, err := s.Store.UserByShareTokenOnly(c.Request().Context(), c.Param("token"))
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusNotFound, errorBody("share token not found"))
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	rangeName := c.QueryParam("range")
	if rangeName == "" {
		rangeName = "last_7_days"
	}
	stats, err := s.publicStats(c.Request().Context(), user, rangeName)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	return writePublicPayload(c, map[string]any{"data": stats, "user": publicUser(user)})
}

func (s *Server) publicShareSummaries(c echo.Context) error {
	user, err := s.Store.UserByShareToken(c.Request().Context(), c.Param("user"), c.Param("token"))
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusNotFound, errorBody("share token not found"))
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	data, err := s.publicSummaries(c.Request().Context(), user, c.QueryParam("start"), c.QueryParam("end"))
	if errors.Is(err, errPublicSummariesDisabled) {
		return c.JSON(http.StatusForbidden, errorBody(err.Error()))
	}
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	return writePublicPayload(c, map[string]any{"data": data, "user": publicUser(user)})
}

func (s *Server) publicShareSummariesByToken(c echo.Context) error {
	user, err := s.Store.UserByShareTokenOnly(c.Request().Context(), c.Param("token"))
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusNotFound, errorBody("share token not found"))
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	data, err := s.publicSummaries(c.Request().Context(), user, c.QueryParam("start"), c.QueryParam("end"))
	if errors.Is(err, errPublicSummariesDisabled) {
		return c.JSON(http.StatusForbidden, errorBody(err.Error()))
	}
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	return writePublicPayload(c, map[string]any{"data": data, "user": publicUser(user)})
}

func (s *Server) publicStats(ctx context.Context, user db.User, rangeName string) (services.Stats, error) {
	if rangeName == "all_time" {
		heartbeats, err := s.Store.AllHeartbeats(ctx, user.ID)
		if err != nil {
			return services.Stats{}, err
		}
		heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
		externalRows, err := s.Store.ListExternalDurations(ctx, user.ID)
		if err != nil {
			return services.Stats{}, err
		}
		costs, err := s.Store.AICostRates(ctx, user.ID)
		if err != nil {
			return services.Stats{}, err
		}
		stats := services.ComputeAllTimeStatsWithExternalDurationsAndAICosts(heartbeats, toServiceExternalDurations(externalRows), time.Duration(user.TimeoutMinutes)*time.Minute, costs)
		return s.applyPublicStatsPermissions(ctx, user, stats)
	}
	now := time.Now().In(userLocation(user))
	if _, err := services.WindowForRange(now, rangeName); err != nil {
		return services.Stats{}, err
	}
	heartbeats, err := s.Store.HeartbeatsForStatsRange(ctx, user.ID, now, rangeName)
	if err != nil {
		return services.Stats{}, err
	}
	heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
	window, err := services.WindowForRange(now, rangeName)
	if err != nil {
		return services.Stats{}, err
	}
	externalRows, err := s.Store.ExternalDurationsBetween(ctx, user.ID, window.Start, window.End)
	if err != nil {
		return services.Stats{}, err
	}
	costs, err := s.Store.AICostRates(ctx, user.ID)
	if err != nil {
		return services.Stats{}, err
	}
	stats, _, err := services.ComputeStatsForRangeWithExternalDurationsAndAICosts(heartbeats, toServiceExternalDurations(externalRows), now, time.Duration(user.TimeoutMinutes)*time.Minute, rangeName, costs)
	if err != nil {
		return services.Stats{}, err
	}
	return s.applyPublicStatsPermissions(ctx, user, stats)
}

func (s *Server) publicSummaries(ctx context.Context, user db.User, startValue, endValue string) ([]map[string]any, error) {
	if !user.PublicShowSummaries {
		return nil, errPublicSummariesDisabled
	}
	startDate, endDate, err := dateRangeInLocation(startValue, endValue, userLocation(user), time.Now())
	if err != nil {
		return nil, err
	}
	heartbeats, err := s.Store.HeartbeatsBetween(ctx, user.ID, float64(startDate.Unix()), float64(endDate.AddDate(0, 0, 1).Unix()))
	if err != nil {
		return nil, err
	}
	heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
	externalRows, err := s.Store.ExternalDurationsBetween(ctx, user.ID, startDate, endDate.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	external := toServiceExternalDurations(externalRows)
	rows := summaryRowsForRange(heartbeats, external, startDate, endDate, time.Duration(user.TimeoutMinutes)*time.Minute, publicSummaryFields(user))
	projectAllow, err := s.publicProjectAllowSet(ctx, user)
	if err != nil {
		return nil, err
	}
	return redactPublicSummaryRows(rows, user, projectAllow), nil
}

func (s *Server) applyPublicStatsPermissions(ctx context.Context, user db.User, stats services.Stats) (services.Stats, error) {
	projectAllow, err := s.publicProjectAllowSet(ctx, user)
	if err != nil {
		return services.Stats{}, err
	}
	if !user.PublicShowTotalTime {
		stats.TotalSeconds = 0
		stats.HumanReadableTotal = ""
		stats.DailyAverageSeconds = 0
		stats.HumanReadableDaily = ""
		stats.BestDay = services.DailyStat{}
		for i := range stats.Days {
			stats.Days[i].TotalSeconds = 0
			stats.Days[i].Text = ""
		}
	}
	if !user.PublicShowProjects || user.PublicProjectVisibility == "none" {
		stats.Projects = nil
		stats.ProjectAI = nil
		for i := range stats.Days {
			stats.Days[i].Projects = nil
		}
		for i := range stats.Hourly {
			stats.Hourly[i].Projects = nil
		}
	} else if projectAllow != nil {
		stats.Projects = filterSliceTotals(stats.Projects, projectAllow)
		stats.ProjectAI = filterAIStats(stats.ProjectAI, projectAllow)
		for i := range stats.Days {
			stats.Days[i].Projects = filterSliceTotals(stats.Days[i].Projects, projectAllow)
		}
		for i := range stats.Hourly {
			stats.Hourly[i].Projects = filterSliceTotals(stats.Hourly[i].Projects, projectAllow)
		}
	}
	if !user.PublicShowLanguages {
		stats.Languages = nil
		for i := range stats.Hourly {
			stats.Hourly[i].Languages = nil
		}
	}
	if !user.PublicShowEditors {
		stats.Editors = nil
	}
	if !user.PublicShowMachines {
		stats.Machines = nil
	}
	if !user.PublicShowOS {
		stats.OperatingSystems = nil
	}
	if !user.PublicShowCategories {
		stats.Categories = nil
	}
	stats.Branches = nil
	stats.Dependencies = nil
	if !user.PublicShowAI {
		stats.AI = services.AIMetrics{}
		stats.ProjectAI = nil
	}
	return stats, nil
}

func (s *Server) publicProjectAllowSet(ctx context.Context, user db.User) (map[string]bool, error) {
	if !user.PublicShowProjects || user.PublicProjectVisibility == "none" || user.PublicProjectVisibility == "all" {
		return nil, nil
	}
	return s.Store.PublicProjectNames(ctx, user.ID)
}

func publicSummaryFields(user db.User) summaryFields {
	return summaryFields{
		Projects:         user.PublicShowProjects && user.PublicProjectVisibility != "none",
		Languages:        user.PublicShowLanguages,
		Categories:       user.PublicShowCategories,
		Dependencies:     false,
		Editors:          user.PublicShowEditors,
		Machines:         user.PublicShowMachines,
		OperatingSystems: user.PublicShowOS,
	}
}

func redactPublicSummaryRows(rows []map[string]any, user db.User, projectAllow map[string]bool) []map[string]any {
	for _, row := range rows {
		if !user.PublicShowTotalTime {
			row["grand_total"] = map[string]any{"total_seconds": 0, "text": ""}
		}
		if projectAllow != nil {
			if projects, ok := row["projects"].([]services.SliceTotal); ok {
				row["projects"] = filterSliceTotals(projects, projectAllow)
			}
		}
	}
	return rows
}

func filterSliceTotals(rows []services.SliceTotal, allow map[string]bool) []services.SliceTotal {
	if allow == nil {
		return rows
	}
	filtered := make([]services.SliceTotal, 0, len(rows))
	for _, row := range rows {
		if allow[row.Name] {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func filterAIStats(rows []services.AIStat, allow map[string]bool) []services.AIStat {
	if allow == nil {
		return rows
	}
	filtered := make([]services.AIStat, 0, len(rows))
	for _, row := range rows {
		if allow[row.Name] {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

type summaryFields struct {
	Projects         bool
	Languages        bool
	Categories       bool
	Dependencies     bool
	Editors          bool
	Machines         bool
	OperatingSystems bool
}

func allSummaryFields() summaryFields {
	return summaryFields{
		Projects:         true,
		Languages:        true,
		Categories:       true,
		Dependencies:     true,
		Editors:          true,
		Machines:         true,
		OperatingSystems: true,
	}
}

func (f summaryFields) Any() bool {
	return f.Projects || f.Languages || f.Categories || f.Dependencies || f.Editors || f.Machines || f.OperatingSystems
}

func summaryRowsForRange(heartbeats []services.Heartbeat, external []services.ExternalDuration, startDate, endDate time.Time, timeout time.Duration, fields summaryFields) []map[string]any {
	data := []map[string]any{}
	for day := startDate; !day.After(endDate); day = day.AddDate(0, 0, 1) {
		next := day.AddDate(0, 0, 1)
		var daily []services.Heartbeat
		for _, heartbeat := range heartbeats {
			t := time.Unix(int64(heartbeat.Time), 0).UTC()
			if !t.Before(day) && t.Before(next) {
				daily = append(daily, heartbeat)
			}
		}
		var dailyExternal []services.ExternalDuration
		for _, duration := range external {
			started := time.Unix(int64(duration.StartTime), 0).UTC()
			ended := time.Unix(int64(duration.EndTime), 0).UTC()
			if started.Before(next) && ended.After(day) {
				dailyExternal = append(dailyExternal, duration)
			}
		}
		stats, _, _ := services.ComputeStatsForRangeWithExternalDurations(daily, dailyExternal, day.Add(12*time.Hour), timeout, "last_7_days")
		row := map[string]any{
			"range": map[string]string{
				"date":  day.Format("2006-01-02"),
				"start": day.Format(time.RFC3339),
				"end":   next.Format(time.RFC3339),
			},
			"grand_total": map[string]any{"total_seconds": stats.TotalSeconds, "text": services.HumanDuration(stats.TotalSeconds)},
		}
		if fields.Projects {
			row["projects"] = stats.Projects
		}
		if fields.Languages {
			row["languages"] = stats.Languages
		}
		if fields.Categories {
			row["categories"] = stats.Categories
		}
		if fields.Dependencies {
			row["dependencies"] = stats.Dependencies
		}
		if fields.Editors {
			row["editors"] = stats.Editors
		}
		if fields.Machines {
			row["machines"] = stats.Machines
		}
		if fields.OperatingSystems {
			row["operating_systems"] = stats.OperatingSystems
		}
		data = append(data, row)
	}
	return data
}

func dailyDumpDateRange(heartbeats []services.Heartbeat, external []services.ExternalDuration, now time.Time) (time.Time, time.Time) {
	start := utcDate(now)
	end := start
	expand := func(t time.Time) {
		day := utcDate(t)
		if day.Before(start) {
			start = day
		}
		if day.After(end) {
			end = day
		}
	}
	for _, heartbeat := range heartbeats {
		if heartbeat.Time > 0 {
			expand(time.Unix(int64(heartbeat.Time), 0).UTC())
		}
	}
	for _, duration := range external {
		if duration.StartTime > 0 {
			expand(time.Unix(int64(duration.StartTime), 0).UTC())
		}
		if duration.EndTime > 0 {
			expand(time.Unix(int64(duration.EndTime), 0).UTC())
		}
	}
	return start, end
}

func utcDate(t time.Time) time.Time {
	year, month, day := t.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

const (
	maxImportBodyBytes  = 128 << 20
	maxImportHeartbeats = 250000
)

func (s *Server) importWakaTimeDump(c echo.Context) error {
	user := userFromContext(c)
	raw, err := readImportBody(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	heartbeats, err := services.ExtractHeartbeatsFromWakaTimeDump(raw)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	if len(heartbeats) > maxImportHeartbeats {
		return c.JSON(http.StatusBadRequest, errorBody(fmt.Sprintf("import limit is %d heartbeats per request", maxImportHeartbeats)))
	}
	defaults := heartbeatDefaults(c.Request().UserAgent())
	if err := s.enqueueWakaTimeImport(c.Request().Context(), user.ID, heartbeats, defaults); err != nil {
		if errors.Is(err, jobs.ErrQueueUnavailable) {
			result, err := importer.ProcessHeartbeats(c.Request().Context(), s.Store, user.ID, heartbeats, defaults, time.Now())
			if err != nil {
				return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
			}
			if result.Inserted > 0 {
				s.enqueueStatsRecompute(c.Request().Context(), user.ID)
			}
			return c.JSON(http.StatusAccepted, map[string]any{"data": result})
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusAccepted, map[string]any{"data": importer.QueuedResult(len(heartbeats))})
}

func (s *Server) listAICosts(c echo.Context) error {
	user := userFromContext(c)
	settings, err := s.Store.ListAICostSettings(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(settings))
}

func (s *Server) replaceAICosts(c echo.Context) error {
	user := userFromContext(c)
	var settings []db.AICostSetting
	if err := c.Bind(&settings); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	if len(settings) > 50 {
		return c.JSON(http.StatusBadRequest, errorBody("AI cost settings limit is 50 agents"))
	}
	if err := db.ValidateAICostSettings(settings); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	updated, err := s.Store.ReplaceAICostSettings(c.Request().Context(), user.ID, settings)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	s.enqueueStatsRecompute(c.Request().Context(), user.ID)
	return c.JSON(http.StatusOK, map[string]any{"data": updated})
}

func (s *Server) insight(c echo.Context) error {
	user := userFromContext(c)
	rangeName := c.Param("range")
	stats, err := s.statsForInsight(c.Request().Context(), user, rangeName)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}

	data, ok := insightData(c.Param("insight_type"), stats)
	if !ok {
		return c.JSON(http.StatusBadRequest, errorBody("unsupported insight type"))
	}
	return c.JSON(http.StatusOK, map[string]any{"data": data, "range": rangeName})
}

func insightData(insightType string, stats services.Stats) (any, bool) {
	switch insightType {
	case "stats":
		return stats, true
	case "projects":
		return stats.Projects, true
	case "languages":
		return stats.Languages, true
	case "editors":
		return stats.Editors, true
	case "machines":
		return stats.Machines, true
	case "operating_systems":
		return stats.OperatingSystems, true
	case "categories":
		return stats.Categories, true
	case "dependencies":
		return stats.Dependencies, true
	case "days":
		return stats.Days, true
	case "hours":
		return stats.Hourly, true
	case "weekdays":
		return services.ComputeWeekdayStats(stats.Days), true
	case "ai_days":
		return stats.AI.Days, true
	case "ai_agents":
		return stats.AI.Agents, true
	case "best_day":
		return stats.BestDay, true
	case "daily_average":
		return map[string]any{"seconds": stats.DailyAverageSeconds, "text": stats.HumanReadableDaily}, true
	case "daily_average_trend":
		return services.ComputeDailyAverageTrend(stats.Days), true
	}
	return nil, false
}

func supportedInsightTypes() []string {
	return []string{
		"stats",
		"projects",
		"languages",
		"editors",
		"machines",
		"operating_systems",
		"categories",
		"dependencies",
		"days",
		"hours",
		"weekdays",
		"best_day",
		"daily_average",
		"daily_average_trend",
		"ai_agents",
		"ai_days",
	}
}

func (s *Server) listOAuthApps(c echo.Context) error {
	user := userFromContext(c)
	apps, err := s.Store.ListOAuthApps(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(apps))
}

func (s *Server) createOAuthApp(c echo.Context) error {
	user := userFromContext(c)
	var payload struct {
		Name         string   `json:"name"`
		RedirectURIs []string `json:"redirect_uris"`
		Scopes       []string `json:"scopes"`
	}
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	if err := db.ValidateOAuthAppInput(db.OAuthAppInput{Name: payload.Name, RedirectURIs: payload.RedirectURIs, Scopes: payload.Scopes}); err != nil {
		if errors.Is(err, db.ErrInvalidOAuthScope) {
			return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
		}
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	app, err := s.Store.CreateOAuthApp(c.Request().Context(), user.ID, payload.Name, payload.RedirectURIs, payload.Scopes)
	if err != nil {
		if errors.Is(err, db.ErrInvalidOAuthScope) {
			return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusCreated, map[string]any{"data": app})
}

func (s *Server) deleteOAuthApp(c echo.Context) error {
	user := userFromContext(c)
	appID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid OAuth app id"))
	}
	if err := s.Store.DeleteOAuthApp(c.Request().Context(), user.ID, appID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, errorBody("OAuth app not found"))
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) statsForInsight(ctx context.Context, user db.User, rangeName string) (services.Stats, error) {
	return s.computeStatsForRange(ctx, user, rangeName)
}

func (s *Server) listAPIKeys(c echo.Context) error {
	user := userFromContext(c)
	keys, err := s.Store.ListAPIKeys(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(keys))
}

func (s *Server) createAPIKey(c echo.Context) error {
	user := userFromContext(c)
	var payload struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	_ = c.Bind(&payload)
	apiKey, raw, err := s.Store.CreateAPIKeyWithScopes(c.Request().Context(), user.ID, payload.Name, payload.Scopes)
	if err != nil {
		if errors.Is(err, db.ErrInvalidOAuthScope) || errors.Is(err, db.ErrInvalidResourceName) {
			return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusCreated, map[string]any{"data": map[string]any{"key": apiKey, "api_key": raw}})
}

func (s *Server) revokeAPIKey(c echo.Context) error {
	user := userFromContext(c)
	keyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid API key id"))
	}
	if err := s.Store.RevokeAPIKey(c.Request().Context(), user.ID, keyID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, errorBody("API key not found"))
		}
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) requireUser(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if key, ok := authKey(c.Request()); ok {
			result, err := s.Store.AuthByAPIKey(c.Request().Context(), key)
			if err == nil {
				c.Set("user", result.User)
				c.Set("auth", authContext{Kind: result.Kind, Subject: result.Subject, Scopes: result.Scopes})
				return next(c)
			}
			result, err = s.Store.AuthByOAuthAccessToken(c.Request().Context(), key)
			if err == nil {
				c.Set("user", result.User)
				c.Set("auth", authContext{Kind: result.Kind, Subject: result.Subject, Scopes: result.Scopes})
				return next(c)
			}
		}
		if token, ok := auth.ExtractBearerToken(c.Request()); ok {
			userID, err := auth.VerifySessionJWT(token, s.Config.SessionSecret)
			if err == nil {
				parsed, err := uuid.Parse(userID)
				if err == nil {
					user, err := s.Store.UserByID(c.Request().Context(), parsed)
					if err == nil {
						c.Set("user", user)
						c.Set("auth", authContext{Kind: "jwt", Subject: user.ID.String(), Scopes: allAuthScopes()})
						return next(c)
					}
				}
			}
		}
		if cookie, err := c.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
			user, err := s.Store.UserBySessionToken(c.Request().Context(), cookie.Value)
			if err == nil {
				c.Set("user", user)
				c.Set("auth", authContext{Kind: "session", Subject: user.ID.String(), Scopes: allAuthScopes()})
				return next(c)
			}
		}
		return c.JSON(http.StatusUnauthorized, wakaError("unauthorized"))
	}
}

type authContext struct {
	Kind    string
	Subject string
	Scopes  []string
}

func (a authContext) RateLimitSubject(userID uuid.UUID) string {
	subject := a.Subject
	if subject == "" {
		subject = userID.String()
	}
	kind := a.Kind
	if kind == "" {
		kind = "user"
	}
	return kind + ":" + subject
}

const (
	scopeReadStats                     = "read_stats"
	scopeReadStatsBestDay              = "read_stats.best_day"
	scopeReadStatsCategories           = "read_stats.categories"
	scopeReadStatsDependencies         = "read_stats.dependencies"
	scopeReadStatsEditors              = "read_stats.editors"
	scopeReadStatsLanguages            = "read_stats.languages"
	scopeReadStatsMachines             = "read_stats.machines"
	scopeReadStatsOperatingSystems     = "read_stats.operating_systems"
	scopeReadStatsProjects             = "read_stats.projects"
	scopeReadSummaries                 = "read_summaries"
	scopeReadSummariesCategories       = "read_summaries.categories"
	scopeReadSummariesDependencies     = "read_summaries.dependencies"
	scopeReadSummariesEditors          = "read_summaries.editors"
	scopeReadSummariesLanguages        = "read_summaries.languages"
	scopeReadSummariesMachines         = "read_summaries.machines"
	scopeReadSummariesOperatingSystems = "read_summaries.operating_systems"
	scopeReadSummariesProjects         = "read_summaries.projects"
	scopeReadHeartbeats                = "read_heartbeats"
	scopeWriteHeartbeats               = "write_heartbeats"
	scopeReadGoals                     = "read_goals"
	scopeReadPrivateLeaderboards       = "read_private_leaderboards"
	scopeWritePrivateLeaderboards      = "write_private_leaderboards"
)

func requireScope(scope string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authInfo, _ := c.Get("auth").(authContext)
			if authInfo.Kind == "session" || authInfo.HasScope(scope) {
				return next(c)
			}
			return c.JSON(http.StatusForbidden, wakaError("insufficient_scope"))
		}
	}
}

func requireLocalAccountAccess(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		authInfo, _ := c.Get("auth").(authContext)
		if authInfo.Kind == "session" || authInfo.Kind == "jwt" || (authInfo.Kind == "api_key" && authInfo.HasAllScopes(db.DefaultAPIKeyScopes())) {
			return next(c)
		}
		return c.JSON(http.StatusForbidden, wakaError("insufficient_scope"))
	}
}

func requireInsightScope(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		authInfo, _ := c.Get("auth").(authContext)
		if authInfo.Kind == "session" || authInfo.HasScope(scopeForInsightType(c.Param("insight_type"))) {
			return next(c)
		}
		return c.JSON(http.StatusForbidden, wakaError("insufficient_scope"))
	}
}

func requireSummarySliceScope(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		authInfo, _ := c.Get("auth").(authContext)
		if authInfo.Kind == "session" || authInfo.HasScope(summaryScopeForSliceBy(c.QueryParam("slice_by"))) {
			return next(c)
		}
		return c.JSON(http.StatusForbidden, wakaError("insufficient_scope"))
	}
}

func requireSummaryScope(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		authInfo, _ := c.Get("auth").(authContext)
		fields := summaryFieldsForAuth(authInfo)
		if fields.Any() {
			return next(c)
		}
		return c.JSON(http.StatusForbidden, wakaError("insufficient_scope"))
	}
}

func scopeForInsightType(insightType string) string {
	switch insightType {
	case "best_day":
		return scopeReadStatsBestDay
	case "categories":
		return scopeReadStatsCategories
	case "dependencies":
		return scopeReadStatsDependencies
	case "editors":
		return scopeReadStatsEditors
	case "languages":
		return scopeReadStatsLanguages
	case "machines":
		return scopeReadStatsMachines
	case "operating_systems":
		return scopeReadStatsOperatingSystems
	case "projects":
		return scopeReadStatsProjects
	default:
		return scopeReadStats
	}
}

func summaryScopeForSliceBy(sliceBy string) string {
	switch sliceBy {
	case "language":
		return scopeReadSummariesLanguages
	case "editor":
		return scopeReadSummariesEditors
	case "machine":
		return scopeReadSummariesMachines
	case "operating_system":
		return scopeReadSummariesOperatingSystems
	case "category":
		return scopeReadSummariesCategories
	case "dependencies":
		return scopeReadSummariesDependencies
	default:
		return scopeReadSummariesProjects
	}
}

func summaryFieldsForAuth(authInfo authContext) summaryFields {
	if authInfo.Kind == "session" {
		return allSummaryFields()
	}
	return summaryFields{
		Projects:         authInfo.HasScope(scopeReadSummariesProjects),
		Languages:        authInfo.HasScope(scopeReadSummariesLanguages),
		Categories:       authInfo.HasScope(scopeReadSummariesCategories),
		Dependencies:     authInfo.HasScope(scopeReadSummariesDependencies),
		Editors:          authInfo.HasScope(scopeReadSummariesEditors),
		Machines:         authInfo.HasScope(scopeReadSummariesMachines),
		OperatingSystems: authInfo.HasScope(scopeReadSummariesOperatingSystems),
	}
}

func (a authContext) HasScope(scope string) bool {
	for _, candidate := range a.Scopes {
		if candidate == scope || strings.HasPrefix(scope, candidate+".") {
			return true
		}
	}
	return false
}

func (a authContext) HasAllScopes(scopes []string) bool {
	for _, scope := range scopes {
		if !a.HasScope(scope) {
			return false
		}
	}
	return true
}

func allAuthScopes() []string {
	return []string{
		scopeReadStats,
		scopeReadStatsBestDay,
		scopeReadStatsCategories,
		scopeReadStatsDependencies,
		scopeReadStatsEditors,
		scopeReadStatsLanguages,
		scopeReadStatsMachines,
		scopeReadStatsOperatingSystems,
		scopeReadStatsProjects,
		scopeReadSummaries,
		scopeReadSummariesCategories,
		scopeReadSummariesDependencies,
		scopeReadSummariesEditors,
		scopeReadSummariesLanguages,
		scopeReadSummariesMachines,
		scopeReadSummariesOperatingSystems,
		scopeReadSummariesProjects,
		scopeReadHeartbeats,
		scopeWriteHeartbeats,
		scopeReadGoals,
		scopeReadPrivateLeaderboards,
		scopeWritePrivateLeaderboards,
		"email",
	}
}

func (s *Server) rateLimitUser(name string, limit int, window time.Duration) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user := userFromContext(c)
			key := name + ":user:" + user.ID.String()
			if err := s.enforceRateLimit(c, key, limit, window); err != nil {
				return err
			}
			return next(c)
		}
	}
}

func (s *Server) rateLimitAuthenticatedRead(limit int, window time.Duration) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user := userFromContext(c)
			authInfo, _ := c.Get("auth").(authContext)
			key := "read:" + authInfo.RateLimitSubject(user.ID)
			if err := s.enforceRateLimit(c, key, limit, window); err != nil {
				return err
			}
			return next(c)
		}
	}
}

func (s *Server) rateLimitIP(name string, limit int, window time.Duration) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := name + ":ip:" + c.RealIP()
			if err := s.enforceRateLimit(c, key, limit, window); err != nil {
				return err
			}
			return next(c)
		}
	}
}

func (s *Server) rateLimitOAuthToken(limit int, window time.Duration) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			clientID, _ := oauthClientCredentials(c.Request())
			if clientID == "" {
				clientID = c.RealIP()
			}
			if err := s.enforceRateLimit(c, "oauth-token:"+clientID, limit, window); err != nil {
				return err
			}
			return next(c)
		}
	}
}

func (s *Server) enforceOAuthTokenUserRateLimit(c echo.Context, userID uuid.UUID) error {
	return s.enforceRateLimit(c, oauthTokenUserRateLimitKey(userID), oauthTokenCreationRateLimit, time.Hour)
}

func oauthTokenUserRateLimitKey(userID uuid.UUID) string {
	return "oauth-token:user:" + userID.String()
}

func (s *Server) enforceRateLimit(c echo.Context, key string, limit int, window time.Duration) error {
	allowed, retryAfter, err := s.Limiter.Allow(c.Request().Context(), key, limit, window)
	if err != nil && s.FallbackLimiter != nil {
		allowed, retryAfter, err = s.FallbackLimiter.Allow(c.Request().Context(), key, limit, window)
	}
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, errorBody("rate limiter unavailable"))
	}
	if allowed {
		return nil
	}
	c.Response().Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
	return c.JSON(http.StatusTooManyRequests, errorBody("rate limit exceeded"))
}

func (s *Server) enqueueStatsRecompute(ctx context.Context, userID uuid.UUID) {
	if s.Jobs == nil {
		return
	}
	_ = s.Jobs.EnqueueStatsRecompute(ctx, userID, jobs.DefaultStatsRanges())
}

func (s *Server) enqueueDataDumpProcess(ctx context.Context, userID, dumpID uuid.UUID) error {
	if s.Jobs == nil {
		return jobs.ErrQueueUnavailable
	}
	return s.Jobs.EnqueueDataDumpProcess(ctx, userID, dumpID)
}

func (s *Server) enqueueCustomRulesApply(ctx context.Context, userID uuid.UUID) error {
	if s.Jobs == nil {
		return jobs.ErrQueueUnavailable
	}
	return s.Jobs.EnqueueCustomRulesApply(ctx, userID)
}

func (s *Server) enqueueWakaTimeImport(ctx context.Context, userID uuid.UUID, heartbeats []services.Heartbeat, defaults services.HeartbeatDefaults) error {
	if s.Jobs == nil {
		return jobs.ErrQueueUnavailable
	}
	return s.Jobs.EnqueueWakaTimeImport(ctx, userID, heartbeats, defaults)
}

func (s *Server) enqueueHeartbeatsPurge(ctx context.Context, retentionDays int) error {
	if s.Jobs == nil {
		return jobs.ErrQueueUnavailable
	}
	return s.Jobs.EnqueueHeartbeatsPurge(ctx, retentionDays)
}

func (s *Server) enqueueLeaderboardUpdate(ctx context.Context, rangeName string) error {
	if s.Jobs == nil {
		return jobs.ErrQueueUnavailable
	}
	return s.Jobs.EnqueueLeaderboardUpdate(ctx, rangeName)
}

func (s *Server) enqueueGoalsEvaluate(ctx context.Context, now time.Time) error {
	if s.Jobs == nil {
		return jobs.ErrQueueUnavailable
	}
	return s.Jobs.EnqueueGoalsEvaluate(ctx, now)
}

func (s *Server) runHeartbeatsPurge(ctx context.Context, retentionDays int) (int64, error) {
	if heartbeatPurgeMode(retentionDays) == heartbeatPurgePerUser {
		return s.Store.PurgeHeartbeatsByUserRetention(ctx, time.Now().UTC())
	}
	cutoff, ok := jobs.HeartbeatsPurgeCutoff(jobs.HeartbeatsPurgePayload{RetentionDays: retentionDays})
	if !ok {
		return 0, nil
	}
	return s.Store.PurgeHeartbeatsBefore(ctx, cutoff)
}

type heartbeatPurgeModeName string

const (
	heartbeatPurgePerUser heartbeatPurgeModeName = "per_user"
	heartbeatPurgeGlobal  heartbeatPurgeModeName = "global"
)

func heartbeatPurgeMode(retentionDays int) heartbeatPurgeModeName {
	if retentionDays > 0 {
		return heartbeatPurgeGlobal
	}
	return heartbeatPurgePerUser
}

func (s *Server) runGoalsEvaluate(ctx context.Context, now time.Time) (int, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	users, err := s.Store.ListUsers(ctx)
	if err != nil {
		return 0, err
	}
	evaluated := 0
	for _, user := range users {
		userNow := now
		if location, err := time.LoadLocation(user.Timezone); err == nil {
			userNow = now.In(location)
		}
		goals, err := s.Store.ListGoals(ctx, user.ID)
		if err != nil {
			return evaluated, err
		}
		for _, goal := range goals {
			if !goal.IsEnabled {
				continue
			}
			start, end := services.GoalEvaluationWindow(goal.Delta, userNow)
			heartbeats, err := s.Store.AllHeartbeats(ctx, user.ID)
			if err != nil {
				return evaluated, err
			}
			heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
			externalRows, err := s.Store.ListExternalDurations(ctx, user.ID)
			if err != nil {
				return evaluated, err
			}
			progress := services.ComputeGoalProgressForWindowWithExternalDurations(toServiceGoal(goal), heartbeats, toServiceExternalDurations(externalRows), start, end, time.Duration(user.TimeoutMinutes)*time.Minute)
			if _, err := s.Store.UpsertGoalEvaluation(ctx, user.ID, goal, progress, start, end); err != nil {
				return evaluated, err
			}
			evaluated++
		}
	}
	return evaluated, nil
}

func (s *Server) sessionUser(c echo.Context) (db.User, bool) {
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return db.User{}, false
	}
	user, err := s.Store.UserBySessionToken(c.Request().Context(), cookie.Value)
	if err != nil {
		return db.User{}, false
	}
	return user, true
}

func sessionTokenFromCookie(c echo.Context) (string, bool) {
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func clearSessionCookies(c echo.Context) {
	for _, name := range []string{sessionCookieName, sessionJWTCookieName} {
		c.SetCookie(&http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
	}
}

func (s *Server) setSessionCookie(c echo.Context, userID uuid.UUID) error {
	token, err := s.Store.CreateSession(c.Request().Context(), userID)
	if err != nil {
		return err
	}
	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   strings.HasPrefix(s.Config.BaseURL, "https://"),
	})
	jwt, err := s.sessionJWT(userID)
	if err != nil {
		return err
	}
	c.SetCookie(&http.Cookie{
		Name:     sessionJWTCookieName,
		Value:    jwt,
		Path:     "/",
		Expires:  time.Now().Add(sessionJWTTTL),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   strings.HasPrefix(s.Config.BaseURL, "https://"),
	})
	return nil
}

func (s *Server) sessionJWT(userID uuid.UUID) (string, error) {
	return auth.GenerateSessionJWT(userID.String(), s.Config.SessionSecret, sessionJWTTTL)
}

func prepHeartbeat(heartbeat *services.Heartbeat, userAgent string) {
	services.PrepareHeartbeat(heartbeat, heartbeatDefaults(userAgent))
}

func heartbeatDefaults(userAgent string) services.HeartbeatDefaults {
	info := apimw.ParseUserAgent(userAgent)
	return services.HeartbeatDefaults{
		Plugin:          info.Plugin,
		PluginVersion:   info.PluginVersion,
		Editor:          info.Editor,
		EditorVersion:   info.EditorVersion,
		OperatingSystem: info.OperatingSystem,
		Architecture:    info.Architecture,
		AIAgent:         info.AIAgent,
		AIAgentVersion:  info.AIAgentVersion,
	}
}

func validateHeartbeat(heartbeat services.Heartbeat) error {
	return services.ValidateHeartbeat(heartbeat)
}

func authKey(r *http.Request) (string, bool) {
	return auth.ExtractAPIKey(r)
}

func userFromContext(c echo.Context) db.User {
	user, _ := c.Get("user").(db.User)
	return user
}

func toServiceGoal(goal db.Goal) services.Goal {
	out := services.Goal{
		ID:               goal.ID.String(),
		Title:            goal.Title,
		CustomTitle:      goal.CustomTitle,
		Delta:            goal.Delta,
		Seconds:          goal.Seconds,
		Languages:        goal.Languages,
		Editors:          goal.Editors,
		Projects:         goal.Projects,
		IgnoreDays:       goal.IgnoreDays,
		IgnoreZeroDays:   goal.IgnoreZeroDays,
		ImproveByPercent: goal.ImproveByPercent,
		IsEnabled:        goal.IsEnabled,
		IsInverse:        goal.IsInverse,
		IsSnoozed:        goal.IsSnoozed,
		CreatedAt:        goal.CreatedAt.Format(time.RFC3339),
		ModifiedAt:       goal.ModifiedAt.Format(time.RFC3339),
	}
	if goal.SnoozeUntil != nil {
		out.SnoozeUntil = goal.SnoozeUntil.Format(time.RFC3339)
	}
	return out
}

func toServiceExternalDurations(durations []db.ExternalDuration) []services.ExternalDuration {
	out := make([]services.ExternalDuration, 0, len(durations))
	for _, duration := range durations {
		out = append(out, services.ExternalDuration{
			ID:         duration.ID.String(),
			ExternalID: duration.ExternalID,
			Provider:   duration.Provider,
			Entity:     duration.Entity,
			Type:       duration.Type,
			Category:   duration.Category,
			StartTime:  duration.StartTime,
			EndTime:    duration.EndTime,
			Project:    duration.Project,
			Branch:     duration.Branch,
			Language:   duration.Language,
			Meta:       duration.Meta,
		})
	}
	return out
}

func (s *Server) leaderboardEntries(ctx context.Context, users []db.User, rangeName, language, country string) ([]services.LeaderboardEntry, error) {
	if users == nil {
		var err error
		users, err = s.Store.ListUsers(ctx)
		if err != nil {
			return nil, err
		}
	}
	entries := make([]services.LeaderboardEntry, 0, len(users))
	now := time.Now()
	for _, user := range users {
		if !leaderboardCountryMatches(user, country) {
			continue
		}
		heartbeats, err := s.Store.HeartbeatsForStatsRange(ctx, user.ID, now, rangeName)
		if err != nil {
			return nil, err
		}
		heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
		window, err := services.WindowForRange(now, rangeName)
		if err != nil {
			return nil, err
		}
		externalRows, err := s.Store.ExternalDurationsBetween(ctx, user.ID, window.Start, window.End)
		if err != nil {
			return nil, err
		}
		stats, _, err := services.ComputeStatsForRangeWithExternalDurations(heartbeats, toServiceExternalDurations(externalRows), now, time.Duration(user.TimeoutMinutes)*time.Minute, rangeName)
		if err != nil {
			return nil, err
		}
		totalSeconds := stats.TotalSeconds
		if language != "" {
			var ok bool
			totalSeconds, ok = leaderboardLanguageSeconds(stats, language)
			if !ok || totalSeconds <= 0 {
				continue
			}
		}
		entries = append(entries, services.LeaderboardEntry{
			UserID:       user.ID.String(),
			Username:     user.GitHubUsername,
			DisplayName:  user.FullName,
			AvatarURL:    user.AvatarURL,
			Country:      user.Country,
			TotalSeconds: totalSeconds,
			Text:         services.HumanDuration(totalSeconds),
		})
	}
	return services.RankLeaderboardEntries(entries), nil
}

func (s *Server) refreshLeaderboardCache(ctx context.Context, rangeName, language, country string) ([]services.LeaderboardEntry, error) {
	entries, err := s.leaderboardEntries(ctx, nil, rangeName, language, country)
	if err != nil {
		return nil, err
	}
	if s.LeaderboardCache != nil {
		if err := s.LeaderboardCache.Set(ctx, leaderboardCacheKey(rangeName, language, country), entries, leaderboardCacheTTL); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

func leaderboardLanguageSeconds(stats services.Stats, language string) (int, bool) {
	for _, row := range stats.Languages {
		if strings.EqualFold(row.Name, language) {
			return row.TotalSeconds, true
		}
	}
	return 0, false
}

func leaderboardCountryMatches(user db.User, country string) bool {
	country = strings.ToUpper(strings.TrimSpace(country))
	if country == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(user.Country), country)
}

func leaderboardCacheKey(rangeName, language, country string) string {
	parts := []string{rangeName}
	if strings.TrimSpace(language) != "" {
		parts = append(parts, "language:"+strings.ToLower(strings.TrimSpace(language)))
	}
	if strings.TrimSpace(country) != "" {
		parts = append(parts, "country:"+strings.ToLower(strings.TrimSpace(country)))
	}
	return strings.Join(parts, ":")
}

func leaderboardMeta(cached bool, rangeName, language, country string) map[string]any {
	meta := map[string]any{"cached": cached, "range": rangeName}
	if strings.TrimSpace(language) != "" {
		meta["language"] = strings.TrimSpace(language)
	}
	if strings.TrimSpace(country) != "" {
		meta["country"] = strings.ToUpper(strings.TrimSpace(country))
	}
	return meta
}

func (s *Server) applyCustomRules(ctx context.Context, user db.User, heartbeat services.Heartbeat) (services.Heartbeat, bool, error) {
	rules, err := s.Store.ListCustomRules(ctx, user.ID)
	if err != nil {
		return heartbeat, false, err
	}
	updated, deleted := services.ApplyCustomRules(heartbeat, serviceCustomRules(rules))
	return updated, deleted, nil
}

func serviceCustomRules(rules []db.CustomRule) []services.CustomRule {
	out := make([]services.CustomRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, services.CustomRule{
			ID:           rule.ID.String(),
			Action:       rule.Action,
			Source:       rule.Source,
			Operation:    rule.Operation,
			SourceValue:  rule.SourceValue,
			Priority:     rule.Priority,
			Destinations: rule.Destinations,
			CreatedAt:    rule.CreatedAt.Format(time.RFC3339),
			ModifiedAt:   rule.ModifiedAt.Format(time.RFC3339),
		})
	}
	return out
}

func validateCustomRules(rules []services.CustomRule) error {
	return services.ValidateCustomRules(rules)
}

func parseUUIDs(values []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, errors.New("invalid id")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

type oauthAuthorizationRequest struct {
	ResponseType string
	ClientID     string
	RedirectURI  string
	Scopes       []string
	State        string
}

func (s *Server) validOAuthAuthorizationRequest(c echo.Context) (oauthAuthorizationRequest, db.OAuthApp, error) {
	params := oauthAuthorizationRequest{
		ResponseType: strings.TrimSpace(c.FormValue("response_type")),
		ClientID:     strings.TrimSpace(c.FormValue("client_id")),
		RedirectURI:  strings.TrimSpace(c.FormValue("redirect_uri")),
		Scopes:       splitScopes(c.FormValue("scope")),
		State:        strings.TrimSpace(c.FormValue("state")),
	}
	if params.ResponseType == "" {
		params.ResponseType = strings.TrimSpace(c.QueryParam("response_type"))
	}
	if params.ClientID == "" {
		params.ClientID = strings.TrimSpace(c.QueryParam("client_id"))
	}
	if params.RedirectURI == "" {
		params.RedirectURI = strings.TrimSpace(c.QueryParam("redirect_uri"))
	}
	if len(params.Scopes) == 0 {
		params.Scopes = splitScopes(c.QueryParam("scope"))
	}
	if params.State == "" {
		params.State = strings.TrimSpace(c.QueryParam("state"))
	}
	if (params.ResponseType != "code" && params.ResponseType != "token") || params.ClientID == "" || params.RedirectURI == "" {
		return params, db.OAuthApp{}, errors.New("invalid OAuth authorization request")
	}
	app, err := s.Store.OAuthAppByClientID(c.Request().Context(), params.ClientID)
	if err != nil {
		return params, db.OAuthApp{}, errors.New("unknown OAuth client")
	}
	if !containsString(app.RedirectURIs, params.RedirectURI) {
		return params, db.OAuthApp{}, errors.New("redirect_uri is not registered for this OAuth app")
	}
	if len(params.Scopes) == 0 {
		params.Scopes = app.Scopes
	}
	for _, scope := range params.Scopes {
		if !oauthAppAllowsScope(app.Scopes, scope) {
			return params, db.OAuthApp{}, fmt.Errorf("scope %q is not allowed for this OAuth app", scope)
		}
	}
	return params, app, nil
}

func oauthAppAllowsScope(allowed []string, requested string) bool {
	for _, scope := range allowed {
		if scope == requested || strings.HasPrefix(requested, scope+".") {
			return true
		}
	}
	return false
}

func oauthClientCredentials(r *http.Request) (string, string) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(header), "basic ") {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header[len("Basic "):]))
		if err == nil {
			clientID, secret, ok := strings.Cut(string(raw), ":")
			if ok {
				return strings.TrimSpace(clientID), strings.TrimSpace(secret)
			}
		}
	}
	return strings.TrimSpace(r.FormValue("client_id")), strings.TrimSpace(r.FormValue("client_secret"))
}

func oauthTokenBody(result db.OAuthTokenResult) map[string]any {
	expiresIn := result.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int(time.Until(result.ExpiresAt).Seconds())
	}
	return map[string]any{
		"access_token":  result.AccessToken,
		"refresh_token": result.RefreshToken,
		"token_type":    "Bearer",
		"expires_in":    expiresIn,
		"scope":         strings.Join(result.Scopes, " "),
		"uid":           result.User.ID.String(),
	}
}

func redirectWithOAuthParams(c echo.Context, rawURL string, values map[string]string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return c.JSON(http.StatusBadRequest, oauthError("invalid_request"))
	}
	query := u.Query()
	for key, value := range values {
		if value != "" {
			query.Set(key, value)
		}
	}
	u.RawQuery = query.Encode()
	return c.Redirect(http.StatusFound, u.String())
}

func redirectWithOAuthFragment(c echo.Context, rawURL string, values map[string]string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return c.JSON(http.StatusBadRequest, oauthError("invalid_request"))
	}
	fragment := url.Values{}
	for key, value := range values {
		if value != "" {
			fragment.Set(key, value)
		}
	}
	u.Fragment = fragment.Encode()
	return c.Redirect(http.StatusFound, u.String())
}

func oauthTokenFragment(result db.OAuthTokenResult, state string) map[string]string {
	expiresIn := result.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int(time.Until(result.ExpiresAt).Seconds())
	}
	return map[string]string{
		"access_token": result.AccessToken,
		"token_type":   "Bearer",
		"expires_in":   strconv.Itoa(expiresIn),
		"scope":        strings.Join(result.Scopes, " "),
		"uid":          result.User.ID.String(),
		"state":        state,
	}
}

func splitScopes(raw string) []string {
	fields := strings.Fields(strings.ReplaceAll(raw, ",", " "))
	seen := map[string]bool{}
	scopes := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		scopes = append(scopes, field)
	}
	return scopes
}

func readImportBody(c echo.Context) ([]byte, error) {
	if strings.HasPrefix(c.Request().Header.Get(echo.HeaderContentType), "multipart/form-data") {
		file, err := c.FormFile("file")
		if err != nil {
			return nil, errors.New("missing import file")
		}
		src, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer src.Close()
		return readAllImportBody(src)
	}
	return readAllImportBody(c.Request().Body)
}

func readAllImportBody(src io.Reader) ([]byte, error) {
	limited := io.LimitReader(src, maxImportBodyBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxImportBodyBytes {
		return nil, fmt.Errorf("import file is too large; limit is %d MiB", maxImportBodyBytes>>20)
	}
	return raw, nil
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func writePublicPayload(c echo.Context, payload any) error {
	callback := strings.TrimSpace(c.QueryParam("callback"))
	if callback == "" {
		return c.JSON(http.StatusOK, payload)
	}
	if !validJSONPCallback(callback) {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSONP callback"))
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.Blob(http.StatusOK, "application/javascript; charset=utf-8", []byte(callback+"("+string(body)+");"))
}

func validJSONPCallback(value string) bool {
	if value == "" {
		return false
	}
	parts := strings.Split(value, ".")
	for _, part := range parts {
		if part == "" {
			return false
		}
		for i, r := range part {
			switch {
			case r >= 'a' && r <= 'z':
			case r >= 'A' && r <= 'Z':
			case r == '_' || r == '$':
			case i > 0 && r >= '0' && r <= '9':
			default:
				return false
			}
		}
	}
	return true
}

func publicUser(user db.User) map[string]any {
	username := user.PublicUsername
	if username == "" {
		username = user.GitHubUsername
	}
	name := user.PublicDisplayName
	if name == "" && user.PublicGitHubLink {
		name = user.FullName
	}
	profile := user.PublicProfile
	layout := profile.Layout
	if layout == "" {
		layout = "terminal"
	}
	payload := map[string]any{
		"id":       user.ID.String(),
		"username": username,
		"name":     name,
		"layout":   layout,
		"permissions": map[string]any{
			"total_time":         user.PublicShowTotalTime,
			"projects":           user.PublicShowProjects,
			"project_visibility": user.PublicProjectVisibility,
			"languages":          user.PublicShowLanguages,
			"editors":            user.PublicShowEditors,
			"machines":           user.PublicShowMachines,
			"operating_systems":  user.PublicShowOS,
			"categories":         user.PublicShowCategories,
			"ai":                 user.PublicShowAI,
			"summaries":          user.PublicShowSummaries,
			"github":             user.PublicGitHubLink,
		},
	}
	if user.PublicGitHubLink {
		payload["github_username"] = user.GitHubUsername
		payload["github_url"] = "https://github.com/" + user.GitHubUsername
		payload["avatar_url"] = user.AvatarURL
	}
	// Personal-info fields are public by default; an explicit "private"
	// visibility entry hides one. (Empty fields are simply omitted.)
	vis := func(key string) bool { return profile.Visibility[key] != "private" }
	setIf := func(key, value string) {
		if value != "" && vis(key) {
			payload[key] = value
		}
	}
	setIf("bio", profile.Bio)
	setIf("location", profile.Location)
	setIf("website_url", profile.WebsiteURL)
	setIf("linkedin_url", profile.LinkedInURL)
	setIf("mastodon_url", profile.MastodonURL)
	setIf("pronouns", profile.Pronouns)
	setIf("company", profile.Company)
	setIf("role", profile.Role)
	if profile.TwitterUsername != "" && vis("twitter") {
		payload["twitter_username"] = profile.TwitterUsername
		payload["twitter_url"] = "https://twitter.com/" + profile.TwitterUsername
	}
	if profile.Location == "" && user.Country != "" && vis("location") {
		payload["country"] = user.Country
	}
	if profile.AvailableForHire && vis("hireable") {
		payload["available_for_hire"] = true
	}
	if profile.EmailPublic && user.Email != "" && vis("email") {
		payload["email"] = user.Email
	}
	return payload
}

func errorBody(message string) map[string][]string {
	return map[string][]string{"errors": []string{message}}
}

func dataArray[T any](items []T) map[string]any {
	return map[string]any{"data": nonNilSlice(items)}
}

func nonNilSlice[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func wakaError(message string) map[string]string {
	return map[string]string{"error": message}
}

func oauthError(message string) map[string]string {
	return map[string]string{"error": message}
}

func oauthErrorHTML(message string) string {
	return "<!doctype html><title>OAuth error</title><body style=\"background:#09090b;color:#f4f4f5;font-family:ui-sans-serif,system-ui,sans-serif\"><main style=\"padding:32px\"><h1>OAuth error</h1><p>" + html.EscapeString(message) + "</p></main></body>"
}

func dayRange(date string) (float64, float64, error) {
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0, 0, fmt.Errorf("date must use YYYY-MM-DD")
	}
	return float64(day.Unix()), float64(day.AddDate(0, 0, 1).Unix()), nil
}

func userLocation(user db.User) *time.Location {
	if user.Timezone != "" {
		if location, err := time.LoadLocation(user.Timezone); err == nil {
			return location
		}
	}
	return time.UTC
}

func dayRangeInLocation(date string, location *time.Location, now time.Time) (time.Time, time.Time, error) {
	if location == nil {
		location = time.UTC
	}
	if now.IsZero() {
		now = time.Now()
	}
	if date == "" {
		date = now.In(location).Format("2006-01-02")
	}
	day, err := time.ParseInLocation("2006-01-02", date, location)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("date must use YYYY-MM-DD")
	}
	return day, day.AddDate(0, 0, 1), nil
}

func dateRange(startValue, endValue string) (time.Time, time.Time, error) {
	if startValue == "" {
		startValue = time.Now().UTC().AddDate(0, 0, -6).Format("2006-01-02")
	}
	if endValue == "" {
		endValue = time.Now().UTC().Format("2006-01-02")
	}
	start, err := time.Parse("2006-01-02", startValue)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("start must use YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", endValue)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("end must use YYYY-MM-DD")
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("end must be on or after start")
	}
	if end.Sub(start) > 370*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("date range is too large")
	}
	return start, end, nil
}

func dateRangeInLocation(startValue, endValue string, location *time.Location, now time.Time) (time.Time, time.Time, error) {
	if location == nil {
		location = time.UTC
	}
	if now.IsZero() {
		now = time.Now()
	}
	if startValue == "" {
		startValue = now.In(location).AddDate(0, 0, -6).Format("2006-01-02")
	}
	if endValue == "" {
		endValue = now.In(location).Format("2006-01-02")
	}
	start, err := time.ParseInLocation("2006-01-02", startValue, location)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("start must use YYYY-MM-DD")
	}
	end, err := time.ParseInLocation("2006-01-02", endValue, location)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("end must use YYYY-MM-DD")
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("end must be on or after start")
	}
	if end.Sub(start) > 370*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("date range is too large")
	}
	return start, end, nil
}

func (s *Server) frontendURL(path string) string {
	base := strings.TrimSpace(s.Config.WebBaseURL)
	if base == "" {
		base = "http://localhost:3000"
	}
	u, err := url.Parse(base)
	if err != nil {
		return path
	}
	u.Path = path
	return u.String()
}

func compactOrigins(values ...string) []string {
	seen := map[string]bool{}
	origins := make([]string, 0, len(values))
	for _, value := range values {
		origin := strings.TrimRight(strings.TrimSpace(value), "/")
		if origin == "" || seen[origin] {
			continue
		}
		seen[origin] = true
		origins = append(origins, origin)
	}
	return origins
}
