# Agent evaluation: the concepts and the platforms

This is a map of the field, not a tour of one repository. The products named in the original brief — Braintrust, Raindrop, and the open-source eval toolkits around them — are different answers to the same question: how do you know an agent did the job, and how do you know a change made it better.

An agent here means a model that can take more than one step. It can call a tool, read the result, call another tool, and only then answer. That extra structure is the whole subject.

## The problem

A normal program fails loudly. A test expects `3` and gets `4`, and the suite is red. An agent fails quietly. It can say something fluent and wrong, call the right tool with the wrong argument, call the right tools in an order that still loses the user's goal, or succeed once and fail the next four times with the same input.

So the field split into two jobs that people often mix up.

**Evaluation** asks a fixed question of a frozen set of cases. Did this version complete the task, call the right tools, stay within a step budget, and do so more than once? You run it before you ship, and you run it again when you change a prompt, a model, or a tool.

**Monitoring** asks an open question of live traffic. Of the runs that happened today, which ones looped, hallucinated, broke a tool, or frustrated the user? You do not have a gold answer for most of those. You have a trace and a signal.

Braintrust started on the first job and grew into the second. Raindrop started on the second and grew a way to turn a production failure back into a test. The local tools (Inspect, promptfoo, DeepEval, mira, ADK eval, and this repo) are mostly the first job, done on your machine.

## An LLM eval is not an agent eval

An LLM eval scores a string. You send a prompt, you get a completion, you compare it to a reference or hand it to a judge. The unit of work is one call.

An agent eval scores a **trajectory**: the ordered record of what the agent did. A typical trajectory contains:

- the user message
- each assistant step, including the tool it decided to call and the arguments
- each tool result, including whether the tool returned an error
- the final answer
- operational numbers: tokens, cost, latency

The final answer can be perfect and the trajectory still bad. A refund agent that says "I've issued your refund" without calling `issue_refund` has failed, even if the sentence matches the expected text. A support agent that calls `lookup_order` three times with the same arguments is looping, even if it eventually answers. That is why every serious agent eval keeps the steps, and why a scorer that only sees `final_output` is an LLM eval wearing an agent costume.

People use four words for almost the same object. They are not quite the same.

| Word | What it usually means |
| --- | --- |
| Trajectory | The steps the eval harness cares about: messages, tool calls, tool results, final output. A scoring format. |
| Transcript | The same idea, often including the raw conversation the judge is allowed to see. |
| Trace | A production telemetry record of one run. It may contain many spans, retries, and nested model calls. |
| Span | One node inside a trace: a single model call, a single tool call, a retrieval. Spans have a start, an end, and a parent. |

OpenTelemetry is the common transport for traces. A span there is a timed operation, not yet an eval. A product either scores the whole trace once, or scores individual spans. Braintrust documents both scopes. Scoring a span answers "was this tool call well formed?" Scoring a trace answers "did the task get done?" Agent quality is almost always a trace-level question.

## The objects every system has

The names change. The objects do not.

**Case.** One example. An input, optional expected data, and tags. Also called a sample, a row, an example, or an eval case. The input for an agent is often a user message plus whatever state the agent needs (an order id, a note, a file). The expected data is not always a full reference answer. It might be a substring, a JSON schema, or the list of tools that should have been called.

**Dataset or suite.** The frozen list of cases. Frozen matters. If the cases change at the same time as the agent, you cannot tell whether the agent improved. Braintrust calls this a dataset. Inspect calls the combination of data, solver, and scorer a task. ADK calls the file an eval set. promptfoo calls it a test file. This repo calls it a suite.

**Target.** The thing under test. A model, a prompt, a binary, or a live agent. mira calls it a subject. Inspect calls the component that produces output a solver. The important design choice is whether the eval tool runs the agent, or only scores a trajectory something else already produced. Running the agent is how you test a change. Scoring a recorded trajectory is how you test the scorers, and how you grade traces a production system already emits.

**Scorer.** A function from `(case, trajectory)` to a judgment. Also called a grader, a metric, an assertion, or a signal. A scorer should do one thing. "Is the final text valid JSON?" is a scorer. "Is this agent good?" is not.

**Score.** The judgment itself. The useful contract, which Braintrust uses and which is easy to get wrong, is:

- a number, conventionally from 0 to 1
- an optional pass or fail, usually `value >= threshold`
- a missing value that means **not applicable**, not zero

