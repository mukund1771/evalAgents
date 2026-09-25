# agenteval

`agenteval` is a local agent-eval runner. It scores a trajectory — the steps, tool calls, and final output — and writes an immutable run to disk. It does not call a hosted eval service.

The default demo is offline. No API key, no model, no network.

## Quickstart

```bash
go build -o agenteval ./cmd/agenteval
./agenteval run examples/support/suite.yaml
```

`go build ./...` typechecks every package. The binary name comes from `-o agenteval`, because building several packages does not leave a binary in the working directory.

That command replays recorded trajectories for a small support agent, prints a pass^k table, and exits 0. A recorded regression is the next two commands. The second exits 1.

```bash
./agenteval run examples/support/suite.yaml
./agenteval run examples/support/regression/suite.yaml
./agenteval diff <good_run_id> <bad_run_id>
```

Run ids are the first column of `./agenteval list`.

To execute the example agent instead of the cassette:

```bash
./agenteval run examples/support/suite-cmd.yaml --record
./agenteval run examples/support/suite-cmd.yaml --offline
```

`--offline` reads cassettes and fails if one is missing. It does not fall through to a live call.

## Design

A case goes in. A trajectory comes out. Scorers read both. The trajectory is the difference between an agent eval and an LLM eval: tool order, tool errors, and step count are part of the result, not metadata around a final string.

```go
type Score struct {
    Name        string
    Value       *float64 // nil means skipped, not zero
    Passed      *bool
    Explanation string
    Evidence    []Span // the step that caused the score
}
```

A skipped scorer is nil, never `0`. `exact_match` skips when `expected.exact` is absent. Tool-match scorers skip when `expected.tools` is absent. `tools_succeeded`, `steps_within`, and `no_loop` always score. The judge skips when it is disabled or `OPENAI_API_KEY` is unset, and it skips again if its JSON is still invalid after one retry. A hallucinated score is inert.

`Evidence` is a span: a step index plus byte offsets into that step. The number is computed by the scorer. The span says where to look.

Two targets:

- `cmd` runs any binary. One JSON case on stdin, one JSON trajectory on stdout. A Python agent does not have to be ported.
- `replay` scores a trajectory file recorded earlier. This is how traces an existing service already emits get scored.

Cassettes are those trajectory files (`{case_id}.json`), not an HTTP recording. A missing cassette is an error.

Runs are immutable JSONL under `.agenteval/runs/<run_id>/` (`manifest.json`, `results.jsonl`, `summary.json`). No SQLite, so the build does not need cgo. `--repeats N` reports pass^1 and pass^k using the tau-bench estimator `C(c,k)/C(n,k)`. `diff` and `--baseline` exit 1 when a case that passed now fails, or a scorer mean drops. New cases are reported and are not regressions.

`tool_call_f1` is also registered as `tool_call_accuracy`. The judge's metric is `task_completion` or `faithfulness`. Those are Lyzr's names for the same ideas: did the tool calls match, and did the task actually finish.

The only dependency outside the standard library is `gopkg.in/yaml.v3`, because the suite file is YAML and the standard library does not parse it.

## Non-goals

- A web UI or TUI.
- SQLite or any cgo dependency.
- A real OTLP receiver. `internal/otlp.Handler` returns 501 and is not served. A later version would accept `POST /v1/traces` and map spans onto trajectory steps.
- Embeddings or semantic similarity.
- A provider zoo. One OpenAI-compatible client honors `OPENAI_BASE_URL` (OpenAI, Ollama, llama.cpp, vLLM).
- More than one LLM judge. `model_graded` is off unless a suite lists it.
- Generating test cases. This tool scores cases. It does not invent them.
- A scheduler. `--concurrency` is a fixed worker pool. The default is 1.

## Prior art

- Braintrust: hosted experiments and a nil score for "not applicable". This tool keeps that score contract and stores the run locally.
- Raindrop: hosted agent monitoring. This tool scores local traces and does not phone home.
- [mira](https://github.com/everruns/mira): Study, Subject, and Scorer, with JUnit output. This tool is one binary and a YAML suite, not a protocol host.
- ADK eval: tests written inside an agent framework. This tool scores any process that speaks JSON on stdio.
- Inspect AI: a Python framework of models, solvers, and datasets. Deterministic trajectory checks run here with no model.
- promptfoo: prompt and assertion matrices. This tool grades tool trajectories.
- DeepEval: Python metrics, many of them LLM judges. The default suite here needs no model and no API key.

## CI

`.github/workflows/ci.yml` builds the binary and runs the support suite. To gate another repo, write JUnit and fail the job on a non-zero exit:

```yaml
- run: go build -o agenteval ./cmd/agenteval
- run: ./agenteval run examples/support/suite.yaml --junit results.xml --baseline ${{ vars.EVAL_BASELINE }}
```

`--matrix model=a,b` runs a `cmd` target once per value, with that value in the child environment, and writes one run each.

## Judge

Leave `model_graded` out of the suite unless you want it. When it is listed, set `OPENAI_API_KEY`. Optional: `OPENAI_BASE_URL`, `OPENAI_MODEL`.

The judge must return `{"value":0-1,"passed":true,"explanation":"...","evidence":[{"step_index":0,"start":0,"end":0}]}`. `passed` is checked for presence, then recomputed from `value >= min`. Invalid JSON is retried once, then skipped.
