ALTER TABLE reviews ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now();

DROP INDEX IF EXISTS idx_reviews_product_id;
CREATE INDEX idx_reviews_product_id_created_at ON reviews (product_id, created_at DESC, id);
