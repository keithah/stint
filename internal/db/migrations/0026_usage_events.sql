-- Canonical AI usage events from the local collector (one row per priced
-- request, cache granularity preserved). Separate from heartbeat AI fields,
-- which stay the source of truth for line-change/session metrics and WakaTime
-- compatibility. Request-level dedup is enforced by the (user_id, event_id) PK.
CREATE TABLE IF NOT EXISTS usage_events (
  user_id                uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  event_id               text NOT NULL,
  message_id             text,
  request_id             text,
  agent                  text NOT NULL,
  session_id             text NOT NULL DEFAULT '',
  project                text,
  model                  text NOT NULL DEFAULT '',
  input_tokens           bigint NOT NULL DEFAULT 0,
  output_tokens          bigint NOT NULL DEFAULT 0,
  cache_create_5m_tokens bigint NOT NULL DEFAULT 0,
  cache_create_1h_tokens bigint NOT NULL DEFAULT 0,
  cache_read_tokens      bigint NOT NULL DEFAULT 0,
  reasoning_tokens       bigint NOT NULL DEFAULT 0,
  cost_usd_provided      double precision,
  billing_type           text,
  ts                     timestamptz NOT NULL,
  tz_offset_minutes      int NOT NULL DEFAULT 0,
  created_at             timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, event_id)
);

CREATE INDEX IF NOT EXISTS usage_events_user_ts_idx ON usage_events (user_id, ts);
CREATE INDEX IF NOT EXISTS usage_events_user_agent_ts_idx ON usage_events (user_id, agent, ts);
CREATE INDEX IF NOT EXISTS usage_events_user_model_ts_idx ON usage_events (user_id, model, ts);
