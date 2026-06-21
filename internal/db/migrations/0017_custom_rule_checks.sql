DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'custom_rules_action_check'
      AND conrelid = 'custom_rules'::regclass
  ) THEN
    ALTER TABLE custom_rules
      ADD CONSTRAINT custom_rules_action_check
      CHECK (action IN ('change', 'delete'));
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'custom_rules_operation_check'
      AND conrelid = 'custom_rules'::regclass
  ) THEN
    ALTER TABLE custom_rules
      ADD CONSTRAINT custom_rules_operation_check
      CHECK (operation IN ('equals', 'contains', 'starts_with', 'ends_with', 'regex', 'matches'));
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'custom_rules_required_text_check'
      AND conrelid = 'custom_rules'::regclass
  ) THEN
    ALTER TABLE custom_rules
      ADD CONSTRAINT custom_rules_required_text_check
      CHECK (
        btrim(source) <> ''
        AND btrim(operation) <> ''
        AND btrim(source_value) <> ''
      );
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'custom_rules_priority_check'
      AND conrelid = 'custom_rules'::regclass
  ) THEN
    ALTER TABLE custom_rules
      ADD CONSTRAINT custom_rules_priority_check
      CHECK (priority > 0);
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'custom_rule_destinations_required_text_check'
      AND conrelid = 'custom_rule_destinations'::regclass
  ) THEN
    ALTER TABLE custom_rule_destinations
      ADD CONSTRAINT custom_rule_destinations_required_text_check
      CHECK (btrim(destination) <> '' AND btrim(destination_value) <> '');
  END IF;
END $$;
