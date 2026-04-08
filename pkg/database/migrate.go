package database

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// RunMigrations runs all pending up-migrations using the given embedded filesystem.
// The databaseURL must be a valid PostgreSQL connection string.
// The migrations fs.FS should contain .up.sql and .down.sql files.
func RunMigrations(databaseURL string, migrations fs.FS) error {
	source, err := iofs.New(migrations, ".")
	if err != nil {
		return fmt.Errorf("creating migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, convertToPgxURL(databaseURL))
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		_ = srcErr
		_ = dbErr
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}

// convertToPgxURL converts a standard postgres:// URL to the pgx5:// scheme
// required by the golang-migrate pgx/v5 driver.
func convertToPgxURL(databaseURL string) string {
	if len(databaseURL) >= 11 && databaseURL[:11] == "postgres://" {
		return "pgx5://" + databaseURL[11:]
	}
	if len(databaseURL) >= 14 && databaseURL[:14] == "postgresql://" {
		return "pgx5://" + databaseURL[14:]
	}
	return databaseURL
}
