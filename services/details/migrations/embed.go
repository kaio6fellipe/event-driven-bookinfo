// Package migrations provides embedded SQL migration files for the details service.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