Zero means "this was graded and it failed." Null means "this scorer does not apply to this case, so do not let it move the average." If `json_valid` runs on a prose answer and records 0, it punishes a case that was never supposed to return JSON. If it records null, the case is judged only by the scorers that had something to say.

**Evidence.** A pointer from the score back to the part of the trajectory that caused it. A step index, a span id, or a character range. Without it, a score of 0.4 is a number you cannot audit. With it, you can open the trace at the tool call that mismatched. This is the difference between a leaderboard and a debugging tool.

**Run or experiment.** One execution of a dataset against one configuration, stored so it can be compared later. The configuration is the model, the prompt, the tools, and the scorers. If you overwrite the previous run, you have lost the baseline. Immutable runs are how "did this change help?" becomes a factual question.

## What scorers actually measure

Agent metrics fall into a few families. Products rename them. The families stay.

### Output checks

These look only at the final answer. They are cheap and deterministic.

- **Exact match.** The string equals the reference. Brittle, and right when the output is a code, an id, or a canned sentence.
- **Contains, regex, JSON schema.** The answer has a required phrase, shape, or field. This is most of promptfoo's built-in assertions.
- **Semantic similarity.** Embed the answer and the reference, then take cosine similarity. It is tempting and usually the wrong default. It rewards paraphrase and does not know whether a tool was called. None of the serious agent suites use it as the primary gate.

### Tool-use checks

These look at the calls, not the prose. They are the part that makes it an agent eval.

- **Exact trajectory.** The sequence of tool name plus arguments equals the expected sequence. ADK's `tool_trajectory_avg_score` is this idea: in-order match of the calls. One extra call fails it. Use it when the procedure is the task, as in "look up the order, then issue the refund."
- **Subsequence.** The expected calls appear in order, possibly with extra calls around them. Partial credit is the fraction of the expected sequence you covered (longest common subsequence divided by the expected length). An agent that does the right steps and then one unnecessary lookup still gets most of the credit.
- **Tool-call F1.** Ignore order. Treat the calls as a multiset of `(name, canonical arguments)`. Precision is the fraction of actual calls that were expected. Recall is the fraction of expected calls that happened. F1 is the harmonic mean. This is the right score when several independent lookups may happen in any order. Lyzr splits the same idea into **tool correctness** (did it pick the right tool) and **argument correctness** (were the types and values right). The F1 version scores both at once, because a right tool with wrong arguments is not a match.
- **Tools succeeded.** A binary check that no tool step came back as an error. The trajectory can look perfect and still be a failure if `lookup_order` timed out. The score should name the call that failed.
- **Step budget and loop detection.** `steps_within` fails a trajectory that wandered. `no_loop` fails the same tool with the same arguments repeated past a limit. Loops are one of the silent failures Raindrop's signals are built to catch in production. Offline, you can assert them directly because you have the calls.

Argument comparison has to be canonical. `{"b": 1, "a": 2}` and `{"a": 2, "b": 1}` are the same call. JSON object key order is not part of the call.

### Reference-free quality

These do not need a gold trajectory. They need a rubric, and usually a second model.

- **Task completion.** Did the agent accomplish what the user asked? Lyzr, DeepEval, and most judges use this name. It is the headline number, and it is the easiest one to fake, because the judge is looking at prose.
- **Faithfulness.** Is the answer grounded in the retrieved documents or the tool results, rather than invented? This is a RAG word that transferred to agents. The source of truth is the tool results in the trajectory, not the model's memory.
- **Hallucination.** The complement of faithfulness, often measured as a rate of unsupported claims. Lyzr lists both.
- **Answer relevancy.** Did the response address the question, even if it is factually grounded?
- **Safety.** Toxicity, bias, policy refusal. Real categories, and a different product surface from tool accuracy.

### Operational checks

Tokens, dollar cost, latency, time to first token. mira treats these as budgets (`tokens_within`, `cost_within`). They do not tell you the answer was right. They tell you the agent stayed inside the envelope you are willing to pay for. A change that raises task completion from 0.7 to 0.9 and triples cost is not an automatic win.

## Deterministic scorers and judges

A **deterministic scorer** is a pure function. Same trajectory, same score, forever. Exact match, trajectory match, F1, step budgets, and loop checks are in this family. They are the right default for a gate. They cannot tell you whether a paragraph of advice was wise.

