-- User-defined custom model prices for private/proxied/unpriced models (e.g.
-- opencode/big-pickle) that the bundled LiteLLM table does not cover. Prices are
-- stored per-million tokens (human-friendly) and converted to per-token when
-- fed into the pricing engine as overrides. Mirrors LiteLLM custom registration.
CREATE TABLE IF NOT EXISTS custom_pricing (
  user_id                      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  model                        text NOT NULL,
  input_per_million_usd        double precision NOT NULL DEFAULT 0,
  output_per_million_usd       double precision NOT NULL DEFAULT 0,
  cache_write_per_million_usd  double precision NOT NULL DEFAULT 0,
  cache_read_per_million_usd   double precision NOT NULL DEFAULT 0,
  created_at                   timestamptz NOT NULL DEFAULT now(),
  modified_at                  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, model),
  CONSTRAINT custom_pricing_model_check CHECK (btrim(model) <> ''),
  CONSTRAINT custom_pricing_prices_check CHECK (
    input_per_million_usd >= 0 AND output_per_million_usd >= 0 AND
    cache_write_per_million_usd >= 0 AND cache_read_per_million_usd >= 0
  )
);
