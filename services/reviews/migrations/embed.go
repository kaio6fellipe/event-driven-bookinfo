// Package migrations provides embedded SQL migration files for the reviews service.
package migrations

import "embed"

//go:embed *.sql
// FS contains the embedded SQL migration files for the reviews service.
var FS embed.FS
