# agenteval

`agenteval` is a local agent-eval runner. It scores a trajectory — the steps, tool calls, and final output — and writes an immutable run to disk. It does not call a hosted eval service.

Everything is offline by default. No API key, no model, no network, no service to run.

## Install

A binary from the [latest release](https://github.com/mukund1771/evalAgents/releases/latest) — no Go toolchain needed. Swap `darwin_arm64` for `darwin_amd64`, `linux_amd64`, `linux_arm64`, or `windows_amd64`:

```bash
curl -fsSL https://github.com/mukund1771/evalAgents/releases/latest/download/agenteval_darwin_arm64.tar.gz | tar xz
./agenteval demo
```

Or with Go:

```bash
go install github.com/mukund1771/evalAgents/cmd/agenteval@latest
agenteval demo
```

Or from a clone:

```bash
go build -o agenteval ./cmd/agenteval
./agenteval demo
```

Each release also ships a `SHA256SUMS` file. `agenteval version` prints what you have.

## Quickstart

`agenteval demo` needs nothing on disk. The example suite and its recorded trajectories are embedded in the binary, so the command unpacks them to a temp directory, scores a support agent against ten recorded trajectories, scores a regressed version of the same agent, and diffs the two:

```bash
agenteval demo          # the whole story in one command
agenteval tui           # browse the two runs it just stored
agenteval init myeval   # write the example out so you can edit it
```

Then the same thing by hand, which is what a real suite looks like. The second run exits 1, and so does the diff:

```bash
agenteval run myeval/suite.yaml
agenteval run myeval/regression/suite.yaml
agenteval diff <good_run_id> <bad_run_id>
```

Run ids are the first column of `agenteval list`. Every subcommand takes `--root` if you want the store somewhere other than `./.agenteval`. `go build ./...` typechecks every package; the binary name comes from `-o agenteval`, because building several packages leaves no binary behind.

To execute an agent instead of replaying a recording:

```bash
agenteval run myeval/suite-cmd.yaml --record
agenteval run myeval/suite-cmd.yaml --offline
```

`--offline` reads cassettes and fails if one is missing. It does not fall through to a live call.

## Browsing a run

`agenteval tui` is a read-only browser over the run store. Runs list, then a run's case × scorer matrix, then one case's scores with each score's reason, the evidence span, and the trajectory step that caused it — with the bytes the scorer named marked in place. Mark two runs with `space` and press `d` for the regression report.

```
agenteval runs   .agenteval
   RUN                      SUITE          WHEN                 PASS^1  CASES
>  20260926T100001Z-bbbbbb  support-agent  2026-09-26 10:00:01  0.50    2
   20260926T100000Z-aaaaaa  support-agent  2026-09-26 10:00:00  1.00    2
```

It renders the same values the CLI prints, because the table is `report.Table`'s own output and the diff is `report.FormatDiff`'s — the two cannot disagree about a number. Evidence is marked with `[[ ]]` rather than colour alone, so it survives `NO_COLOR`, a dumb terminal, and a pipe.

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

Runs are immutable JSONL under `.agenteval/runs/<run_id>/` (`manifest.json`, `results.jsonl`, `summary.json`). No SQLite, so the build does not need cgo. Each result row carries `duration_ms` measured by the harness around the target call, including a failed one. Tokens and cost are whatever the target reported about itself, because only the target knows them; `agenteval show <run> --case <id>` prints both, and labels the second kind as reported. `--repeats N` reports pass^1 and pass^k using the tau-bench estimator `C(c,k)/C(n,k)`. `diff` and `--baseline` exit 1 when a case that passed now fails, or a scorer mean drops. New cases are reported and are not regressions.

`tool_call_f1` is also registered as `tool_call_accuracy`. The judge's metric is `task_completion` or `faithfulness`. Those are Lyzr's names for the same ideas: did the tool calls match, and did the task actually finish.

The eval path has one dependency outside the standard library, `gopkg.in/yaml.v3`, because the suite file is YAML and the standard library does not parse it. `agenteval tui` adds Bubble Tea, Bubbles and Lip Gloss, which is the honest cost of the browser and the reason it is a separate subcommand rather than something on the scoring path. Still no cgo, still one static binary, still nothing on a port.

## Non-goals

- A hosted service, accounts, multi-tenancy, or a web UI. `agenteval tui` is a read-only reader over the run store: it renders the files the CLI prints, recomputes nothing, and cannot launch a run or edit a suite. Nothing listens on a port and nothing phones home.
- A labelling or authoring interface. Suites are YAML files you edit in your own editor.
- SQLite or any cgo dependency.
- An OTLP receiver. Mapping `POST /v1/traces` spans onto trajectory steps is the obvious next step, and the `replay` target is the seam it would plug into.
- Embeddings or semantic similarity.
- A provider zoo. One OpenAI-compatible client honors `OPENAI_BASE_URL` (OpenAI, Ollama, llama.cpp, vLLM).
- More than one LLM judge. `model_graded` is off unless a suite lists it.
- Generating test cases. This tool scores cases. It does not invent them.
- A scheduler. `--concurrency` is a fixed worker pool. The default is 1.
- Retries around the target. The harness cannot know whether calling your agent twice is safe, so a target that fails is a failed case with its error recorded, not a silent second attempt. The judge is the one exception and retries once, because reasking for JSON has no side effects.

## Background

[docs/agent-eval-concepts.md](docs/agent-eval-concepts.md) is a map of the field: what a trajectory is, why a null score is not a zero, the difference between pass@k and pass^k, and which of these metric families the platforms below each chose. It explains the design decisions above rather than restating them.

## Prior art

- Braintrust: hosted experiments and a nil score for "not applicable". This tool keeps that score contract and stores the run locally.
- Raindrop: hosted agent monitoring. This tool scores local traces and does not phone home.
- [mira](https://github.com/everruns/mira): Study, Subject, and Scorer, with JUnit output. This tool is one binary and a YAML suite, not a protocol host.
- ADK eval: tests written inside an agent framework. This tool scores any process that speaks JSON on stdio.
- Inspect AI: a Python framework of models, solvers, and datasets. Deterministic trajectory checks run here with no model.
- promptfoo: prompt and assertion matrices. This tool grades tool trajectories.
- DeepEval: Python metrics, many of them LLM judges. The default suite here needs no model and no API key.

## CI

`.github/workflows/ci.yml` checks formatting, vets, runs the tests under `-race`, builds the binary, runs the support suite, runs `agenteval demo` so the path a downloaded binary takes is covered too, and then asserts the gate: the recorded regression suite must exit non-zero and `diff` must name the case that broke. The feature the tool exists for is tested in CI rather than described in a README.

`.github/workflows/release.yml` builds static binaries for darwin arm64/amd64, linux amd64/arm64 and windows amd64 on a `v*` tag, and uploads them with a `SHA256SUMS` file.

To gate another repo, write JUnit and fail the job on a non-zero exit:

```yaml
- run: go build -o agenteval ./cmd/agenteval
- run: ./agenteval run examples/support/suite.yaml --junit results.xml --baseline ${{ vars.EVAL_BASELINE }}
```

`--matrix model=a,b` runs a `cmd` target once per value, with that value in the child environment, and writes one run each.

## Judge

Leave `model_graded` out of the suite unless you want it. When it is listed, set `OPENAI_API_KEY`. Optional: `OPENAI_BASE_URL`, `OPENAI_MODEL`.

The judge must return `{"value":0-1,"passed":true,"explanation":"...","evidence":[{"step_index":0,"start":0,"end":0}]}`. `passed` is checked for presence, then recomputed from `value >= min`. Invalid JSON is retried once, then skipped.

## Commands

```
agenteval run <suite.yaml> [--filter substr] [--repeats N] [--baseline id]
                           [--offline] [--record] [--json] [--junit path]
                           [--concurrency N] [--matrix key=a,b] [--root dir]
agenteval diff <run_a> <run_b> [--root dir]
agenteval list [--root dir]
agenteval show <run_id> [--case id] [--root dir]
agenteval tui [--root dir]             browse stored runs, read-only
agenteval demo [--root dir]            run the embedded example, then diff it
agenteval init [dir]                   write the example out so you can edit it
agenteval version
```

`run` and `diff` exit 1 when a case fails or a baseline regresses, which is what a CI job gates on. Everything else exits 0, or 2 on a bad argument — `tui` never exits 1, so it cannot be mistaken for a gate.

## License

MIT. See [LICENSE](LICENSE).
