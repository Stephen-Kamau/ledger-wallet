# Test strategy and standard

<!-- ag-init managed: canonical instructions, refreshed on every run. This repo's answers live in docs/standing/test-standard.implementation.md. -->

How this repo's tests are designed, what they are held to, and how we know they work. It is
written for a person deciding whether to trust the suite - a reviewer, a new maintainer, a
client's QA lead - and it is the criteria document every review scores tests against: a
finding that contradicts a clause here names the clause.

This file holds canonical's instructions and `ag-init` refreshes it on every run, so write
nothing here. This repo's answers - its wall-time target, test runner, coverage and mutation
commands and the platforms it supports - live in `docs/standing/test-standard.implementation.md`,
which `ag-init` seeds once and never compares or overwrites. A review reads both files. The
AI-gile commands use the canonical install path (`$HOME/.claude/aigile-canonical/scripts/...`);
the answers file names your own where this repo has one.

## What the suite is for

In priority order:

1. **Catch defects.** A behaviour change that a user or a caller would notice fails a test.
   Everything else serves this.
2. **Run on every change.** Fast enough that nobody skips it: the whole suite within the
   wall-time target the answers file names, on the development host. Wall time is what a person waits for, so it is
   the measure - not CPU, not test count.
3. **Mean the same thing everywhere.** The same result in any order, on any worker, in a fresh
   clone, on every platform this repo supports. A test that passes only in some order is a
   defect, not a flake.
4. **Stay cheap to keep.** Every test, helper and rule has to earn its place. A test nobody can
   justify is deleted.

## The shape of the suite

Every test has a SIZE, borrowed from Google's taxonomy. Size is about what a test may touch.

| | Small | Medium |
| --- | --- | --- |
| May launch a process | No | Yes |
| May write to disk | No, except in a wider-than-function fixture or a `shared_build()` block | Yes |
| May use real git, a database or a network service | No | Yes |
| Default? | Yes - most tests | Only where the boundary is the point |
| Declared by | Nothing | A marker naming what only this test proves |

**Why so strict.** Launching processes and creating files is where suites lose time and gain
flakiness: process start-up and file creation cost more as parallel workers grow, and shared
temp trees, inherited environment and half-built repositories make results depend on what ran
beside them. A small test is a function call on values - fast, order-independent and portable
by construction.

**What a medium test is for.** Exactly the properties only a real boundary can show: that a
reader parses what the real tool actually prints, that a command's exit code reaches a shell,
that a file lands where it should. One medium test per reader per output shape, one per process
boundary per exit path. Everything else about the same code is a small test.

## How code is made testable: readers and decisions

Most code reads some facts (files, git, the environment, a service) and then decides something.
Split the two:

- A **reader** gathers the facts once and hands them on.
- A **decision** is a pure function that takes those facts as arguments and returns a verdict.

The decision gets small tests that pass it content directly: every case, every edge, in
milliseconds. The reader gets a medium test against the real thing for every output shape the
decision consumes.

**Why this does not lose defects.** A defect lives in one of three places:

- **In the decision.** Small tests cover every case directly, more than an end-to-end test could.
- **In the reader**, which handles a real output shape wrongly. Each shape a decision consumes
  (a rename in `git status`, a quoted path, an empty result) has a medium test on the real tool.
- **Between the two.** The decision was tested on an input the reader never produces. This is
  the real risk of the method, and it is closed by making the connection checkable: a test that
  runs the real reader and asserts a shape is marked as PROVING that shape, and a test that
  hands a decision hand-written reader output, or fakes the process a reader calls, is marked as
  USING it. A check fails the suite when a used shape has no proof, or when a small test fakes a
  process without naming the shape it stands in for.

## How we know the tests catch defects

Two measures, each answering a different question. Both are REPORTED, not gates: a number that
blocks a commit gets gamed, and these are for people to read and act on, unless a burst or plan
names a figure as its own exit criterion.

| Measure | Question it answers | What it cannot tell you | Command |
| --- | --- | --- | --- |
| Coverage | Which lines and branches did the suite run? | Whether any test would notice them change | The coverage command the answers file names |
| Mutation | If this code changed, would a test fail? | Anything about code outside the sample | `$HOME/.claude/aigile-canonical/scripts/ag-mutate --base <sha>` |

