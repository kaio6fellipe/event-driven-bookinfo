DROP INDEX IF EXISTS idx_reviews_product_id_created_at;
CREATE INDEX idx_reviews_product_id ON reviews (product_id);
ALTER TABLE reviews DROP COLUMN IF EXISTS created_at;
