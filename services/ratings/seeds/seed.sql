INSERT INTO ratings (id, product_id, reviewer, stars)
VALUES
  ('r0001', 'd0001', 'reviewer1', 5),
  ('r0002', 'd0002', 'reviewer2', 4),
  ('r0003', 'd0003', 'reviewer3', 3)
ON CONFLICT (id) DO NOTHING;
