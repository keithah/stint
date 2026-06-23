-- Per-agent billing-mode overrides. A user declares which agents are flat-rate
-- subscription (marginal cost $0, since the spend is already covered by the
-- subscription) vs metered API (marginal = equivalent-API cost). The pricing
-- engine zeroes marginal cost for subscription usage; this table overrides, at
-- view time, which agent's events count as subscription regardless of the
-- billing_type the collecting adapter stamped on the stored event.
CREATE TABLE IF NOT EXISTS billing_prefs (
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  agent         text NOT NULL,
  billing_type  text NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  modified_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, agent)
);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'billing_prefs_agent_check'
  ) THEN
    ALTER TABLE billing_prefs
      ADD CONSTRAINT billing_prefs_agent_check CHECK (btrim(agent) <> '');
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'billing_prefs_billing_type_check'
  ) THEN
    ALTER TABLE billing_prefs
      ADD CONSTRAINT billing_prefs_billing_type_check CHECK (billing_type IN ('api', 'subscription'));
  END IF;
END$$;
