// Command agenteval runs local agent evaluations.
package main

import (
	"os"

	"github.com/mukund1771/evalAgents/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
// A binary that cannot say what it is turns every bug report into a guess.
var version = "dev"

func main() {
	cli.Version = version
	os.Exit(cli.Run(os.Args[1:]))
}
