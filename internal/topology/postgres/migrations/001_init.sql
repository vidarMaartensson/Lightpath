-- Network topology. Kept deliberately close to the domain model:
-- nodes and links form the graph, customers attach to one or more cabinets.

CREATE TABLE IF NOT EXISTS isps (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS nodes (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    area TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('CORE', 'AGGREGATION', 'ACCESS', 'CABINET')),
    x    DOUBLE PRECISION NOT NULL,
    y    DOUBLE PRECISION NOT NULL
);

CREATE TABLE IF NOT EXISTS links (
    id   TEXT PRIMARY KEY,
    a_id TEXT NOT NULL REFERENCES nodes (id),
    b_id TEXT NOT NULL REFERENCES nodes (id),
    -- Order matters: a node's first listed uplink is its primary path.
    position INT NOT NULL,
    CHECK (a_id <> b_id)
);

CREATE TABLE IF NOT EXISTS customers (
    id       TEXT PRIMARY KEY,
    isp_id   TEXT NOT NULL REFERENCES isps (id),
    business BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS customer_attachments (
    customer_id TEXT NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    node_id     TEXT NOT NULL REFERENCES nodes (id),
    -- 0 is the primary attachment.
    position    INT  NOT NULL,
    PRIMARY KEY (customer_id, node_id)
);

CREATE INDEX IF NOT EXISTS customer_attachments_node_idx ON customer_attachments (node_id);
