ALTER TABLE data_dumps
  ALTER COLUMN status SET DEFAULT 'Pending',
  ALTER COLUMN percent_complete SET DEFAULT 0,
  ALTER COLUMN is_processing SET DEFAULT true;
