DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'custom_rules_source_check'
      AND conrelid = 'custom_rules'::regclass
  ) THEN
    ALTER TABLE custom_rules
      ADD CONSTRAINT custom_rules_source_check
      CHECK (lower(btrim(source)) IN ('entity', 'type', 'category', 'project', 'branch', 'language', 'editor', 'operating_system'));
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'custom_rule_destinations_destination_check'
      AND conrelid = 'custom_rule_destinations'::regclass
  ) THEN
    ALTER TABLE custom_rule_destinations
      ADD CONSTRAINT custom_rule_destinations_destination_check
      CHECK (lower(btrim(destination)) IN ('entity', 'type', 'category', 'project', 'branch', 'language', 'editor', 'operating_system'));
  END IF;
END $$;
