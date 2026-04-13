// Package migrations provides embedded SQL migration files for the dlqueue service.
package migrations

import "embed"

// FS contains the embedded SQL migration files for the dlqueue service.
//
//go:embed *.sql
var FS embed.FS
