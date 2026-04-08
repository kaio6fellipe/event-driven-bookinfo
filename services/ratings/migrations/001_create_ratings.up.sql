CREATE TABLE IF NOT EXISTS ratings (
    id         TEXT PRIMARY KEY,
    product_id TEXT NOT NULL,
    reviewer   TEXT NOT NULL,
    stars      INTEGER NOT NULL CHECK (stars >= 1 AND stars <= 5)
);

CREATE INDEX idx_ratings_product_id ON ratings (product_id);
