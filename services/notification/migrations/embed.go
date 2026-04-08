// Package migrations provides embedded SQL migration files for the notification service.
package migrations

import "embed"

//go:embed *.sql
// FS contains the embedded SQL migration files for the notification service.
var FS embed.FS
