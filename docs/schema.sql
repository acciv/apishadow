CREATE TABLE IF NOT EXISTS apishadow_diffs (
    id String,
    method LowCardinality(String),
    path String,
    status_primary UInt16,
    status_shadow UInt16,
    score UInt8,
    verdict LowCardinality(String),
    diff_json String,
    ingested_at DateTime DEFAULT now()
) ENGINE = MergeTree
ORDER BY (path, ingested_at, id);
