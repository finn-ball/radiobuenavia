package main

import "fmt"

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "local"
)

func versionString() string {
	return fmt.Sprintf("rbv version %s (commit=%s date=%s builtBy=%s)", version, commit, date, builtBy)
}

func runVersion() {
	fmt.Println(versionString())
}