A **model-graded scorer** (LLM-as-judge) sends the case and the trajectory to a second model with a rubric and parses a score. DeepEval's G-Eval is a structured version of this: the judge is given criteria and evaluation steps, and the score is derived from its probabilities or from a constrained output, not from a shrug in prose. Braintrust, Inspect, promptfoo (`llm-rubric`), and Lyzr all ship a judge path.

Judges have three known failure modes.

1. They prefer longer, more confident answers. A polished wrong answer beats a short right one.
2. They do not reliably follow a numeric rubric unless the output is forced into a schema. Free text like "I'd give this a 7" will be parsed wrong.
3. They hallucinate evidence. A judge that says "the error is in step 14" when the trajectory has 6 steps has not scored anything.

The defense is the same one you would use for any untrusted parser. Require a JSON object with a value in range and spans that point at real steps. If the object is invalid, retry once. If it is still invalid, record null, not a guessed number. A missing score cannot flip a gate. A fabricated 0.9 can. Lyzr's own material treats LLM classification as less deterministic than a code check, which is the reason to keep the judge off unless the suite asks for it, and to keep task completion from being the only metric.

A judge is still the right tool when the expected behavior cannot be written as a string or a tool list. "Did this clinical extraction capture the medications that are actually in the note, without adding any?" is a faithfulness question. You can check the JSON schema deterministically, and you still want a judge, or a human, on whether the fields match the note.

## Reference-based and reference-free

**Reference-based** means the case carries an expected value written ahead of time. Exact tool lists, gold JSON, a reference sentence. Cheap, stable, and only as good as the gold data. A wrong expected tool sequence will fail a correct agent forever.

**Reference-free** means the scorer looks at the input and the trajectory and applies a rule or a rubric. "No tool errored." "The answer is valid JSON." "A judge thinks the task was completed." You can run these on production traces, because production has no gold file.

Most real suites mix them. The tool sequence is reference-based. "No tool errored" and "no loop" are reference-free and therefore run on every case. The judge is reference-free and optional.

## Reliability: why one run is not a measurement

Agents sample. Temperature, tool flakes, and race conditions mean the same case can pass and then fail. A single green run is an anecdote.

Two different statistics get abbreviated in ways that look similar.

**pass@k** comes from code generation (HumanEval). Draw `n` samples. Estimate the probability that **at least one** of `k` samples is correct. It answers "if I am allowed to try k times and keep the best, do I succeed?" That is the right question for a coding agent with a retry button.

**pass^k** comes from τ-bench (tau-bench), which evaluates tool-using agents on multi-step tasks. Draw `n` trials with `c` successes. The estimate is the number of ways to choose `k` successes divided by the number of ways to choose `k` trials: `C(c, k) / C(n, k)`, and 0 when `c < k`. It answers "if I must succeed on all k independent tries, do I?" For `k = n` that is 1 only when every trial passed. An agent that completes a refund four times out of five has a high pass@1 and a poor pass^5. Customer-facing agents need the second number. A gate that reports only the mean of one run will ship the flaky agent.

Repeating a **replay** of a frozen trajectory does not measure this. The cassette is deterministic, so pass^k equals pass^1. Repeats matter when the target is a live model. The formula still belongs in the runner so the live path is not a different product.

The other half of reliability is the fixture. A **cassette** (also called a fixture, a recording, or a VCR tape) is a saved trajectory or a saved HTTP response. The next run reads it instead of calling the model or the tool. Two rules make fixtures trustworthy:

- **Record explicitly.** You know when the tape was made and what produced it.
- **Fail closed.** If the suite asks for a cassette and the file is missing, the run errors. It does not quietly call the network. A fallback to live is how an "offline" demo starts depending on an API key and a model that changed yesterday.

Replay of a trajectory scores the trace. Replay of HTTP (what a VCR library does) re-executes the agent against saved tool responses. Those are different. Trajectory replay tests scorers and grades traces you already have. HTTP replay tests the agent's decisions given frozen tool outputs. Raindrop Workshop's replay is closer to the second: take a real trace and run the agent code again, locally, against that situation.

## Knowing a change helped

Store every run. Do not update the previous one in place. Then compare.

A **baseline** is a chosen earlier run, not "whatever the mean was last week" recomputed from scratch. Comparison has to be per case and per scorer. An average can rise because you added easy cases. A real regression is one of:

- a case that passed on the baseline and fails now
- a scorer whose mean, on the cases both runs share, dropped

