package controlplane

import _ "embed"

//go:embed migrations/001_up.sql
var migration001Up string

//go:embed migrations/001_down.sql
var migration001Down string

//go:embed migrations/002_up.sql
var migration002Up string

//go:embed migrations/002_down.sql
var migration002Down string

//go:embed migrations/003_up.sql
var migration003Up string

//go:embed migrations/003_down.sql
var migration003Down string

type Migration struct {
	Version string
	Up      string
	Down    string
}

func ControlPlaneMigrations() []Migration {
	return []Migration{
		{Version: "001", Up: migration001Up, Down: migration001Down},
		{Version: "002", Up: migration002Up, Down: migration002Down},
		{Version: "003", Up: migration003Up, Down: migration003Down},
	}
}

func RequiredMigrationVersions() []string {
	migrations := ControlPlaneMigrations()

	versions := make([]string, 0, len(migrations))
	for _, migration := range migrations {
		versions = append(versions, migration.Version)
	}

	return versions
}
