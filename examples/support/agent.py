#!/usr/bin/env python3
"""A deterministic support agent for the cmd-target example.

It reads one JSON case on stdin and writes one JSON trajectory on stdout, which
is the entire contract between agenteval and the thing being evaluated. Python
rather than Go on purpose: the point of the cmd target is that your agent does
not have to be written in the harness's language, and an example that only works
inside this repository's module would prove the opposite.

Needs nothing but a python3 on PATH. No dependencies, no network, no API key.
"""

import json
import sys

# case id -> (final answer, [(tool name, args, tool result), ...])
CASES = {
    "refund-happy": (
        "I've issued a refund for order 1001.",
        [
            ("lookup_order", {"order_id": "1001"}, {"status": "delivered", "damaged": True}),
            ("issue_refund", {"order_id": "1001", "reason": "damaged"}, {"refund_id": "r-1001"}),
        ],
    ),
    "order-status": (
        "Order 1002 has shipped. Tracking number 1Z999.",
        [("lookup_order", {"order_id": "1002"}, {"status": "shipped", "tracking": "1Z999"})],
    ),
    "refund-denied": (
        "Order 1003 is outside the refund window, so I can't refund it.",
        [
            ("lookup_order", {"order_id": "1003"}, {"status": "delivered", "age_days": 90}),
            ("check_policy", {"order_id": "1003"},
             {"eligible": False, "reason": "outside the refund window"}),
        ],
    ),
    "ask-order-id": ("I can help with a refund. What is the order id?", []),
    "tracking-json": (
        '{"order_id":"1002","tracking":"1Z999"}',
        [("lookup_order", {"order_id": "1002"}, {"status": "shipped", "tracking": "1Z999"})],
    ),
    "exact-greeting": ("Hello, how can I help with your order today?", []),
    "replacement": (
        "I've created a replacement for order 1004.",
        [
            ("lookup_order", {"order_id": "1004"}, {"status": "delivered", "damaged": True}),
            ("create_replacement", {"order_id": "1004"}, {"replacement_id": "rp-1004"}),
        ],
    ),
    "cancel-order": (
        "Order 1005 is canceled.",
        [
            ("lookup_order", {"order_id": "1005"}, {"status": "processing"}),
            ("cancel_order", {"order_id": "1005"}, {"status": "canceled"}),
        ],
    ),
    "address-update": (
        "I updated the address on order 1006.",
        [
            ("lookup_order", {"order_id": "1006"}, {"status": "processing"}),
            ("update_address", {"address": "10 Main St", "order_id": "1006"}, {"updated": True}),
        ],
    ),
    "refund-policy": (
        "Our refund policy covers damaged items within 30 days of delivery.",
        [],
    ),
}


def compact(obj):
    """Sorted keys and no spaces, so a recorded cassette is byte-stable."""
    return json.dumps(obj, sort_keys=True, separators=(",", ":"))


def trajectory(case_id, user, final, calls):
    steps = [{"kind": "user", "content": user}]
    for name, args, result in calls:
        steps.append({"kind": "assistant",
                      "tool_calls": [{"name": name, "args": args}]})
        steps.append({"kind": "tool_result", "call_id": name,
                      "content": compact(result)})
    steps.append({"kind": "final", "content": final})
    return {
        "case_id": case_id,
        "steps": steps,
        "final_output": final,
        # Only the agent can know its own token count; the harness measures
        # wall-clock itself and stores it as duration_ms on the result row.
        "metrics": {"tokens": 80},
    }


def main():
    case = json.load(sys.stdin)
    case_id = case.get("id", "")
    if case_id not in CASES:
        print("unknown case %s" % case_id, file=sys.stderr)
        return 1
    final, calls = CASES[case_id]
    user = (case.get("input") or {}).get("user", "")
    json.dump(trajectory(case_id, user, final, calls), sys.stdout)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
