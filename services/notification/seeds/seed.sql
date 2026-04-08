INSERT INTO notifications (id, recipient, channel, subject, body, status, sent_at)
VALUES
  ('n0001', 'admin@bookinfo.local', 'email', 'New book added', 'The Comedy of Errors has been added to the catalog.', 'sent', '2026-01-15T10:00:00Z'),
  ('n0002', 'admin@bookinfo.local', 'email', 'New book added', 'The Odyssey has been added to the catalog.', 'sent', '2026-01-15T10:05:00Z'),
  ('n0003', 'admin@bookinfo.local', 'email', 'New book added', 'Clean Code has been added to the catalog.', 'sent', '2026-01-15T10:10:00Z')
ON CONFLICT (id) DO NOTHING;
