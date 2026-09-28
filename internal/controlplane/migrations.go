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
