# agenteval

[![CI](https://github.com/mukund1771/evalAgents/actions/workflows/ci.yml/badge.svg)](https://github.com/mukund1771/evalAgents/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/mukund1771/evalAgents?sort=semver)](https://github.com/mukund1771/evalAgents/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/mukund1771/evalAgents)](go.mod)
[![License](https://img.shields.io/github/license/mukund1771/evalAgents)](LICENSE)

**A local agent-eval runner.** It scores a *trajectory* — the steps an agent took, the tools it called, the arguments it passed, and the answer it landed on — then stores the run so the next run can be compared against it. `agenteval diff` exits non-zero when something that used to pass now fails, which is the whole reason a tool like this exists.

One static binary. No service, no accounts, no API key, and the default example runs with no network at all.

<!-- ─────────────────────────────────────────────────────────────────────────
     SCREEN RECORDING GOES HERE

     Two ways to do it:

     1. GIF committed to the repo (works everywhere, including on npm-style
        mirrors and in offline clones):
            put the file at docs/assets/demo.gif
            then replace this comment block with:
            ![agenteval demo](docs/assets/demo.gif)

     2. MP4 hosted by GitHub (better quality, plays with sound, but the URL
        only exists on github.com):
            drag the .mp4 into any issue or PR comment on this repo,
            copy the https://github.com/user-attachments/... URL it produces,
            then replace this comment block with:
            https://github.com/user-attachments/assets/<id>

     See docs/assets/README.md for what to capture.
     ───────────────────────────────────────────────────────────────────────── -->

## Thirty seconds

```bash
agenteval demo
```

That needs nothing on disk. The example suite and its recorded trajectories are embedded in the binary, so `demo` unpacks them to a temp directory, scores a support agent against ten recorded trajectories, scores a version of the same agent after a regression, and diffs the two — the full loop, in one command.

Then browse what it stored:

```bash
agenteval tui
```

## Install

Grab a binary — no Go toolchain needed. Swap `darwin_arm64` for `darwin_amd64`, `linux_amd64`, `linux_arm64`, or `windows_amd64`:

```bash
curl -fsSL https://github.com/mukund1771/evalAgents/releases/latest/download/agenteval_darwin_arm64.tar.gz | tar xz
./agenteval demo
```

Or with Go:

```bash
go install github.com/mukund1771/evalAgents/cmd/agenteval@latest
```

Or from a clone:

```bash
go build -o agenteval ./cmd/agenteval
```

Every release ships a `SHA256SUMS` file. `agenteval version` says what you have.

## Why trajectories, not answers

An agent that says *"I've issued your refund"* without ever calling `issue_refund` has failed, even though the sentence is perfect. An agent that calls `lookup_order` three times in a row is looping, even though it eventually answers. A scorer that only sees the final string cannot tell you either of those things.

So the unit of work here is the trajectory: every step, every tool call and its arguments, every tool result including whether it errored, and the final answer. Scorers read all of it.

Here is that difference on one line of real output. `refund-denied` produced a perfectly good answer, and its tool call failed:

```
CASE           PASS^1  PASS^K  contains  tool_trajectory_exact  tools_succeeded  steps_within
refund-denied  0.00    0.00    1.00      1.00                   0.00             1.00
```

`contains` is happy. `tool_trajectory_exact` is happy. The case still fails, because `check_policy` timed out and the agent answered from its own assumptions.

## A suite

A suite is one YAML file. This is the whole thing:

```yaml
name: refunds
target:
  kind: replay          # score recorded trajectories
  dir: cassettes        # one {case_id}.json per case, relative to this file
scorers:
  - kind: contains
  - kind: tool_trajectory_exact
  - kind: tools_succeeded
  - kind: steps_within
    max: 6
cases:
  - id: refund-happy
    tags: [refund]
    input: { user: "Refund order 1001, it arrived damaged." }
    expected:
      contains: refund
      tools:
        - { name: lookup_order, args: { order_id: "1001" } }
        - { name: issue_refund, args: { order_id: "1001", reason: damaged } }

  - id: refund-denied
    tags: [refund, policy]
    input: { user: "Refund order 1003." }
    expected:
      contains: outside the refund window
      tools:
        - { name: lookup_order, args: { order_id: "1003" } }
        - { name: check_policy, args: { order_id: "1003" } }

  - id: ask-order-id
    tags: [refund]
    input: { user: "I want a refund." }
    expected:
      contains: order id     # no tools expected: this case should just ask
```

`input` is handed to the target as JSON, so it can hold whatever your agent needs. `expected` is read by the scorers, and **a scorer that does not find its field skips** rather than failing — `ask-order-id` expects no tools, so the tool scorers sit that case out.

Run it:

```console
$ agenteval run suite.yaml
run 20260926T094647Z-f8c7b2  suite refunds  pass^1 1.00  pass^k 1.00
CASE           PASS^1  PASS^K  contains  tool_trajectory_exact  tools_succeeded  steps_within
refund-happy   1.00    1.00    1.00      1.00                   1.00             1.00
refund-denied  1.00    1.00    1.00      1.00                   1.00             1.00
ask-order-id   1.00    1.00    1.00      -                      1.00             1.00
```

That `-` is the most important character in the table. It means *skipped*, not zero — see [the score contract](#the-score-contract).

## Catching a regression

Now the agent changes: it forgets to call `issue_refund` and loops on `lookup_order`, and the policy service starts timing out.

```console
$ agenteval run regression/suite.yaml
run 20260926T094647Z-30ca06  suite refunds-after-change  pass^1 0.33  pass^k 0.33
CASE           PASS^1  PASS^K  contains  tool_trajectory_exact  tools_succeeded  steps_within
refund-happy   0.00    0.00    0.00      0.00                   1.00             1.00
refund-denied  0.00    0.00    1.00      1.00                   0.00             1.00
ask-order-id   1.00    1.00    1.00      -                      1.00             1.00

$ echo $?
1
```

A number going down is not the same as a regression, so `diff` says which:

```console
$ agenteval diff 20260926T094647Z-f8c7b2 20260926T094647Z-30ca06
baseline 20260926T094647Z-f8c7b2
current  20260926T094647Z-30ca06
5 regression(s)
regression: case refund-happy passed and now fails (contains, tool_trajectory_exact)
regression: case refund-denied passed and now fails (tools_succeeded)
regression: contains mean 1.0000 -> 0.6667
regression: tool_trajectory_exact mean 1.0000 -> 0.5000
regression: tools_succeeded mean 1.0000 -> 0.6667

$ echo $?
1
```

A regression is a case that passed and now fails, or a scorer mean that dropped on the cases both runs share. **Cases that are new, or that were removed, are reported as notes and never as regressions** — otherwise adding an easy case would look like an improvement and deleting a hard one would look like a fix.

`--baseline <run_id>` folds the comparison into the run, which is the one-liner a CI job wants:

```yaml
- run: agenteval run suite.yaml --junit results.xml --baseline ${{ vars.EVAL_BASELINE }}
```

## Browsing runs

`agenteval tui` is a **read-only** browser over the run store. It renders the same files the CLI prints and recomputes nothing — the table below *is* the CLI's table, and the diff *is* `agenteval diff`'s output.

<!-- Screenshots of these four views can go in docs/assets/ and be linked here.
     The plain-text frames below are real output and stay useful either way. -->

Runs, newest first:

```
agenteval runs   .agenteval
    RUN                      SUITE                 WHEN                 PASS^1  CASES
>   20260926T094647Z-30ca06  refunds-after-change  2026-09-26 09:46:47  0.33    3
    20260926T094647Z-f8c7b2  refunds               2026-09-26 09:46:47  1.00    3
?  keys
```

`enter` opens one run's case × scorer matrix:

```
agenteval run   .agenteval
  run 20260926T094647Z-30ca06  suite refunds-after-change  pass^1 0.33  pass^k 0.33
  CASE           PASS^1  PASS^K  contains  tool_trajectory_exact  tools_succeeded  steps_within
> refund-happy   0.00    0.00    0.00      0.00                   1.00             1.00
  refund-denied  0.00    0.00    1.00      1.00                   0.00             1.00
  ask-order-id   1.00    1.00    1.00      -                      1.00             1.00
  enter a case for its scores, evidence, and trajectory
?  keys
```

`enter` again opens the case, with every scorer's reason and the span it points at:

```
agenteval case   .agenteval
case refund-denied   0.1ms
target reported: 74 tokens
FINAL OUTPUT
Order 1003 is outside the refund window of 30 days.
SCORES
  contains                     1.00
      final output contains the expected text
      evidence step 5 [14,39)
  tool_trajectory_exact        1.00
      tool sequence matches
  tools_succeeded              0.00
      tool error: step 4 (check_policy)
?  keys
```

Scroll down and the trajectory shows those spans marked in place. Two different scorers, two different steps, exactly the bytes each one named:

```
agenteval case   .agenteval
TRAJECTORY
  [0] user
      Refund order 1003.
  [1] assistant
      lookup_order {"order_id":"1003"}
  [2] tool_result call=lookup_order
      {"status":"delivered","age_days":90}
  [3] assistant
      check_policy {"order_id":"1003"}
  [4] tool_result call=check_policy error
      [[{"error":"policy service timeout"}]]
  [5] final
      Order 1003 is [[outside the refund window]] of 30 days.
?  keys
```

Evidence is marked with `[[ ]]`, not colour alone — colour disappears under `NO_COLOR`, on a dumb terminal, and through a pipe, and the mark has to survive all three.

Mark two runs with `space` and press `d` for the regression report. `A` is always the baseline and `B` the candidate, decided by timestamp rather than by which you marked first, because `Compare(newer, older)` would report a real regression as "no regressions".

| Key | |
|---|---|
| `↑` `↓` / `j` `k` | move, or scroll |
| `enter` | open · `esc` back |
| `space` | mark a run · `d` diff the marked pair · `c` clear |
| `n` `p` | step through repeats of a case |
| `g` `G` | first / last · `pgup` `pgdn`, `ctrl+u` `ctrl+d` scroll |
| `?` | keys · `q` quit |

## Scorers

Every scorer takes `as` to rename it in the output and `min` to set its pass threshold (default `1`).

| `kind` | Reads | Scores |
|---|---|---|
| `exact_match` | `expected.exact` | 1 if the final output equals it, trimmed |
| `contains` | `expected.contains` | 1 if the final output contains it |
| `json_valid` | `expected.json: true` | 1 if the final output parses as JSON |
| `tool_trajectory_exact` | `expected.tools` | 1 only if the calls match in order, name and arguments |
| `tool_trajectory_subsequence` | `expected.tools` | longest common subsequence ÷ expected length, so extra calls cost little and missing ones cost a lot |
| `tool_call_f1` | `expected.tools` | order-free F1 over `(name, arguments)` as a multiset; reports precision and recall. Also registered as `tool_call_accuracy` |
| `tools_succeeded` | nothing | 1 if no tool step came back as an error. Names the call that failed |
| `steps_within` | `max:` on the scorer, or `expected.max_steps` per case | 1 if the trajectory fits the budget |
| `no_loop` | `max_repeats:` (default 2) | 0 if the same call with the same arguments repeats past the limit |
| `model_graded` | `metric:` — `task_completion` or `faithfulness` | an LLM judge. Off unless the suite lists it. See [Judge](#judge) |

Arguments are compared canonically, so `{"b":1,"a":2}` and `{"a":2,"b":1}` are the same call — JSON key order is not part of what an agent did.

The four tool scorers are what make this an agent eval rather than an LLM eval. `tool_trajectory_exact` is right when the procedure *is* the task ("look up the order, then issue the refund"). `tool_call_f1` is right when several independent lookups may happen in any order.

## The score contract

```go
type Score struct {
    Name        string
    Value       *float64 // nil means skipped, not zero
    Passed      *bool
    Explanation string
    Evidence    []Span   // where in the trajectory this came from
}
```

`nil` and `0` mean different things and the difference matters. `0` means *this was graded and it failed*. `nil` means *this scorer had nothing to say about this case*, and it is left out of every average. If `json_valid` scored 0 on a case that was never supposed to return JSON, it would drag down a case that did nothing wrong.

So: `exact_match` skips without `expected.exact`. The tool scorers skip without `expected.tools`. `tools_succeeded`, `steps_within` and `no_loop` always apply. The judge skips when it is disabled, when `OPENAI_API_KEY` is unset, and again if its JSON is still invalid after one retry — a hallucinated score is inert rather than wrong.

`Evidence` is a step index plus byte offsets into that step's content. The number is the scorer's claim; the span says where to look so you can check it. Spans are validated against the step they name, and one that does not fit is reported as out of range rather than quietly trimmed to fit.

## Targets

Two, behind one interface.

**`replay`** scores a trajectory recorded earlier. This is how you test the scorers themselves, and how you grade traces a service already emits.

```yaml
target: { kind: replay, dir: cassettes }
```

**`cmd`** runs any executable. One JSON case in on stdin, one JSON trajectory out on stdout. A Python agent does not have to be ported.

```yaml
target:
  kind: cmd
  command: [python3, agent.py]
  timeout: 30s        # default 30s
  cassettes: cassettes  # where --record writes and --offline reads
```

<details>
<summary><b>The trajectory JSON your command must print</b></summary>

```json
{
  "case_id": "refund-happy",
  "steps": [
    { "kind": "user",      "content": "Refund order 1001, it arrived damaged." },
    { "kind": "assistant", "tool_calls": [
        { "name": "lookup_order", "args": { "order_id": "1001" } } ] },
    { "kind": "tool_result", "call_id": "lookup_order",
      "content": "{\"status\":\"delivered\"}", "is_error": false },
    { "kind": "final", "content": "I've issued a refund for order 1001." }
  ],
  "final_output": "I've issued a refund for order 1001.",
  "metrics": { "tokens": 80, "cost": 0.0 }
}
```

`kind` is one of `user`, `assistant`, `tool_result`, `final`. `is_error` on a tool result is what `tools_succeeded` reads. `metrics` is what your agent knows about itself — only it can count its own tokens; wall-clock latency is measured by the harness and stored as `duration_ms` on the result row.

`--record` saves each trajectory as `{case_id}.json` so the next run can replay it. `--offline` replays and **fails if a cassette is missing**: it never falls through to a live call, because an "offline" demo that quietly starts needing an API key is worse than one that breaks loudly.

</details>

## Reliability

One green run is an anecdote. Agents sample, tools flake, and the same case can pass and then fail.

`--repeats N` runs every case N times and reports two numbers. **pass^1** is the plain success rate. **pass^k** is the τ-bench estimator `C(c,k)/C(n,k)` — the probability that *all* k independent attempts succeed. An agent that completes a refund four times in five has a good pass^1 and a poor pass^5, and for anything customer-facing the second number is the one that matters.

Replaying a cassette is deterministic, so pass^k equals pass^1 there. Repeats earn their keep when the target is a live model.

## Storage

Runs are immutable, one directory each, under `.agenteval/runs/<run_id>/`:

| File | |
|---|---|
| `manifest.json` | suite, target, scorers, repeats, filter, matrix, command, start time |
| `results.jsonl` | one line per (case, repeat): the trajectory, every score, `duration_ms`, any harness error |
| `summary.json` | the comparable artifact — per-case pass^1/pass^k and per-scorer means |

Plain files, so `jq` works and nothing has to be running. No SQLite, which means no cgo, which is why a single static binary cross-compiles to five platforms. `agenteval` refuses to write a run id twice, so a baseline cannot be overwritten in place.

## CI

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) checks formatting, vets, runs the tests under `-race`, builds, runs the example suite, runs `agenteval demo` so the path a downloaded binary takes is covered too — and then asserts the gate itself: the recorded regression suite must exit non-zero and `diff` must name the case that broke. The feature the tool exists for is tested in CI rather than described here.

[`.github/workflows/release.yml`](.github/workflows/release.yml) builds static binaries for five platforms on a `v*` tag and uploads them with a `SHA256SUMS` file.

## Judge

Leave `model_graded` out of the suite unless you want it. When it is listed, set `OPENAI_API_KEY`; `OPENAI_BASE_URL` and `OPENAI_MODEL` are optional, and the base URL is what points it at Ollama, llama.cpp or vLLM instead of OpenAI.

The judge must return exactly:

```json
{"value": 0.0, "passed": true, "explanation": "...",
 "evidence": [{"step_index": 0, "start": 0, "end": 0}]}
```

`passed` is checked for presence and then **recomputed** from `value >= min`, because a judge that grades itself is not a gate. `value` must be in range, and every `step_index` must name a step that exists — a judge citing step 14 of a 6-step trajectory has not scored anything. Invalid JSON is retried once, then the score is skipped rather than guessed.

Judges prefer longer, more confident answers, and they do not follow numeric rubrics reliably without a schema. That is why the deterministic scorers are the default gate and this one is opt-in.

## Non-goals

Deliberate omissions, with reasons. This list is as much the design as the code is.

- **A hosted service, accounts, multi-tenancy, or a web UI.** `agenteval tui` is a read-only reader over the run store: it renders the files the CLI prints, recomputes nothing, and cannot launch a run or edit a suite. Nothing listens on a port and nothing phones home.
- **A labelling or authoring interface.** Suites are YAML you edit in your own editor.
- **Generating test cases.** This tool scores cases; it does not invent them.
- **SQLite, or any cgo dependency.** Plain files, one static binary.
- **An OTLP receiver.** Mapping `POST /v1/traces` spans onto trajectory steps is the obvious next step, and the `replay` target is the seam it would plug into.
- **Embeddings or semantic similarity.** It rewards paraphrase and knows nothing about whether a tool was called.
- **A provider zoo.** One OpenAI-compatible client, pointed wherever you like by `OPENAI_BASE_URL`.
- **More than one LLM judge.**
- **A scheduler.** `--concurrency` is a fixed worker pool, default 1.
- **Retries around the target.** The harness cannot know whether calling your agent twice is safe, so a target that fails is a failed case with its error recorded, not a silent second attempt. The judge is the one exception and retries once, because reasking for JSON has no side effects.

## Commands

```
agenteval run <suite.yaml> [--filter substr] [--repeats N] [--baseline id]
                           [--offline] [--record] [--json] [--junit path]
                           [--concurrency N] [--matrix key=a,b] [--root dir]
agenteval diff <run_a> <run_b> [--root dir]
agenteval list [--root dir]
agenteval show <run_id> [--case id] [--root dir]
agenteval tui [--root dir]              browse stored runs, read-only
agenteval demo [--root dir]             run the embedded example, then diff it
agenteval init [dir]                    write the example out so you can edit it
agenteval version
```

`run` and `diff` exit **1** when a case fails or a baseline regresses — that is what a CI job gates on. Everything else exits 0, or 2 on a bad argument. `tui` never exits 1, so an interactive session can never be mistaken for a gate.

`--filter` matches a case id or a tag. `--matrix model=a,b` runs a `cmd` target once per value with that value in the child environment, and writes one run each.

## Development

```bash
go build ./...                 # typecheck every package
go test ./... -race            # 52 tests
gofmt -l . && go vet ./...     # what CI checks
agenteval init /tmp/x && agenteval run /tmp/x/suite.yaml
```

The eval path has one dependency outside the standard library, `gopkg.in/yaml.v3`, because the suite file is YAML. `agenteval tui` adds Bubble Tea, Bubbles and Lip Gloss — the honest cost of the browser, and the reason it is a separate subcommand rather than something on the scoring path. Still no cgo, still one static binary.

| Package | |
|---|---|
| `internal/eval` | the contracts: `Case`, `Trajectory`, `Step`, `Score`, `Span` |
| `internal/suite` | loads and validates the YAML |
| `internal/target` | `replay` and `cmd`, plus cassette read/write |
| `internal/scorer` | every scorer, and the judge client's caller |
| `internal/runner` | the worker pool, one run per matrix value |
| `internal/store` | immutable run directories, pass^k, summaries |
| `internal/report` | the table, the diff, JUnit XML |
| `internal/tui` | the read-only browser |
| `internal/demo` | the embedded example |

## Background

[docs/agent-eval-concepts.md](docs/agent-eval-concepts.md) is a map of the field rather than a tour of this repository: what a trajectory is, why a null score is not a zero, the difference between pass@k and pass^k, which metric families exist and what each platform chose. It explains the decisions above rather than restating them.

## Prior art

- **Braintrust** — hosted experiments, and a nil score for "not applicable". This keeps that score contract and stores the run locally.
- **Raindrop** — hosted agent monitoring. This scores local traces and does not phone home.
- **[mira](https://github.com/everruns/mira)** — Study, Subject and Scorer, with JUnit output. This is one binary and a YAML suite, not a protocol host.
- **ADK eval** — tests written inside an agent framework. This scores any process that speaks JSON on stdio.
- **Inspect AI** — a Python framework of models, solvers and datasets. The deterministic trajectory checks here need no model.
- **promptfoo** — prompt and assertion matrices. This grades tool trajectories.
- **DeepEval** — Python metrics, many of them LLM judges. The default suite here needs no model and no API key.

## License

MIT. See [LICENSE](LICENSE).
