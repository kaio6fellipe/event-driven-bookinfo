// Package migrations provides embedded SQL migration files for the details service.
package migrations

import "embed"

//go:embed *.sql
// FS contains the embedded SQL migration files for the details service.
var FS embed.FS
