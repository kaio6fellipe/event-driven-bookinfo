CREATE TABLE IF NOT EXISTS reviews (
    id         TEXT PRIMARY KEY,
    product_id TEXT NOT NULL,
    reviewer   TEXT NOT NULL,
    text       TEXT NOT NULL
);

CREATE INDEX idx_reviews_product_id ON reviews (product_id);
