-- Captures the information about the cids shared or requested over bitswap.
CREATE TABLE shared_cids (
    timestamp  DateTime64(3), -- Timestamp of the event, with millisecond precision.
    direction  String,        -- Direction of the event: "sent" or "received".
    cid        String,        -- Content Identifier (CID) involved in the event.
    peer_id    String,        -- Peer ID of the remote node in the event.
    type       String         -- Type of Bitswap message: "want", "have", "dont-have", or "block".
) ENGINE ReplicatedReplacingMergeTree(timestamp)
    PRIMARY KEY (timestamp, cid)
