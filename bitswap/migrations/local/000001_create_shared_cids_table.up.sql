-- DO NOT EDIT: This file was generated with: just generate-local-clickhouse-migrations

-- Captures the information about the CIDs shared or requested over Bitswap.
CREATE TABLE shared_cids (
    timestamp       DateTime,                 -- Timestamp of the event, with millisecond precision.
    direction       LowCardinality(String),   -- Direction of the event: "sent" or "received". Using LowCardinality for efficient storage & lookup.
    cid             String,                   -- Content Identifier (CID) involved in the event.
    producer_id     String,                   -- Peer ID of the producer bitswap client.
    peer_id         String,                   -- Peer ID of the remote node in the event.
    msg_type        LowCardinality(String)    -- Message type: "want", "have", "dont-have", or "block". Renamed from 'type' for clarity.
)
ENGINE = MergeTree()
PARTITION BY toMonth(timestamp)               -- Partition data by month, based on the event timestamp.
PRIMARY KEY (timestamp)                       -- Use timestamp as primary key for sorting and efficient range queries.
TTL timestamp + INTERVAL 180 DAY              -- TTL to expire data older than 180 days.
