// Command agenteval runs local agent evaluations.
package main

import (
	"os"

	"github.com/mukund1771/evalAgents/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
