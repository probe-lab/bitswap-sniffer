-- DO NOT EDIT: This file was generated with: just generate-local-clickhouse-migrations

-- Drop the origin column from the table
ALTER TABLE shared_cids DROP COLUMN origin;
