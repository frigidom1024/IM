// Package mysqlscripts contains the SQL shipped with the migration binary.
package mysqlscripts

import "embed"

// Files is independent of the process working directory.
//
//go:embed init.sql migrations/*.up.sql
var Files embed.FS
