// Package migrations provides embedded SQL migration files for the reviews service.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
