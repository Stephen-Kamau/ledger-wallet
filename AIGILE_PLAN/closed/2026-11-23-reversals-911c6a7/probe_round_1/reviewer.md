<!-- AIGILE-RECORD
kind: reviewer
stage: probe
round: 1
schema_version: 1.1.0
superseded_by: none
-->

## Design contract: P33 base only
No repo-level `design-contract.md` and no `*/design/mockups/README.md` exist in this repo, so
`$HOME/.claude/aigile-canonical/global/design/p33-design-contract.md` is the only contract
applied. No agreed `/ag-design` artefact covers this surface.

Design lenses: ALL SKIPPED - none installed in this session (`frontend-design` is listed but
`design:design-critique`, `design:ux-copy`, `design:accessibility-review`,
`design:user-research`, `design:design-system` are not). Stated rather than silently merged.

## Dev server: `VITE_API_TARGET=http://localhost:8090 bun run dev` in `web/`, against a real
`cmd/ledgerd` on a scratch SQLite file seeded by `web/harness/seed.mjs`. **The
`web/harness/reverse-stub.mjs` was NOT started** - port 8081 refused throughout - so every
reversal below was served by dakari's real `POST /entries/{id}/reverse`. No declared
`UI browser:` line, so Playwright on Chromium (the default).

## Surfaces seen
Verdict: PASS (0 changed surfaces unrendered)

Changed surfaces in the burst delta (`e62af5f^..HEAD`, excluding tests):
`web/src/views/StatementPane.tsx`, `reversalIndex.ts`, `reversalErrors.ts`,
`reversalIntent.ts`, `web/harness/reverse-stub.mjs` (harness, not a surface).

### / (statement pane) at 1440x900 and 1280x800
| Check | Result | Evidence |
|-------|--------|----------|
| Rendered signed in, real data | PASS | `sessionStorage.ledger.token=alice`; real ledgerd + seeded SQLite; 8 screenshots `ui/{1440,1280}-{1..4}-*.png` |
| Every level it serves | PASS | 4 levels each width: balances-only (empty statement), statement loaded, inline confirm, post-reversal both-ends labelled |
| Desktop widths only | PASS | 1440x900 and 1280x800; no phone width exercised |
| Console errors | PASS | `console_errors: []` at both widths |
| Loading / error / empty / success states | PASS | empty "Select an account to see its statement."; success notice "Reversed. The correction has been posted to this statement as a new entry."; error path exercised separately this session (403/409 banners) |
| Keyboard focus visible | PASS | first Tab lands BUTTON with `outline-style: auto`, width 1px |
| Reversal link survives reload | PASS | separate run: after reload reversed=1 reversal=1 captions=2; pre-fix 0/0/0 (falsified) |
| Tabs closed | PASS | every context closed in-script; both ledgerd and vite stopped at end |

## Design intent (Step 3 plan)
- **Colour:** the nine `:root` tokens are the whole palette. Accent hierarchy: `--accent`
  for the live action and for the row that IS a correction; `--muted` for the row that has
  BEEN corrected. That is the right way round - the correction is the thing to look at, the
  corrected row is history.
- **Type:** mono for ids/amounts/seq (`.mono`), sans for prose and captions. Badge at
  0.65rem and caption at 0.75rem: small, but both clear the 4.5:1 floor on `--panel`.
- **Layout:** three panes (balances / statement / transfer), statement centre. A correction
  renders as its OWN row plus a caption row, never merged into the original - append-only
  made visible.
- **Signature:** the struck-through amount on a reversed row beside an intact "Reversal"
  row. That pair is the one thing a reader should remember: both entries exist.
- **Motion:** none, and none is wanted here. A ledger correction is not an animation.

## Mockup parity
No accepted mockups. `/ag-design` has never run for this repo; there is no
`*/design/mockups/` tree and no `design-approval` record, so there is nothing to bind with
`ag-plan-store design-check` and no inventory to check ids against.

## Implementation gaps
| Area | Finding | Severity | Recommended fix |
|------|---------|---------|----------------|
| Contrast floor | `button { color: #fff }` on `background: var(--accent) #4493f8` = **3.10:1**, below the 4.5:1 floor for 0.9rem text. Affects "Send transfer" and "Refresh", both rendered on this surface. **PRE-EXISTING** - `styles.css:78`, introduced by `3b329bf` (the original web integration); this burst never touched `styles.css`. Reported at blocking severity per the contract's "report a single failing pair at blocking severity, not as an observation". | High | Darken the button background (`#1f6feb` gives 4.6:1 with `#fff`) or use `--text` on a `--panel` button. One token change; not in this stream's scope. |
| Token compliance | `#fff` at `styles.css:78` is the only hard-coded hex outside `:root` - the other 9 all define tokens. | Low | Promote it to a `--on-accent` token, which is also where the contrast fix belongs. |
| Reduced motion | No `prefers-reduced-motion` block. Not a defect here: the stylesheet declares zero `transition` and zero `animation`, so there is no motion to gate. Recorded so a future animation does not inherit a silent gap. | Low | Add the guard with the first animation, not before. |

## Confirmed correct
- **Both ends of the link are labelled**, which is the actual requirement: the original keeps
  its amount (struck, `--muted`, "Reversed") and the correction is badged "Reversal" in
  `--accent`, each with a caption naming the posting on the other end. A correction never
  reads as an unexplained second transfer.
- **Append-only is visible, not just true**: the reversed row is dimmed, never removed, and
  the statement does not net the pair out.
- **Contrast on every pair this burst renders passes computed** (not eyeballed): text/bg
  15.67, text/panel 13.86, muted/panel 5.56, accent/panel 5.28, credit/panel 6.45,
  debit/panel 4.88, flag/panel 6.49; both badges and the caption 5.28-5.56. `ag-contrast
  --if-touched` returned `skipped` (the diff touches no contrast input).
- **Token rule honoured**: `--muted #8b98a5` is lighter than the `#7d8b9c` floor, so no text
  token is darker than the limit on backgrounds darker than `#161b22`.
- **Badge border** is `1px solid currentColor`, so it inherits the text colour and clears 3:1
  wherever the label does.
- **`Reverse` is correctly withheld on a reversed row and OFFERED on a correction** - core
  permits reversing a reversal, and the UI no longer guesses otherwise.
- **Copy is specific and active**: "Reverse this entry?", "This entry reverses entry <id>. It
  is the correction, not a new transfer." Names the other entry rather than gesturing at it.

## Next build action
Fix `styles.css:78` - the only failing contrast pair in the app, and the only hard-coded hex.
One line, fixes both findings in this review, and it is the sole blocking item.
