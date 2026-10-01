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

//go:embed migrations/004_up.sql
var migration004Up string

//go:embed migrations/004_down.sql
var migration004Down string

//go:embed migrations/005_up.sql
var migration005Up string

//go:embed migrations/005_down.sql
var migration005Down string

//go:embed migrations/006_up.sql
var migration006Up string

//go:embed migrations/006_down.sql
var migration006Down string

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
		{Version: "004", Up: migration004Up, Down: migration004Down},
		{Version: "005", Up: migration005Up, Down: migration005Down},
		{Version: "006", Up: migration006Up, Down: migration006Down},
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
