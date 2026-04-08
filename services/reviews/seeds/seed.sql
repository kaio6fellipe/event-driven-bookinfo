INSERT INTO reviews (id, product_id, reviewer, text)
VALUES
  ('v0001', 'd0001', 'reviewer1', 'A brilliant comedy of mistaken identities. Shakespeare at his lightest and most entertaining.'),
  ('v0002', 'd0002', 'reviewer2', 'An epic journey that still resonates thousands of years later. Essential reading.'),
  ('v0003', 'd0003', 'reviewer3', 'Practical advice on writing clean, maintainable code. Every developer should read this.')
ON CONFLICT (id) DO NOTHING;