New cases are news, not regressions. Removed cases are news, not regressions. The process should exit non-zero on a regression so a CI job can block the merge. JUnit XML is the boring format that GitHub, Jenkins, and almost every test UI already understand. That is why mira, promptfoo, and a small local runner all bother to emit it. The product being sold is not the table. It is the gate.

Braintrust's version of this is an **experiment**: a named run of a dataset, comparable in the UI, with scorers attached. You can also rescore an old experiment without re-running the agent, which matters when the judge prompt improved and the trajectories are still valid. Raindrop's version is an **A/B experiment on live traffic**, often behind a feature flag: ship the change to a slice of real sessions and watch whether the signal rate moved. Offline diff tells you the frozen set did not get worse. Online experiment tells you real sessions got better. You want both, and they answer different questions.

## Offline eval and online monitoring

| | Offline eval | Online monitoring |
| --- | --- | --- |
| Input | A dataset you wrote or curated | Production traces |
| Ground truth | Often present | Usually absent |
| Question | Did this version pass the suite? | What is breaking, for whom, since when? |
| Failure mode it misses | Anything the dataset does not contain | Subtle one-off failures if your signals are weak |
| Typical product | Braintrust experiments, Inspect, promptfoo, ADK eval, this runner | Raindrop, Braintrust logs with online scoring, LangSmith |

**Online scoring** means attaching a scorer to traces as they arrive, continuously. Braintrust supports span scope (one model call), trace scope (one agent run), and group scope (several related traces). Raindrop's **signals** are the same idea under a monitoring name: a classifier that labels every interaction so you can plot tool errors, refusals, or user frustration over time.

Raindrop also separates a **stumble** from an **issue**. A stumble is one bad run. An issue is the same stumble repeating across users, grouped and ranked, the way Sentry groups exceptions. That grouping is the part a local eval runner does not do, and does not need to do, until you have production volume.

The loop the best teams close looks like this. A signal fires on a live trace. Someone, or a coding agent, turns that trace into a case with an assertion. The assertion joins the offline suite. The next prompt change has to pass it. Raindrop Workshop is explicit about this loop: read the span, write the eval from the real failure, rerun until it passes. A hand-written suite that never absorbs production failures goes stale. A monitor that never creates a regression test will page you for the same bug after every deploy.

## What each platform is for

### Braintrust

Braintrust is a hosted eval and observability product. You upload a dataset, define scorers in code or in the UI, and run experiments. A scorer returns a number from 0 to 1 and may return null to skip. Experiments are comparable. Traces are first-class: a trace-level scorer can see every span in the run, and a span-level scorer judges one model or tool call. The same scorers can run online on production logs, and you can rescore stored outputs without calling the agent again.

Use it when the dataset, the experiment history, and the review UI are the product, and a vendor can hold the traces. Do not use it when the requirement is a single binary that never phones home.

### Raindrop

Raindrop is a monitoring product for agents in production, closer to Sentry than to pytest. It records messages, tool calls, retries, and errors, then looks for silent failures: hallucinations, loops, forgotten context, broken tools. Signals score those behaviors across traffic. Stumbles group into issues. A triage agent investigates from Slack or over MCP. Experiments compare a change on live traffic behind a flag.

Workshop is the open-source local half. It streams spans to a local UI as the agent runs, exposes them to a coding agent, can replay a trace against your code, and is built to turn a failure into an eval. The hosted product and the local debugger share the same SDK shape so instrumentation is not rewritten at deploy time.

Use Raindrop when the question is "what are real sessions doing?" Use an offline runner when the question is "does this commit pass the suite?"

### Lyzr

Lyzr sells the agents and the evaluation around them, including a simulation engine. The published metrics are the ones the rest of the field uses, under their names: `task_completion`, hallucinations, `faithfulness`, toxicity, bias, tool accuracy, and the finer split of tool correctness versus argument correctness. Simulation generates personas and scenarios, runs them at the agent, and scores the conversations. The improvement loop looks at live traces and proposes changes to the agent's instructions.

Their metric names are a useful vocabulary even if you never call their API. `task_completion` is the judge question. Tool accuracy is the deterministic tool question. Faithfulness is the grounding question. A local runner that uses those names is speaking the same language as the product, on purpose.

### mira

mira is a Rust toolkit, not a hosted product. A study owns subjects and scorers. A subject turns a sample into a transcript: an in-process function, an external binary, or a live runtime. Scorers are ordinary code, including trajectory checks (`tool_called_with`, `steps_within`), operational budgets, and one `model_graded` judge. A score can be N/A, and N/A is excluded from the pass rate rather than counted as a failure. The CLI saves runs and can emit JUnit.

