package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/Songmu/skillsmith"
	"github.com/Tocyuki/rdq/skills"
)

// dispatchSkills handles `rdq skills ...`. It runs before Kong parses
// os.Args (see main.go) because the skills subcommand does not need AWS
// credentials and skillsmith parses its own argv via the stdlib `flag`
// package; routing through Kong would just be double-parsing.
//
// skillsmith.Smith.Run writes its own diagnostic output to ErrWriter
// (os.Stderr) before returning, so we deliberately do NOT re-print its
// returned error — only translate it to a non-zero exit. Constructor
// errors (skillsmith.New) happen before any output and DO get an
// "rdq:" prefix.
func dispatchSkills(argv []string) {
	s, err := skillsmith.New("rdq", versionString(), skills.FS)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rdq:", err)
		os.Exit(1)
	}
	if err := s.Run(context.Background(), argv[2:]); err != nil {
		os.Exit(1)
	}
}

// versionString returns the rdq module version for skillsmith's per-skill
// metadata file (.skillsmith.json). For binaries built by `go install
// .../cmd/rdq@vX.Y.Z` this resolves to the module tag automatically. For
// local `go build` invocations Main.Version is "(devel)", which we map to
// a stable placeholder so skillsmith's semver parser stays happy.
func versionString() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		v := info.Main.Version
		if v != "" && v != "(devel)" {
			return v
		}
	}
	return "0.0.0-dev"
}
