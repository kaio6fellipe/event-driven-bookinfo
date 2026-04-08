// Package migrations provides embedded SQL migration files for the reviews service.
package migrations

import "embed"

// FS contains the embedded SQL migration files for the reviews service.
//
//go:embed *.sql
var FS embed.FS