The idea worth stealing is the subject boundary. The eval host does not contain the agent. Any language that can be executed can be a subject. The idea worth not copying, for a two-day local tool, is the protocol between a host process and a study process. A YAML file and a binary on stdin is a smaller version of the same boundary.

### Inspect AI

Inspect is the UK AI Safety Institute's Python framework, and the one most research groups mean by "an eval harness." A task binds a dataset, a solver, and scorers. Solvers can be a model, a chain, or an agent with tools and a sandbox. It is built for careful, reproducible experiments, including the awkward parts: epochs (repeats), multiple models, and logging every message. It is a library and a CLI you run yourself, not a multi-tenant app.

Use it when the eval is a research object and Python is the lab. It is a poor fit when the requirement is a static binary with no Python environment on the reviewer's laptop.

### promptfoo

promptfoo is a YAML-and-CLI test runner aimed at prompts, RAG, and red teaming. A test has variables, a list of providers (the matrix of models or endpoints), and assertions: contains, equals, JavaScript, similar, `llm-rubric`, and others. It shines when you want to know which model and which prompt pass the same assertions, and when you want a red-team scan. Agent trajectories are not its center of gravity. You can assert on structured output, but you are mostly grading completions and HTTP responses, not a typed tool sequence with evidence spans.

### DeepEval

DeepEval is a Python library in the pytest style, from Confident AI. You write test cases with input, actual output, expected output, and retrieval context, then attach metrics. The catalog is the standard RAG-and-agent list: answer relevancy, faithfulness, contextual precision and recall, hallucination, G-Eval, tool correctness, task completion. Many of those metrics are LLM judges with a threshold you set like a pytest assertion (`assert_test` fails under 0.7). Confident AI is the hosted dashboard around the same metrics.

It is the fastest way to get a faithfulness number in a Python codebase. It is the wrong default when you want the number to be a pure function of the tool calls.

### ADK eval

Google's Agent Development Kit ships an eval command for agents built in ADK. An eval set is a JSON file of conversations. Each turn can carry an expected tool trajectory and a reference response. The built-in scores are `tool_trajectory_avg_score` (in-order tool match, averaged) and `response_match_score` (overlap between the final response and the reference, in the ROUGE family). You run it with `adk eval` or from pytest.

It is the cleanest illustration of the trajectory-versus-response split, and it only evaluates ADK agents. A `cmd` target that speaks JSON on stdin is the same idea with the framework removed.

### The neighbors

LangSmith (LangChain) and Langfuse are tracing products with evals attached. You instrument the agent, you get a tree of spans, you build a dataset from production runs, you attach evaluators. They belong in the same mental shelf as Braintrust logs and Raindrop, not in the shelf with pytest. If a team already lives in LangChain, LangSmith is the path of least resistance. If the requirement is that traces never leave the building, Langfuse is the one you can host yourself.

The named benchmarks are datasets plus a harness, not products. SWE-bench grades code agents on real GitHub issues. τ-bench grades tool-using agents across multiple trials and is the source of pass^k. WebArena and GAIA grade agents that browse or use tools on fixed tasks. You do not "install" them as your eval platform. You might use their cases as a suite, and you still need a runner.

## How a small local runner sits in this map

A local runner is the offline, deterministic slice of the field, with the hosted parts left out on purpose.

