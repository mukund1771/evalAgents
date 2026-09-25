// Command agenteval runs local agent evaluations.
package main

import (
	"os"

	"agenteval/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
