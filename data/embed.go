package data

import "embed"

// MigrationsFS contains all SQL migration files embedded at compile time.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// ActionsFS contains all action definition JSON files embedded at compile time.
// The files are organized by platform: actions/<platform>/<ACTION_TYPE>.json
//
//go:embed actions
var ActionsFS embed.FS
