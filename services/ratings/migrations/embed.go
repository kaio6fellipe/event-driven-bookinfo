// Package migrations provides embedded SQL migration files for the ratings service.
package migrations

import "embed"

//go:embed *.sql
// FS contains the embedded SQL migration files for the ratings service.
var FS embed.FS
