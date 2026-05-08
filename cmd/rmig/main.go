package main

import (
	"os"

	"reporting-db-migrations/internal/app"
)

var (
	version = "0.1.0-dev"
	commit  = "dev"
)

func main() {
	os.Exit(app.Run(os.Args, app.BuildInfo{Version: version, Commit: commit}))
}