It has a suite (the dataset), a `cmd` target (mira's external subject, ADK's agent runner, without the framework), and a `replay` target (Braintrust's "rescore without re-running," in the cheapest form: the trajectory is a file). Its scorers cover the output family and the tool family. The judge exists, takes an OpenAI-compatible endpoint, and stays out of the default suite. Scores use the null-means-skipped contract. Evidence points at a step. Runs are immutable JSONL so a diff can exit 1. pass^k is reported so a live target can be held to τ-bench's standard. JUnit is there so the suite can be a CI gate.

It does not monitor production, group stumbles into issues, run a hosted experiment UI, generate personas, or red-team a model. Those are Raindrop, Braintrust, Lyzr's simulation engine, and promptfoo. Leaving them out is the product decision. A founder who sells on-prem agents cannot adopt an eval tool that requires a vendor account. A reviewer who was given two sentences and no spec is grading whether you knew which slice to build.

## Concepts worth being precise about

**Threshold.** A score of 0.8 is not a pass until you say the minimum is 0.8. Exact-match gates use 1. Partial-credit metrics (F1, subsequence) need an explicit minimum or they silently fail every imperfect trajectory. Put the minimum on the scorer, not in a dashboard setting nobody can see.

**Aggregation.** Average the non-null scores. Do not average in the skips. A case passes when every applicable score passes, not when the mean clears a bar. A mean hides one hard failure behind nine easy passes.

**Canonicalization.** Tool arguments match after parsing and re-serializing JSON, so key order and insignificant whitespace do not matter. Strings for exact match should have an explicit rule about trimming. Write the rule down. Hidden trimming is a source of suites that pass on one machine and fail on another.

**Contamination.** If the model was trained on the benchmark, the score measures memorization. Private cases, or cases built from your own traces, are the ones that predict your production. Public leaderboard numbers are a different instrument.

**Overfitting the suite.** An agent that is patched until these 10 cases pass has not become a good agent. It has become good at these 10 cases. Keep a slice of cases out of the inner loop. Add production failures one at a time instead of growing the suite to whatever the latest prompt happens to pass.

**The judge grading the judge.** If the same model family writes the answer and grades it, scores drift up. Use a different model for the judge when you can, and keep at least one deterministic scorer on the same case so a friendly judge cannot be the only vote.

**Humans.** Annotation queues exist in Braintrust and LangSmith because some judgments should not be automated. A judge is a model of a rater, not a rater. For anything with a real cost of being wrong, sample the judge's scores and check them. Evidence spans are what make that check take a minute instead of a reread of the whole trace.

**Eval-driven development.** Write the case and the expected tool list before you change the prompt. A red suite that becomes green is evidence. A prompt you tweaked until the output "looked right," and then froze as the expected output, is a snapshot of the current behavior, including its bugs.

## A short glossary

| Term | Meaning |
| --- | --- |
| Case | One input, plus whatever expected fields the scorers need. |
| Suite / dataset | The frozen set of cases. |
| Target / subject / solver | The thing that turns a case into a trajectory. |
| Trajectory | Steps, tool calls, tool results, final output. The object you score. |
| Trace | A telemetry tree for one live or recorded run. |
| Span | One node in a trace, or a pointer (step index, byte range) from a score back into a trajectory. |
| Scorer / grader / assertion / signal | One check. Code or a judge. |
| Score | A number in \[0, 1\], a pass bit, or null for "not applicable." |
| Evidence | Where in the trajectory the score came from. |
| Reference-based | Uses expected data on the case. |
| Reference-free | Uses only the trajectory and a rule or rubric. |
| LLM-as-judge | A model assigns the score. Constrain its output or treat it as untrusted. |
| Cassette / fixture | A recorded trajectory or HTTP response. Missing means error. |
| Experiment / run | One immutable execution of the suite against one configuration. |
| Baseline | The run you refuse to get worse than. |
| Regression | A previously passing case now fails, or a scorer mean drops. |
| pass@k | Chance that at least one of k samples is correct. |
| pass^k | Chance that all k trials are correct. The agent-reliability number. |
| Online scoring | Scorers applied to production traces as they happen. |
| Stumble / issue | One bad run, versus the same failure grouped across runs. Raindrop's distinction. |
| Gate | A command that exits non-zero and can block a merge. |

## Where to read next

- Braintrust on writing scorers, including null and trace scope: https://www.braintrust.dev/docs/evaluate/write-scorers
- Raindrop's product frame (monitoring, signals, experiments): https://www.raindrop.ai/docs/introduction/
- Raindrop Workshop (local traces, replay, failure-to-eval): https://github.com/raindrop-ai/workshop/
- Lyzr's metric list: https://docs.lyzr.ai/enterprise/get-started/concepts/evaluating-agents
- mira's scorer model, including N/A: https://github.com/everruns/mira/blob/main/docs/scorers.md
- Inspect AI: https://inspect.aisi.org.uk/
- promptfoo assertions: https://www.promptfoo.dev/docs/configuration/expected-outputs/
- DeepEval metrics: https://deepeval.com/docs/metrics-introduction
- τ-bench, for pass^k and tool-using agents: https://arxiv.org/abs/2406.12045

The local counterpart of all of the above is the suite, the trajectory types, and the scorers in this repository. The concepts those files implement are the ones in this document. The platforms are what you get when the same concepts grow a UI, a production ingest path, or a research harness.
