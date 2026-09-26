// Package evalagents exists for one reason: //go:embed cannot reach outside the
// directory of the file that declares it, and the example suite lives at the
// repository root. Embedding it here rather than copying it under internal/
// keeps one copy of the examples, so the demo inside the binary and the files a
// reader browses on GitHub cannot drift apart.
//
// Everything else in this module lives under cmd/ and internal/.
package evalagents

import "embed"

// Examples is the support-agent example: the suite, its recorded trajectories,
// and the regression variant of the same suite. This is what makes a downloaded
// binary useful on its own, with no clone and no files on disk.
//
//go:embed examples
var Examples embed.FS