**Coverage targets.** Line coverage 85-95% and branch coverage at least 75%, per module of 20
or more executable statements, measured over this repo's own source with child processes
included where the tool supports it. A shorter module is reported individually with no
threshold, never folded silently into an average. Changed code in a commit: at least 90%
(Google's per-commit goal). The 85-95% band and the 95% ceiling are local policy: above 95% is as
much a finding as below 85%, because it often means tests are pinning structure, not
behaviour. Fowler argues only that high coverage is not the goal ("you are testing too much
if you can remove tests while still having enough") and sets no numeric cutoff.

**Mutation testing.** A mutant is one small deliberate change to the code - flip a comparison,
swap `and` for `or`, negate a condition, nudge a constant, return `None`, delete a statement. If
the tests still pass, nobody checks that code. Each mutant is applied in a private worktree
(the real tree is never touched) and only the tests that execute the changed line run. The
outcome is KILLED (a test failed), SURVIVED (all passed), TIMEOUT (never finished: noticed, so
counted as killed) or UNCOVERED (no test executes the line).

Two scopes answer two questions.

- **Burst** (over the lines the burst changed): **detection rate = killed, timeouts included,
  / ALL mutants tried.** "If we change this code, do our tests fire?" An uncovered mutant is a
  miss; its count is reported alongside.
- **Repo** (a ledger with one row per function): **mutation coverage = functions with a CURRENT
  row / all mutable functions**, plus the detection rate across current rows. A row is current
  while the hash of its function's source is unchanged; edit the function and the row is stale,
  counts for nothing, and is replaced the next time that function is run. Staleness is judged
  by code version, never by age.

Survivors, not percentages, are what a reviewer acts on. Each is read. A survivor that changes
behaviour gets a test that kills it, shown to fail against that mutant. One that cannot change
behaviour is recorded as equivalent. One that sits in dead code means the code is deleted.

**Mutation and adversarial review.** Mutation finds tests that do not check code that exists;
review finds behaviour nobody wrote a test for. Neither replaces the other.

**Red first.** New behaviour starts with a failing test, written against a public function or
the CLI, never a private helper. The test is shown to fail before the code makes it pass.

## The rules a test is held to

### A test must justify its existence

A kept test can answer four questions. A test that cannot is deleted; deletion is the default.

1. **What does it guard?** A named defect, incident, backlog entry, or a behaviour a consumer
   relies on.
2. **Is that path live?** The code it exercises still has a consumer.
3. **Why is it the only test asserting this?** If another test asserts the same property, the
   two merge. A test whose covered lines are a subset of another's, asserting nothing the other
   does not, is a redundancy candidate.
4. **If it is medium, what does the boundary buy?** Named where the test is declared.

"A rule required it" answers none of them.

### Writing the test

- **Behaviour, not structure** (Beck). Assert what the code does, through its public function
  or CLI, so a refactor that keeps the behaviour keeps the test green. Patch only external
  boundaries - the network, the clock, a remote service - never a symbol this repo defines.
- **One equivalence class per case**, plus its boundaries. A further case names the defect it
  guards.
- **The docstring says why**: the property, and the incident if there is one, so a reader
  deciding whether to delete it can tell what would break.
- **Readable on its own** (Google: DAMP over DRY). A shared helper exists only where several
  tests need the same setup or assertion, and is named for what it packages.
- **Tests of non-code** - a document, a prompt an agent follows - carry a `Justification:` line
  naming the consumer that reads it and the failure the test guards.
- **A test reaches no external system it does not need.** The repository root, a commit list, a
  file set, a config value: pass them in or build them in code. A real git, database or network
  call is justified only when that system's behaviour is the property under test.

## Known limits

- **Mutation is a sample.** A burst's run covers the modules it changed, capped per module;
  untouched code is not re-measured each time. Repo mutation coverage says how much has been.
- **Coverage shows what ran, not what was checked.** A line can be executed by a test that
  asserts nothing about it; mutation is the answer to that, and it has its own limits above.
- **Hand-fed text that patches nothing** cannot be detected mechanically. Marking it as using a
  shape is the author's job and the reviewer's to check.
- **A test that spawns a child process** usually reports no coverage under its own context,
  because the child does not inherit the parent's. Such tests are counted as unmeasured and
  excluded from redundancy claims, never counted as clean.
- **Wall time depends on load.** The figure is measured on the development host; a loaded
  machine runs slower, and a different host is not comparable.

## Sources

- **Kent Beck, *Test Desiderata*** - tests coupled to behaviour and decoupled from structure.
- ***Software Engineering at Google*, ch. 11-12** - test sizes; test via public APIs; DAMP.
- **Robert C. Martin, F.I.R.S.T.** - fast, independent, repeatable, self-validating, timely.
  Self-validating is measured here by mutation.
- **Martin Fowler, "TestCoverage"** - coverage in the upper 80s or 90s; 100% is a smell.
- **Google's 60/75/90 coverage guidance** - 90% per commit for new and changed code.
- **Mutation testing** (DeMillo, Lipton and Sayward, 1978; Google's mutation testing at scale,
  Petrovic et al., 2018) - survivors, not percentages, are what a reviewer acts on.
