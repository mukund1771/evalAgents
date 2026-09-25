// Package otlp documents a future trace ingest. The handler is not served.
package otlp

import "net/http"

// Handler is the stub for POST /v1/traces. It always returns 501.
// A future implementation would map OTLP spans onto eval.Step values
// (kind, tool call, tool result) and hand the trajectory to the replay scorer.
// This process does not listen.
func Handler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "OTLP ingest is not implemented", http.StatusNotImplemented)
}
