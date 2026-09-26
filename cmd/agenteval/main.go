// Command agenteval runs local agent evaluations.
package main

import (
	"os"
	"runtime/debug"

	"github.com/mukund1771/evalAgents/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3", which is
// how the release archives are stamped. A binary that cannot say what it is
// turns every bug report into a guess.
var version = "dev"

// buildVersion resolves the version for all three install routes. A release
// archive carries the ldflag. `go install module@v1.2.3` cannot, but the
// toolchain records the version it resolved in the build info, so read it from
// there rather than reporting "dev" for a perfectly well-identified build. A
// build from a git working tree lands there too and comes out as the nearest
// tag, suffixed "+dirty" when the tree has uncommitted changes. "dev" is left
// only for a build with no ldflag and no version control to ask.
func buildVersion() string {
	if version != "dev" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok || bi.Main.Version == "" || bi.Main.Version == "(devel)" {
		return version
	}
	return bi.Main.Version
}

func main() {
	cli.Version = buildVersion()
	os.Exit(cli.Run(os.Args[1:]))
}
