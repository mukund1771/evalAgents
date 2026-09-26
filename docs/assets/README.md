# Assets for the README

Drop the screen recording and any screenshots here, then link them from the top
of the main [README](../../README.md) — there is a marked comment block where the
recording goes.

## What to capture

The recording should tell the same story the README does, in about 40 seconds.
Everything below is one terminal, no editing needed:

```bash
agenteval demo          # scores a suite, scores a regression, diffs them
agenteval tui           # then: enter, j, enter, G, esc, esc, space, j, space, d
```

Worth making sure these land on screen, because they are the four things that
distinguish this from an LLM output eval:

1. The `-` column in the run table — a skipped scorer, not a zero.
2. A case that fails on `tools_succeeded` while `contains` still reads 1.00.
3. The `[[ ]]` evidence markers in the trajectory view.
4. `diff` exiting 1, and the reverse direction exiting 0.

A wide terminal helps: the example suite's table needs about 95 columns, and the
full ten-scorer suite in `agenteval demo` needs about 140.

## Formats

**GIF** — commit it here as `demo.gif` and reference it with
`![agenteval demo](docs/assets/demo.gif)`. Works in every markdown renderer and
in an offline clone. Keep it under a few MB; `agg` or `terminalizer` both produce
reasonable output from an `asciinema` cast.

**MP4** — better quality and it plays with sound, but the file cannot live in the
repo and render inline. Drag it into any issue or PR comment on this repository,
copy the `https://github.com/user-attachments/assets/...` URL that appears, and
paste that URL on its own line in the README. GitHub turns a bare URL into a
player.

**Screenshots** — `runs.png`, `run-detail.png`, `case-detail.png`, `diff.png` if
you want them beside the plain-text frames already in the README. The text frames
are real output and stay useful where images do not load, so they should stay
either way.
