# Local Development with PostgreSQL — Design Spec

## Context

The root `docker-compose.yml` currently uses `STORAGE_BACKEND=memory` for all services. This means local dev doesn't exercise the postgres code path, and there's no seed data to populate the UI. Developers need separate make targets to manage migrations and seed data, and the whole flow must be idempotent — running `make run` repeatedly should always converge to a healthy, seeded environment.

---

## Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Default storage backend for `make run` | postgres | Local dev should match production. Memory backend remains for `make e2e` and unit tests |
| Seed data format | SQL files with `INSERT ... ON CONFLICT (id) DO NOTHING` | Idempotent, fast, no services required to be running, follows per-service pattern |
| Seed data location | `services/<name>/seeds/seed.sql` | Co-located with service, same pattern as migrations |
| Seed execution | `psql` via `docker compose exec postgres` | Direct SQL, no Go tooling needed |
| Migration execution | At service startup via `RUN_MIGRATIONS=true` | Already implemented, `golang-migrate` skips applied migrations |
| Data persistence | Named volume `pgdata` on postgres container | Data survives `make stop`/`make run` cycles |
| Fresh start | `make clean-data` runs `docker compose down -v` | Removes volume, next `make run` recreates everything |
| Init databases script location | `dev/init-databases.sql` (shared at root) | Reusable by docker-compose.yml, separate from e2e test copy |

---

## docker-compose.yml Changes

Add postgres service and switch all backend services to postgres backend:

```yaml
services:
  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: bookinfo
      POSTGRES_PASSWORD: bookinfo
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./dev/init-databases.sql:/docker-entrypoint-initdb.d/init.sql
    healthcheck:
      test: ["CMD", "pg_isready", "-U", "bookinfo"]
      interval: 2s
      timeout: 5s
      retries: 5

  ratings:
    # ... existing build config ...
    environment:
      SERVICE_NAME: ratings
      STORAGE_BACKEND: postgres
      DATABASE_URL: postgres://bookinfo:bookinfo@postgres:5432/bookinfo_ratings?sslmode=disable
      RUN_MIGRATIONS: "true"
    depends_on:
      postgres:
        condition: service_healthy

  # details, reviews, notification follow same pattern
  # productpage unchanged (aggregator, no storage)

volumes:
  pgdata:
```

---

## Init Databases Script

`dev/init-databases.sql` — creates per-service databases on first postgres boot. Idempotent because postgres `initdb.d` scripts only run when the data directory is empty (first boot or after volume removal).

```sql
CREATE DATABASE bookinfo_ratings;
CREATE DATABASE bookinfo_details;
CREATE DATABASE bookinfo_reviews;
CREATE DATABASE bookinfo_notification;
```

---

## Seed Data

Per-service SQL files at `services/<name>/seeds/seed.sql`. All use hardcoded UUIDs and `ON CONFLICT (id) DO NOTHING` for idempotency.

### details — 3 books

```sql
INSERT INTO details (id, title, author, year, type, pages, publisher, language, isbn10, isbn13)
VALUES
  ('d0001', 'The Comedy of Errors', 'William Shakespeare', 1595, 'paperback', 120, 'Penguin Classics', 'English', '0140714898', '9780140714890'),
  ('d0002', 'The Odyssey', 'Homer', -800, 'hardcover', 560, 'Oxford University Press', 'English', '0199536783', '9780199536788'),
  ('d0003', 'Clean Code', 'Robert C. Martin', 2008, 'paperback', 464, 'Prentice Hall', 'English', '0132350882', '9780132350884')
ON CONFLICT (id) DO NOTHING;
```

### ratings — 3 ratings (one per book)

```sql
INSERT INTO ratings (id, product_id, reviewer, stars)
VALUES
  ('r0001', 'd0001', 'reviewer1', 5),
  ('r0002', 'd0002', 'reviewer2', 4),
  ('r0003', 'd0003', 'reviewer3', 3)
ON CONFLICT (id) DO NOTHING;
```

### reviews — 3 reviews (one per book)

```sql
INSERT INTO reviews (id, product_id, reviewer, text)
VALUES
  ('v0001', 'd0001', 'reviewer1', 'A brilliant comedy of mistaken identities. Shakespeare at his lightest and most entertaining.'),
  ('v0002', 'd0002', 'reviewer2', 'An epic journey that still resonates thousands of years later. Essential reading.'),
  ('v0003', 'd0003', 'reviewer3', 'Practical advice on writing clean, maintainable code. Every developer should read this.')
ON CONFLICT (id) DO NOTHING;
```

### notification — 3 notifications

```sql
INSERT INTO notifications (id, recipient, channel, subject, body, status, sent_at)
VALUES
  ('n0001', 'admin@bookinfo.local', 'email', 'New book added', 'The Comedy of Errors has been added to the catalog.', 'sent', '2026-01-15T10:00:00Z'),
  ('n0002', 'admin@bookinfo.local', 'email', 'New book added', 'The Odyssey has been added to the catalog.', 'sent', '2026-01-15T10:05:00Z'),
  ('n0003', 'admin@bookinfo.local', 'email', 'New book added', 'Clean Code has been added to the catalog.', 'sent', '2026-01-15T10:10:00Z')
ON CONFLICT (id) DO NOTHING;
```

---

## Makefile Targets

| Target | Description | Idempotent |
|---|---|---|
| `make run` | Build, start (postgres + services), wait healthy, seed, show summary | Yes |
| `make stop` | `docker compose down` — keeps postgres volume | Yes |
| `make clean-data` | `docker compose down -v` — removes postgres volume for fresh start | Yes |
| `make seed` | Run all `services/*/seeds/seed.sql` via psql on postgres container | Yes |
| `make migrate` | Restart backend services to re-trigger startup migrations | Yes |
| `make run-logs` | `docker compose logs -f` (unchanged) | N/A |

### `make run` flow

1. `docker compose up -d --build`
2. Services start, each runs migrations at boot
3. Health check polling (15s timeout per service)
4. `make seed` (idempotent SQL inserts)
5. Status summary + app URL

### `make migrate` implementation

Restarts only the 4 backend services (not postgres, not productpage) which re-triggers migration execution at startup:

```
docker compose restart ratings details reviews notification
```

### `make seed` implementation

Iterates over services that have seed files and runs them via psql:

```
for each service with seeds/seed.sql:
  docker compose exec -T postgres psql -U bookinfo -d bookinfo_<service> -f /dev/stdin < seed.sql
```

---

## Idempotency Guarantees

| Operation | Why it's idempotent |
|---|---|
| `docker compose up -d` | Creates only missing containers, skips existing |
| `init-databases.sql` | Only runs on empty data dir (first postgres boot) |
| Migrations (`golang-migrate`) | Tracks applied version, skips if current |
| Seed data | `INSERT ... ON CONFLICT (id) DO NOTHING` |
| `docker compose down` | No-op if nothing running |
| `docker compose down -v` | No-op if no volume exists |

---

## Files Summary

### New Files

```
dev/init-databases.sql
services/ratings/seeds/seed.sql
services/details/seeds/seed.sql
services/reviews/seeds/seed.sql
services/notification/seeds/seed.sql
```

### Modified Files

```
docker-compose.yml          # Add postgres, switch to postgres backend, add pgdata volume
Makefile                    # Add seed, migrate, clean-data targets; update run to include seed
```
