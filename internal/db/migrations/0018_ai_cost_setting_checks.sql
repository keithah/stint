DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'ai_cost_settings_agent_check'
      AND conrelid = 'ai_cost_settings'::regclass
  ) THEN
    ALTER TABLE ai_cost_settings
      ADD CONSTRAINT ai_cost_settings_agent_check
      CHECK (btrim(agent) <> '');
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'ai_cost_settings_costs_check'
      AND conrelid = 'ai_cost_settings'::regclass
  ) THEN
    ALTER TABLE ai_cost_settings
      ADD CONSTRAINT ai_cost_settings_costs_check
      CHECK (input_cost_per_million_cents >= 0 AND output_cost_per_million_cents >= 0);
  END IF;
END $$;
