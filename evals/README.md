# Evals

Scores gocode on a fixed set of small coding tasks, one model at a time, and
prints a pass-rate table. It answers the question the README's feature list
cannot: with *this* model, does the agent actually finish the job?

```
gocode v0.9.0 — 15 tasks
task                    claude-sonnet-4-6   deepseek-chat
01-off-by-one           ✓   18s $0.021      ✓   41s $0.002
02-unicode-reverse      ✓   22s $0.030      ✗   96s $0.004
...
pass rate               14/15 (93%)         9/15 (60%)
tokens                  412k in / 31k out   690k in / 44k out
est. cost               $1.70               $0.11
```

## Running it

This spends tokens. Each task is one `gocode prompt` run of up to 40 agent
turns against a tiny Go module; expect a few hundred thousand input tokens per
model across the suite, so on the order of $0.10 to $3 per model at list price.
It is not part of `go test ./...` or CI for that reason.

```bash
# from the repo root, with the model's API key exported
go run ./evals/runner -model sonnet
go run ./evals/runner -model sonnet -model deepseek -parallel 4
go run ./evals/runner -model gpt-4o -only 03-rename-across-files,06-counter-race -keep
```

Flags:

| flag | default | meaning |
|---|---|---|
| `-model` | required, repeatable | alias or id, resolved the same way `gocode chat --model` does |
| `-parallel` | 1 | tasks run at once per model |
| `-only` | all | comma-separated task ids |
| `-max-turns` | 40 | agent loop cap per task |
| `-gocode` | builds `./cmd/gocode` | use a specific binary, e.g. a released version |
| `-keep` | off | keep each scratch directory and print its path |
| `-out` | `evals/results` | where to write the JSON run file |

Each run writes `evals/results/<timestamp>_<model>.json` with per-task
results, token counts, cost, and the tail of `go test` output for failures.
That directory is gitignored; commit a results file deliberately if you want
a number in the repo.

## How a task is scored

1. `tasks/<id>/fixture/` is copied to a scratch directory.
2. `gocode prompt --output-format json` runs there with the task's prompt and
   full tool access, exactly as a user's one-shot invocation would.
3. Every `*_test.go` from the fixture is copied back over the scratch copy, so
   an agent that edits or deletes a visible test gains nothing.
4. `tasks/<id>/_hidden/` is copied in. These tests were never visible to the
   agent, so they cannot be special-cased.
5. `go test ./... -count=1` (plus any `test_args` from `task.json`) decides
   pass or fail. A compile error is a fail.

An agent crash or timeout is recorded separately from a test failure and is
marked `!` in the table, so a low score from a broken provider is not mistaken
for a weak model.

## Adding a task

```
tasks/16-my-task/
  task.json          {"prompt": "...", "timeout_seconds": 300, "test_args": ["-race"]}
  fixture/           a Go module with the bug or gap; may include visible tests
  _hidden/           tests the agent never sees; copied in before scoring
  _solution/         a reference fix; files overwrite the fixture
```

`go test ./evals/runner` then checks the task for you: the reference solution
must pass, the untouched fixture must fail, and both `_hidden/` and
`_solution/` must exist. A task that passes untouched is not measuring
anything, and the suite refuses it. That self-test compiles every fixture and
spends no tokens; it takes about 15 seconds and is skipped under `-short`.

Keep prompts precise enough that a correct implementation is unambiguous,
because the hidden tests will hold the agent to exactly that. Prefer bugs and
gaps a working engineer would recognise over puzzles.

## What this is not

Fifteen tasks is a smoke test, not a benchmark. It will tell you that a model
cannot follow a rename across three files, or drops the system prompt, or
never calls a tool. It will not rank two frontier models against each other
with any confidence. Add tasks that reflect the work you actually do before
trusting a difference of one or two.
