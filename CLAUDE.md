# CLAUDE.md

<!-- AIGILE:START - Do not edit this block. Managed by ag-init. -->

## AI-gile Rules

Directives only. The reasoning, the incidents each rule was written after, and the
history behind them are in [`docs/standing/aigile-rules-rationale.md`](docs/standing/aigile-rules-rationale.md),
linked per section. Read a rule here; read why it exists there.

### Trust the builder, or do not use one

**We have picked. We trust it.** So do not scaffold as if we have not.

**The test, before building any guard: if this mechanism is needed, is the agent
fit to be writing code here at all?**

**What it does NOT license.** Three kinds of control survive it:

- The outcome is **unrecoverable** - git doesn't un-send a payment, un-publish a
  release, or un-leak a secret.
- The outcome **crosses a trust boundary** - a push to a shared remote, an
  external API, another person's repository.
- The control **backs a published claim** - the enforcement stays or the prose
  goes, in the same change.

Rationale: [trust the builder](docs/standing/aigile-rules-rationale.md#trust-the-builder).

### Delete aggressively, and delete completely

**The default is delete.** Not archive, not comment out, not "leave it in case".
If a thing has no live consumer, it goes.

**Recoverability is not the test.**

**A partial removal is worse than none.** Everything that existed only to serve it
goes in the SAME change: constants, helpers, tests, documentation, and every
reference asserting it exists.

**History is not exempt, it is relocated.** The commit trail is the record. The
working tree carries only the most recent closed burst and no more (CD-151);
trimming that window is not a deletion of history and needs no retirement row,
because the commit that removed the file is the record of its removal.

**`AIGILE_HISTORY.md` is MUTABLE until the NEXT burst ships on top of it.**

**A deletion is DONE when nothing live mentions the deleted thing.** There is no
register and no enforcer - the commit that removes a thing IS the record of its
removal. Prove it by running `scripts/ag-search` UNSCOPED before declaring the
deletion done, and attach the evidence statement it emits.

**A document whose SUBJECT is deleted is deleted in the same commit.** There is no
move and no destination. `docs/archive/aigile-v1/` is a DIFFERENT thing and
survives: `scripts/ag-init` creates it during a v1 migration to preserve a client's
own pre-AI-gile artefacts - a client's property kept during adoption, not our
retired documents parked.

The CHARTER edit that removes a deleted item rides on the deletion's own
approval - see `### CHARTER: the north star, and changes are instructed`,
which states it.

Rationale: [delete aggressively](docs/standing/aigile-rules-rationale.md#delete-aggressively).

### Rule precedence

Higher tier wins; within a tier, more specific wins.

1. A HARD STOP, a mandatory gate, or a recorded stop condition.
2. This managed block.
3. A skill's own contract - its `SKILL.md` and the `docs/*.md` it cites.
4. A standing correction entry in `AIGILE_PLAN/BACKLOG.md`.
5. Harness defaults.

A `/goal` or `/loop` directive never overrides tier 1.

**Do not escalate a decision about your own machinery.** The escalation test is the
tier-1 list above, plus a push to a shared remote, an unrecoverable or outward-facing
action, a CHARTER change, or a genuine fork in DIRECTION. Everything else is decided
and stated in one line with its reason.

Rationale: [rule precedence](docs/standing/aigile-rules-rationale.md#rule-precedence).

### Session start

Act on the SessionStart hook output if it fired. Where no hook is configured,
`scripts/ag-status --quiet` reports repo state and is the fastest way to orient:

```bash
$HOME/.claude/aigile-canonical/scripts/ag-status --quiet
```

`ag-status` emits the routing instruction itself - follow what it prints. `ACTION REQUIRED`
and `AIGILE RESUME (in-flight)` are self-executing; a bare `AIGILE RESUME:` is
presented to the Orchestrator first.

Rationale: [session start](docs/standing/aigile-rules-rationale.md#session-start).

### Knowledge-graph rebuild

The rebuild happens at `/ag-probe`, once per burst, over the burst diff.
`scripts/ag-graph-rebuild` runs one by hand when a repo never reaches `/ag-probe`.

Session start does not dispatch a rebuild and must not be made to.

`ag-status` emits at most a NOTE, and only for the two states a probe-driven rebuild cannot
reach on its own: the repo has no graph at all, and a graph exists that no probe has
rebuilt. Neither is a directive - mention it once and continue.

Freshness is `scripts/ag-graph-advisory`; coverage is `scripts/ag-graph-coverage`. Never
edit the plugin file.

Rationale: [knowledge-graph rebuild](docs/standing/aigile-rules-rationale.md#understand-anything).

### Pre-flight init check

Before executing any `ag-` skill (`ag-plan`, `ag-probe`, `ag-ship`, `ag-resume`,
`ag-charter`, `ag-brief`, `ag-handoff`, `ag-stream`):

```bash
"$HOME/.claude/aigile-canonical/scripts/ag-preflight" run "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
```

- `PREFLIGHT_OK` - proceed.
- `PREFLIGHT_FAIL: ...` - **HARD STOP.** Surface the message verbatim and wait. Exit 26 is
  the branch gate; exit 25 means the repo was never initialised (run `ag-init`), preflight
  reported `PREFLIGHT_DRIFT_UNRESOLVED`, or the STATUS Phase is invalid (checked only when
  the init marker needs a refresh). A failed
  `ag-init --refresh-marker` refresh of a stale or absent marker is not a stop: preflight
  prints `WARNING: ag-preflight init marker` and `PREFLIGHT_OK`. Exit 32 is a Suspended
  repo - resume with `ag-phase transition <root> Idle`. Exit 31 is an install whose clone is
  gone - restore it, or `ag-setup --develop` another.

### The model and the cycle

One Orchestrator (the human) owns decomposition, steer checkpoints and ship decisions.
You are the build engine. Human specialists sub in for specific Bursts and hand back
when done.

```
DECOMPOSE -> PLAN -> BUILD (parallel streams) -> VERIFY+STEER (/ag-probe) -> SHIP
```

Adversarial review is integrated at decision points:

- **VERIFY** - code gets a `Reviewer`-seat adversarial review after tests pass.
- **SHIP** - both a `Ship-review`-seat verdict and a `Reviewer`-seat challenge before the
  main merge; one does not substitute for the other. `ag-gate ship-preflight` checks that a
  recorded review binds to the burst base; that BOTH seats reported is discipline, not a
  gate.

The Orchestrator decides what to act on.

### Plan first

Start every non-trivial task with `/ag-plan`, and iterate until the plan is solid before
building.

### Build commits

Committing stream work as it lands is the normal build rhythm during `Phase=Producing`.

Commits must land on `dev`. The burst base is the adversarial review anchor; commits must land on dev for the review to cover the actual diff.

Run the streams back to back. Stop only for a mandatory gate, a HARD STOP, or a decision
the Orchestrator genuinely owns.

Rationale: [build commits](docs/standing/aigile-rules-rationale.md#build-commits).

### Bursts carry a meaningful amount of work

A burst is packed to a level of effort approaching difficult.

**Pack bursts with streams.** Actively look for adjacent work the burst can absorb -
fired DEFERRED triggers, a pre-existing red test, a defect the survey turned up -
rather than shipping the narrow goal alone.

**A defect found during a burst is FIXED IN THAT BURST.** Discovery is the
trigger, not a reason to file. Two defeats, and there is no third: it has a
dependency that cannot be resolved inside the burst, or it belongs to an `AGV`
bundle whose entries must move together and that cannot be brought in whole.
Either defeat is STATED with the entry it names; everything else is fixed now. A
`PENDING:` written for a defect that fails neither test is the thing this rule
exists to stop.

**The burst coordinator makes reasonable in-scope decisions on the Orchestrator's
behalf and states each one**, rather than returning each to them.

**Make the call.** When no gate, CHARTER Hard Spec or irreversible act is involved,
state the decision as MADE and act on it in the same turn. Reserve genuine choices for
the Orchestrator, offering two options with the tradeoff named.

This does NOT license skipping a gate. Rule precedence tier 1 is unaffected.

Rationale: [bursts carry work](docs/standing/aigile-rules-rationale.md#bursts-carry-work).

### Delegation

Delegate by default. Never assess your own work yourself - that one is not a judgement
call.

**Every stream names its work class, and the class names the rung that runs it** - the
ladder in `global/work-ladder.yaml`: runner, builder, designer, architect. Running lower
is always allowed. Running higher needs an `Escalation:` reason specific to that stream,
shown on the burst page; that the work is important, consequential or high-blast-radius
is not one. The coordinator runs a stream inline only when its estimated new context is
under the measured startup threshold.

**Delegating means writing the contract and the red tests FIRST.**

**For full-stack tasks, propose splitting into independent streams before building.**
Each stream owns its file list and builds in its own git checkout on the same branch.

**Delegate to another model through the scripts, which work from any harness:**
`ag-review` (Reviewer), `ag-second-opinion` (Second-opinion), `ag-ship-review`
(Ship-review). Each resolves its seat from the frozen profile; never choose a family by
tool name. A harness's own subagent tool is a route only for a seat whose resolved
family is the host's own.

**During a burst the dispatch guard refuses an Agent dispatch that omits `model`, carries no
`AG_STREAM=` line, or names a model above its stream's rung without an `Escalation:` reason.**

**A delegate that stops and reports a problem has done its job.** VERIFY every reported
result rather than accept it, then act on what it changes.

**In a working tree shared by parallel agents, never let a shell command's target set
be implicit.** Stage and commit by explicit pathspec, never `git stash` in a shared
tree, and never a bare `git commit`. Run `git status --short` first and leave foreign
staged work alone; pre-edit evidence in a shared tree is `scripts/ag-preimage <path>`.
Full procedure: `docs/standing/parallel-stream-hygiene.md`.

Rationale: [executor routing](docs/standing/aigile-rules-rationale.md#executor-routing).

### Stage ownership and gate discipline

After every substantive stage, tell the Orchestrator four things:

1. What completed, and the evidence for it.
2. The exact next mandatory action.
3. Who owns that action.
4. Whether Orchestrator confirmation is required.

Start normal, safe, in-scope next actions without waiting.

**Every option in a decision put to the Orchestrator carries a NAME and a one-line
description of what it does.**

**When a published artefact carries the content, its link IS the delivery: the message
carries only what changed since the last message and the decision asked, and the content
goes in the message body only when no artefact exists for it.** Tool output is evidence
that the command ran, never delivery of the artefact.

Mandatory gates are never optional - state the gate, its evidence or failure, and
the compliant route forward. If none exists, request an explicit Orchestrator
exception and record it in `AIGILE_DECISIONS.md`.

**A `/goal` or `/loop` directive never overrides a gate.** Contract:
`docs/standing/goal-loop-contract.md`.

Rationale: [gate discipline](docs/standing/aigile-rules-rationale.md#gate-discipline).

### Report the result, not the process

Default response shape: what changed, then what needs a decision, then stop.

Parsimony applies to every artefact, not just replies: do the most with the least
ceremony. Volume is not rigour.

Detail earns its place only where the Orchestrator has a choice to make - then give
enough to decide with, including the option you would not pick. Everywhere else:

- A defect you already found and fixed does not get a paragraph. It gets a clause, or
  nothing.
- Do not restate a measurement the Orchestrator already has.
- No aphorisms, no summaries of your own reasoning, no narration of how carefully you
  worked.
- Commit messages state what changed and why, briefly. Not a case history.

Rationale: [report the result](docs/standing/aigile-rules-rationale.md#report-the-result).

### Start what the rules already start

When a rule says to do something without being asked, DO IT. Do not restate the rule and
then ask anyway.

**A second tell is a closing sentence that ANNOUNCES an action instead of taking it** -
written, then the turn ends and nothing runs. If a message would end claiming an action
is underway and no tool call for it appears in the SAME turn, the claim is false. Delete
the sentence, or make the call before ending.

This does NOT loosen a gate.

Rationale, and both tells with examples: [start what the rules already
start](docs/standing/aigile-rules-rationale.md#start-already-start).

### Tests come first, at the level the property lives

For new behaviour, write the failing test first.

Write it at the level the property lives, and derive what it asserts from its subject -
a literal copied beside the subject goes stale when the subject moves:

- **Outcome level**, where a contract spans components.
- **Unit level**, where the property is an invariant, an architecture constraint, or an
  internal contract with many consumers. These stand alone and need no outcome test
  above them.

Translating or refactoring existing tests is not red-first.

**A test reaches no git it does not need.** Anything a test can pass to the code as a
parameter, or supply from code - the root, a commit list, a file set, a branch name - it
passes. A git action in a test process is justified only when it is the property under
test, or is required AND cheaper than every other way to get the action done. Each thin
git reader is tested on a real repo for every output shape it hands on, and a full run
reports its git launches against an advisory budget (CD-142, CD-148): a prompt to look,
never a refusal.

Rationale, worked examples and why the outcome/unit split is not stricter:
[tests come first](docs/standing/aigile-rules-rationale.md#tests-first).

### A gate blocks only what cannot be undone, crosses a boundary or backs a claim

**A gate BLOCKS only when its outcome is unrecoverable, crosses a trust boundary or backs
a published claim** - the three kinds of control `### Trust the builder, or do not use one`
keeps. Such a gate fails CLOSED on every unhandled or edge state: block, with a structured
error, never open.

**Every other check WARNS.** It names the state and its remedy on stderr and lets the work
proceed. A warning is never silent and never reported as a pass: a gate's OK line names
each check that only warned. A writer refusing to write a malformed record is not a gate
and stays a refusal. Every gate exit is classed in `docs/standing/gate-register.md` (CD-149).

**The states are enumerated at plan time, not discovered round by round.** Any stream
parsing semi-structured or fallible input owes its plan a matrix, one row per input shape,
each mapped to BLOCK or WARN and a negative test.

**Enumerate an exempt set; never key an exemption, legacy path, skip or default on
`!= <strict value>`.**

Rationale, and the two matrices: [fail-closed matrices](docs/standing/aigile-rules-rationale.md#fail-closed-matrices).

### Narrowing a control derives its boundary from every consumption site

**Narrowing a control derives its new boundary from every consumption site.** Enumerate the
consumers by SEARCHING for them with `scripts/ag-search`, never by reading the function you
are editing. Write one test per input inside the new boundary, and one COMPLEMENT test per
input deliberately left OUT, stating why.

Rationale: [narrowing a control](docs/standing/aigile-rules-rationale.md#narrowing-a-control).

### Verify your work

Always self-verify: run tests, check the UI, check the API.

A completed build burst goes to `/ag-probe` next. STATUS Phase moves
`Producing -> Piloting` via probe only.

**UI work is verified in the browser, signed in, on real data, at desktop sizes only.**
- A login-gated UI has a local harness that brings it up signed in. A missing harness is
  a defect fixed in the burst, never a reason to review from code.
- A changed surface left unrendered fails the review.
- Every tab opened is closed.
- No phone or tablet widths unless asked.

Contract: `docs/standing/ui-verification-contract.md`.

**When something fails, the first action is to open the log, transcript or artefact -
never to propose a cause.** **When a review finding, a CI failure or a self-noticed
defect names one instance, run the sweep that lists every instance before fixing** and
state the count; fix all of them and add the sweep as a test. **Never kill a running
process that is not stalled**; stalled means evidenced, never a harness timeout reached,
and a process `pgrep` matches is live only if its start time and output say so. Never
`nohup ... &` inside an already-backgrounded wrapper.
**Do not hand-write a wait.** A backgrounded task notifies on completion, so the
notification IS the signal - end the turn; when the result is needed inside the turn,
use the Monitor tool, never a `tail -1 | grep` loop (the exit marker always occupies
the last line).

Rationale: [verify your work](docs/standing/aigile-rules-rationale.md#verify-your-work).

### Test verification economy

Never invoke a bare full-suite test command. Run `/ag-test`, or `scripts/ag-test-plan`
directly, with an explicit scope and rationale. The plan runs in one xdist call on this machine,
capped at half the cores; `shard_count` is for the CI matrix only.

This applies everywhere, including ad-hoc fix-and-verify work.

Rationale: [test economy](docs/standing/aigile-rules-rationale.md#test-economy).

### A control must earn its keep

**A control must mitigate more risk than it costs to maintain. If it does not,
delete it.** Not "review it", not "consider retiring it" - delete it, and withdraw
anything that claimed it was there. Detail: `docs/standing/control-registry-spec.md`.

**Apply the test continuously, not at a sweep.** That includes WHEN a control runs: one
that runs at a moment no boundary is crossed controls nothing.

**A DIRECTIVE is a control, and earns its keep the same way.** Each one must be
USEFUL - something changes if it is absent - and UNIQUE. Two rules a reader has to
hold apart are the defect a rule-surface ceiling exists to catch, so merge
near-duplicates by REWORDING them into one rule. Never by deleting a claim to clear
a number: the ceiling is a tripwire that says LOOK, and this test is what decides.

**Two things this does NOT license.** It is not a reason to remove a control that is
working - the evidence for deletion is cost exceeding risk mitigated, not distaste
for ceremony.

Rationale: [control earns its keep](docs/standing/aigile-rules-rationale.md#control-earns-its-keep).

### Review economy

**One independent review of the built diff. One independent plan round. That is all.**
`/ag-plan` dispatches ONE independent challenge, presents the scored findings, and
stops; **a second round happens on Orchestrator instruction and only that.**

Build stage: one adversarial pass on the built diff, by a family different from every
builder family since burst base. **The reviewer SCORES every finding** - severity,
likelihood, evidence, novelty - per the locked contract in
`docs/standing/codex-reviewer-constraints.md`.

**VERIFY every finding a review returns, and every complaint it surfaces, against the
code**, the same way `### Delegation` requires VERIFYing every delegate report. Do not
act on one, quote one to the Orchestrator, or pass one into a delegate's instructions
until you have opened what it names. Verify it against the agreed design and the
Orchestrator's rulings too: a finding that contradicts either goes to the Orchestrator,
never into a fix.

**SHIP reviews only the delta since the newest build review's `reviewed_head`** - or,
when a completed Ship-review round follows that review, since that round's head; the
burst base when neither exists - code and agent-executed instructions changed since
that head; a records-only delta dispatches
nothing; and a fix to a recorded finding is re-reviewed only at High or Critical.

Rationale: [review economy](docs/standing/aigile-rules-rationale.md#review-economy).

### Vendor names: seats in prose, families in config

Live prose names a SEAT - `Builder`, `Reviewer`, `Ship-review`, `Planner`,
`Second-opinion`. Only config names a model FAMILY, because prose that hardcodes a family
has made a routing decision the frozen profile owns, in a place nothing can update.

Rationale: [vendor names](docs/standing/aigile-rules-rationale.md#vendor-names).

### Bash output economy

Weekly token limits apply. Keep tool output small:

- Always bound commands that can produce many lines: `git log --oneline -20`,
  `find . -name "*.py" | head -40`.
- Never use `cat` on files - use the `Read` tool, with `offset`/`limit` where the file
  is large.
- Pipe test output through `tail -30` or `grep -E "PASS|FAIL|ERROR"` unless the
  full output is needed to diagnose a failure.
- If a command result was larger than ~50 lines and only part was needed, note
  that so it is not repeated.

### Demo-readiness goals enumerate the journeys

**For a demo-readiness goal, enumerate the journeys.** When a burst's goal is
"demo-ready" or "commercially credible", list the user-visible journeys a prospect walks
through and verify each is covered by a stream or explicitly deferred. A plan carrying only
component fix streams and no journey stream does not meet a demo-readiness goal, however
complete the fix list is. Guidance: nothing checks this, and a deficiency assessment is not
a scope definition.

### A shallowness challenge is answered with a new evidence pass

**A shallowness challenge is answered with a new evidence pass, not a rewrite.** When the
Orchestrator says an audit, assessment or report is shallow, go back to the files, screens
or data and inspect them again BEFORE touching the findings. Restating the examples the
Orchestrator supplied is not a re-audit: the corrected report must carry findings you
verified independently, the journeys they affect, and the evidence for each. Guidance:
nothing can check whether a second pass actually happened.

### Codex invocation

Codex is invoked through the CLI route, never the Skill-routed `/codex:<cmd>` and never
the Codex Claude-Code PLUGIN. AI-gile does not depend on the plugin, wherever it is
installed.

```
$HOME/.claude/aigile-canonical/scripts/ag-codex-dispatch [--seat <name>] [--model <id>] [prompt text...]
```

Treat its exit code as authoritative: non-zero means STOP and surface stderr verbatim
(20-25, the `AG_CODEX_EXIT_*` constants in `lib/ag_codex_dispatch.py`). It has no plugin-style
`rescue`/resumable-thread concept to carry forward - it is a plain prompt-in/text-out
dispatch through `codex exec --json` with usage capture, not an agentic task runner.

Rationale: [codex invocation](docs/standing/aigile-rules-rationale.md#codex-invocation).

### Canonical edits

`~/.claude/aigile-canonical` and the skill and agent copies under `~/.claude/` are never
edited from a client repo. They serve a canonical clone (`ag-setup --develop <clone>`), and a
change is made in that clone on a branch, never through the install path. A P33 developer
changes canonical through `/ag-contribute`: `start` a branch in a canonical clone, try it live,
`submit` a tested pull request against canonical `dev`. A collaborator on the methodology mirror
cannot open a pull request, because CI writes the mirror: `/ag-contribute report` files the
defect as a structured GitHub Issue on the mirror, with its body printed on stdout when it
cannot reach GitHub. Canonical maintainers edit their own clone (default
`~/dev/aigile-canonical`), never the install.

### AI-gile Document Layout

Canonical files live at repo root with the `AIGILE_` prefix: `AIGILE_CHARTER.md` (vision and
the constraints a burst can violate), `AIGILE_DESIGN.md` (how it is built), `AIGILE_PLAN/`,
`AIGILE_STATUS.md`, `AIGILE_HISTORY.md`, `AIGILE_DECISIONS.md`.

The `current/`-`closed/` lifecycle is `AIGILE_PLAN/README.md`. `ag-init` creates and
migrates them.

### AIGILE_PLAN/current/ is a STORE

Records are addressed by `(kind, stage, round)`, never by filename, from a CLOSED
vocabulary of kinds in `global/plan-store-schema.yaml`. **You do not choose a
filename** - `scripts/ag-plan-store address` computes one. Write-once through the store's own
API: `scripts/ag-plan-store supersede` is the only other write and RETAINS the prior record.
`iteration`, `feedback` and `decision` are mutable and take `scripts/ag-plan-store update`. Every record
carries an envelope; **there is no legacy record class** - an envelope-less record in
`current/` fails closed.

Contract: `docs/standing/plan-store-schema.md`.

Rationale: [plan store](docs/standing/aigile-rules-rationale.md#plan-store).

### CHARTER: the north star, and changes are instructed

Read `AIGILE_CHARTER.md` before planning. Hard Specs are non-negotiable. A blank
`AIGILE_CHARTER.md` has no Hard Specs to plan against; `/ag-adopt` seeds it.

For extracted content that changes frequently, read the peer artefacts instead:
`AIGILE_PLAN/CAPABILITIES.md`, `AIGILE_PLAN/BACKLOG.md` and `AIGILE_DECISIONS.md`.

`AIGILE_CHARTER.md` holds Vision, Hard Specs, Constraints, Objectives and Success
Criteria. Operational content belongs in `AIGILE_PLAN/`.

**A CHARTER change needs Orchestrator instruction or approval, and never needs
`/ag-charter`.** What stays forbidden is INFERRING a CHARTER change from findings or
judgement: surface it and ask.

**Approval for a DELETION carries the CHARTER edit that removes the deleted item, as one
decision.** It needs no second approval.

Rationale: [charter changes](docs/standing/aigile-rules-rationale.md#charter-changes).

### Context economy

Burst state lives in files - STATUS, HISTORY, CORRECTIONS, CHARTER, commits - not in
conversation.

**Post-burst `/clear`** needs all three: STATUS `**Phase:**` = `Idle`, `**Active burst:**` =
`None`, working tree clean. Suggest it after every burst without being asked.

**Mid-planning `/clear`** needs `Phase` = `Planning`, the plan store initialised, nothing
unsaved outside `AIGILE_PLAN/current/` and `AIGILE_STATUS.md`, and any plan record
validating. Resume with `/ag-plan`.

**Mid-burst `/clear`** needs a clean tree, `Phase` = `Producing`, and the current round's
`determination.md` written. Never `/clear` while `Probing` or `Piloting`.
`/compact` is the weaker alternative when the burst is not done.

**Never assert a context level as grounds for a decision - cite a checkable condition.**

Before running `/compact`, write the current task, position, next step and any open
decisions to `AIGILE_STATUS.md` under `## Session Checkpoint`, and read it back after
compacting.

Rationale, and the Session Checkpoint protocol:
[context economy](docs/standing/aigile-rules-rationale.md#context-economy).

### Compact instructions

When compacting, keep: STATUS `Phase`, the active burst and its burst base, the current
stream and its file list, the next mandatory action, and open Orchestrator decisions.
After a compaction `ag-status --source compact` re-injects the Session Checkpoint; read
it before continuing.

### Corrections

**If the fix is small and available now, MAKE IT.** `PENDING:` is for a disposition
that is genuinely open. File it through `ag-backlog add` and announce inline:
_"PENDING correction written: [rule summary]"_.

**One route.** File every correction as `--store deferred` with `kind: work-item`.
`Builder-family:` and `Builder-model:` are set at write time, never inferred:
`docs/standing/backlog-lifecycle-schema.md`.

**`PENDING:` marks an undecided RESOLUTION, not an unproven finding.** At the probe
retrospective the sweep covers the **WHOLE queue**, not only the entries this burst wrote.

Entry format: [corrections](docs/standing/aigile-rules-rationale.md#corrections).

Shape is refused at write by `ag-backlog add`; `ag-backlog lint` reports violations, and `hooks/pre-commit` warns on them.

### HAV redirect capture

**Capture is mechanical; classification happens at the retrospective.** The
`hooks/hav-capture` `UserPromptSubmit` hook records every Orchestrator turn of an active
burst verbatim to `AIGILE_PLAN/current/turns.jsonl`. Nothing is asked of you in the
moment.

**An empty buffer during an active burst is a defect in the capture mechanism**, never
evidence that no redirect occurred.

Classify FROM the buffer at `/ag-probe` retrospective Step 3 into
`AIGILE_PLAN/current/feedback.md`. Full taxonomy: `docs/standing/hav-classification-rubric.md`.

Rationale: [hav redirect capture](docs/standing/aigile-rules-rationale.md#hav-redirect-capture).

### Push to main

Never push to `main` without stating explicitly what you are about to do and waiting for
"confirmed" or "go ahead". Steer approval is not push approval - they are separate
confirmations.

Burst work never lands on `main` without an open PR: five layers enforce this
(`/ag-plan` Step 0, local `pre-commit`/`pre-push` hooks, `ag-status` warnings,
GitHub branch protection). **The local hooks and GitHub branch protection are separate
contracts. A true direct-to-main push needs BOTH overrides** (the two-contract override
model, `CD-026`).

Local override: `AIGILE_ALLOW_MAIN=1` (command-scoped, preferred) or an untracked,
burst-scoped `.aigile-allow-main` file (line 1 reason, line 2 burst-base SHA). **When
`multi_user_enabled` is ON the file needs a third line naming the operator, which must be
the current burst owner** - a 2-line file is refused under multi-user (`AIGILE_ALLOW_MAIN=1`
in your own shell stays owner-scoped and needs no line 3). Server override: a PR with a
`hotfix` label, or an admin push if `enforce_admins=false`. Full procedure:
`docs/standing/BRANCH-PROTECTION.md`.

### Size advisories are rewrite triggers, not limits

`SKILL_SIZE_ADVISORY` and its peers fire when a file crosses a round number. That is a
prompt to LOOK: extract a section, delete a stale one, or accept the size with a stated
reason.

**Do not trim prose to clear the number.** A file that drops under a threshold because
sentences were shortened has not improved; the trigger fired and nothing happened. No test
pins a named file under a size threshold, deliberately, so that reading is unavailable.

### Docs sync with build

A burst that changes a number, capability or behaviour a `docs/papers/current/` file
describes updates that paper in the same commit; the papers are read as current.
`tests/pytest/meta/test_no_stale_spend_figures.py` covers spend and economics figures only.

### AI-gile artefacts are never auto-formatted

**No formatter AI-gile installs runs in the commit path.** A formatter's reach is set
by a **path allowlist** (`lint-staged.config.mjs`), never by an extension pattern plus
an ignore file.

Rationale, why an allowlist rather than an ignore file, and the corruption incident:
[no auto-format](docs/standing/aigile-rules-rationale.md#no-auto-format).

### Failing tests are in scope by default

A pre-existing failing test may NOT be declared out of scope by a burst plan.

`/ag-plan` Step 1 surfaces the pre-existing red-test count and requires a disposition for
it. **Never inherit the figure from the plan.**

**Cite the previous burst's closing evidence; never open a burst with a full run.** Run
`scripts/ag-test-plan receipt-status --baseline`: `BASELINE` stands as the count and names
its scope and HEAD; `CURRENT_RED` lists failures that are in scope; `NO_BASELINE` means the
burst's own `/ag-probe` measures the delta; `BASELINE_INVALID` warns, naming the broken
record (CD-149). **A scoped green run never licenses a repo-wide
claim.** It is a baseline, not a release claim; ship's `--tests` leg cites the burst's own
full run plus the delta since it (CD-119), never a fresh full run just to ship.

An explicit skip needs a stated, tested precondition.

Rationale: [failing tests](docs/standing/aigile-rules-rationale.md#failing-tests).

The strict mode's receipt is bound by HEAD plus a digest over `git status --porcelain`
including untracked files; `--baseline` reports drift since then as a commit count instead
of refusing.

### A search is never scoped by accident

Use `scripts/ag-search`. It is global by default; a narrower scope is a named argument from
the registry in `docs/standing/search-scopes.md`, which makes the narrowing visible in the
invocation.

Any burst that retires, renames or deprecates anything runs it unscoped before declaring
scope, and attaches the evidence statement its output emits to that burst's adversarial
review.

Rationale: [search scope](docs/standing/aigile-rules-rationale.md#search-scope).

<!-- AIGILE:END -->


## Project

**Name:** ledger-wallet
**Repo:** github.com/Stephen-Kamau/ledger-wallet (private)
**Purpose:** Double-entry ledger/wallet (Go) - toy vehicle for exercising multi-user AI-gile.
**Owner:** Stephen Kamau, Prospect 33

## Stack

**Language:** Go 1.24 (backend); React + TypeScript (web)
**Framework:** net/http stdlib router (api); React functional components (web)
**Database:** SQLite (WAL) behind a storage-agnostic core port
**Package manager:** go modules (backend); bun (web)
**Deploy:** n/a (toy; no Railway)
**Test runner:** go test (backend); vitest (web)
**Formatter:** gofmt (backend); prettier (web)

## Key Commands

```bash
# Backend build / test
go build ./...
go test ./...

# Web (once scaffolded)
bun install
bun run dev
bun run test
```

## Operators

Multi-user AI-gile is ON. 5 signed operators build concurrently; see `docs/operators.md` and
`AIGILE_DESIGN.md`. Build bursts run in `operators/<login>/` clones with signed claims so the
overlap and review-independence gates fire on real activity.

Declare your browser for screen captures in your own CLAUDE.md (`~/.claude/CLAUDE.md`
or this file), one line; skills read it, default Playwright on Chromium:

    UI browser: opera-browser-cli | playwright | claude-in-chrome | built-in-browser

## AI-gile Operating Model

This repo operates under AI-gile.
The operating rules are the AI-gile Rules managed block. Repo-specific additions below only.

## Repo-Specific Rules

<!-- Rules genuinely specific to this repo only. -->
<!-- Do not duplicate the AI-gile Rules managed block here -->

<!-- Corrections from real mistakes live in AIGILE_PLAN/BACKLOG.md, as -->
<!-- `kind: work-item` entries, not here. File one with                -->
<!-- `scripts/ag-backlog add --store deferred`; see that store for     -->
<!-- past errors and their fixes.                                      -->
