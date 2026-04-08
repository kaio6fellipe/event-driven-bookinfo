CREATE TABLE IF NOT EXISTS details (
    id        TEXT PRIMARY KEY,
    title     TEXT NOT NULL,
    author    TEXT NOT NULL,
    year      INTEGER NOT NULL,
    type      TEXT NOT NULL DEFAULT '',
    pages     INTEGER NOT NULL,
    publisher TEXT NOT NULL DEFAULT '',
    language  TEXT NOT NULL DEFAULT '',
    isbn10    TEXT NOT NULL DEFAULT '',
    isbn13    TEXT NOT NULL DEFAULT ''
);
