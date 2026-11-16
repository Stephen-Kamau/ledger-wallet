<!-- AIGILE-RECORD
kind: determination
stage: build
round: 1
schema_version: 1.1.0
superseded_by: none
-->

# Determination - round 1, burst collision-v2 (same-file collision test)
Goal: run a REAL two-user same-file collision, resolve keeping BOTH edits, done+tested.
Outcome: PASS.
- Overlap gate (advisory) flagged amara∩kofi on [core/**, api/**, internal/sqlite/**] at claim time.
- Git (hard) rejected kofi's second push (non-fast-forward) - clobber prevented.
- Real merge conflict in 7 files; resolved keeping BOTH features (holds + currencies), incl a
  semantic interleave (two handler bodies) reconstructed. merge authored by Kofi.
- e2e found a drift defect (currencies 500 live); fixed in-burst by Amara (migrations = single
  source) + /available endpoint + regression test.
- Full suite + e2e green. Features: PlaceHold/ReleaseHold/AvailableBalance, GET /currencies.
Authorship: feature/holds-v2 (Amara), feature/currencies-v2 (Kofi incl merge). dev pushed.
Disposition: burst complete. Remaining deferred: C-3 UNIQUE->replay backstop (unchanged).
