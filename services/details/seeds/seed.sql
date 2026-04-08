INSERT INTO details (id, title, author, year, type, pages, publisher, language, isbn10, isbn13)
VALUES
  ('d0001', 'The Comedy of Errors', 'William Shakespeare', 1595, 'paperback', 120, 'Penguin Classics', 'English', '0140714898', '9780140714890'),
  ('d0002', 'The Odyssey', 'Homer', -800, 'hardcover', 560, 'Oxford University Press', 'English', '0199536783', '9780199536788'),
  ('d0003', 'Clean Code', 'Robert C. Martin', 2008, 'paperback', 464, 'Prentice Hall', 'English', '0132350882', '9780132350884')
ON CONFLICT (id) DO NOTHING;
