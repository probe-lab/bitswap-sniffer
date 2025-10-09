-- DO NOT EDIT: This file was generated with: just generate-local-clickhouse-migrations

-- Extend the share_cids table to have an extra column called "origin"
-- which will define if the CID came from bitswap or from the dht
ALTER TABLE shared_cids ADD COLUMN IF NOT EXISTS origin LowCardinality(String) DEFAULT 'bitswap';
