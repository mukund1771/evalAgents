// Command agent is a deterministic support agent for the offline demo.
// It reads one JSON case on stdin and writes one JSON trajectory on stdout.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"agenteval/internal/eval"
)

func main() {
	var c eval.Case
	if err := json.NewDecoder(os.Stdin).Decode(&c); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	tr, ok := respond(c)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown case %s\n", c.ID)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(tr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type spec struct {
	final string
	calls []call
}

type call struct {
	name   string
	args   string
	result string
}

func respond(c eval.Case) (eval.Trajectory, bool) {
	var in struct {
		User string `json:"user"`
	}
	_ = json.Unmarshal(c.Input, &in)
	known := map[string]spec{
		"refund-happy": {
			final: "I've issued a refund for order 1001.",
			calls: []call{
				{"lookup_order", `{"order_id":"1001"}`, `{"status":"delivered","damaged":true}`},
				{"issue_refund", `{"order_id":"1001","reason":"damaged"}`, `{"refund_id":"r-1001"}`},
			},
		},
		"order-status": {
			final: "Order 1002 has shipped. Tracking number 1Z999.",
			calls: []call{
				{"lookup_order", `{"order_id":"1002"}`, `{"status":"shipped","tracking":"1Z999"}`},
			},
		},
		"refund-denied": {
			final: "Order 1003 is outside the refund window, so I can't refund it.",
			calls: []call{
				{"lookup_order", `{"order_id":"1003"}`, `{"status":"delivered","age_days":90}`},
				{"check_policy", `{"order_id":"1003"}`, `{"eligible":false,"reason":"outside the refund window"}`},
			},
		},
		"ask-order-id": {
			final: "I can help with a refund. What is the order id?",
		},
		"tracking-json": {
			final: `{"order_id":"1002","tracking":"1Z999"}`,
			calls: []call{
				{"lookup_order", `{"order_id":"1002"}`, `{"status":"shipped","tracking":"1Z999"}`},
			},
		},
		"exact-greeting": {
			final: "Hello, how can I help with your order today?",
		},
		"replacement": {
			final: "I've created a replacement for order 1004.",
			calls: []call{
				{"lookup_order", `{"order_id":"1004"}`, `{"status":"delivered","damaged":true}`},
				{"create_replacement", `{"order_id":"1004"}`, `{"replacement_id":"rp-1004"}`},
			},
		},
		"cancel-order": {
			final: "Order 1005 is canceled.",
			calls: []call{
				{"lookup_order", `{"order_id":"1005"}`, `{"status":"processing"}`},
				{"cancel_order", `{"order_id":"1005"}`, `{"status":"canceled"}`},
			},
		},
		"address-update": {
			final: "I updated the address on order 1006.",
			calls: []call{
				{"lookup_order", `{"order_id":"1006"}`, `{"status":"processing"}`},
				{"update_address", `{"address":"10 Main St","order_id":"1006"}`, `{"updated":true}`},
			},
		},
		"refund-policy": {
			final: "Our refund policy covers damaged items within 30 days of delivery.",
		},
	}
	s, ok := known[c.ID]
	if !ok {
		return eval.Trajectory{}, false
	}
	return build(c.ID, in.User, s), true
}

func build(id, user string, s spec) eval.Trajectory {
	steps := []eval.Step{{Kind: "user", Content: user}}
	for _, c := range s.calls {
		steps = append(steps,
			eval.Step{Kind: "assistant", ToolCalls: []eval.ToolCall{{Name: c.name, Args: json.RawMessage(c.args)}}},
			eval.Step{Kind: "tool_result", CallID: c.name, Content: c.result},
		)
	}
	steps = append(steps, eval.Step{Kind: "final", Content: s.final})
	return eval.Trajectory{
		CaseID:      id,
		Steps:       steps,
		FinalOutput: s.final,
		Metrics:     eval.Metrics{Tokens: 80, LatencyMS: 12},
	}
}
