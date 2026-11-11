# Operators

The signed developers who build **ledger-wallet** concurrently. They are the point of the
exercise: **5 developers** working the same repo at the same time, each with a **distinct
signed identity**, so multi-user AI-gile's ownership, overlap, and review-independence
machinery is actually exercised on real activity.

**The model.** After setup (repo scaffolded, signed roster written, `multi_user_enabled`
ON), each operator **clones the repo** into `operators/<login>/` and works their **assigned**
stream. An operator may also reach for work **not assigned to them** — this is deliberate and
the most important case: when a second operator touches files an assigned owner already
holds, the **claim-overlap gate and the ownership clock must fire**.

Names are Swahili, chosen to fit each role.

## Roster

| | Operator | Meaning | Role | Rung | Owns (declared globs) | Depends on | Identity |
|---|---|---|---|---|---|---|---|
| A | **Amara** Okeyo | grace | Ledger architect — owns the double-entry invariants and the contract | architect | `core/**` | — (chain head) | `amara-1` |
| B | **Dakari** Mwangi | joy | Transfer-API builder, auth boundary | builder | `api/**` | Amara's core contract | `dakari-1` |
| C | **Nia** Achieng | purpose | Statements / balances UI | designer | `web/**` | Dakari's API shape | `nia-1` |
| D | **Kofi** Otieno | born on Friday | Migrations + cross-stream QA | builder | `migrations/**`, `tests/**` | shares the schema contract with Amara → **overlap trigger** | `kofi-1` |
| E | **Zola** Juma | quiet / calm | Reviewer + unassigned-work probe | runner | *(no assigned stream)* | arbitrated by the gates | `zola-1` |

All five are enrolled in the signed roster (`authority/trust-roots.jsonl`, root `root-dev`)
and sign the quorum `multi_user_enabled` flag. Keys live in the local, gitignored keystore
(`.aigile/keystore/<login>.json`); the root private key (`.aigile/root-dev.key`) is
gitignored and held by the authority (the Orchestrator). Only public material is committed.

## The dependency chain this is built to exercise

`core` (Amara) → `api` (Dakari) → `web` (Nia). Each stream consumes the contract the stage
above it froze — a **real cross-stream dependency chain**, not a single shared file. On top
of that:

- **Overlap.** Amara's `core` and Kofi's `migrations` both touch the schema contract. Where
  their declared file sets intersect, the **claim-overlap gate** and the **ownership clock**
  arbitrate who holds which files.
- **Review-independence.** Zola holds no build stream, switches to the **reviewer** seat, and
  reviews a stream it did not build. Review-independence requires the reviewer's signed
  identity to differ from every builder since burst base, and that it holds no live claim
  intersecting the burst under review — exactly what Zola exercises.
- **Auth boundary.** A transfer debiting an account must be authorised by that account's
  owner (R7) — a real authorization check, not just disjoint file ownership.

## Signed activity

Claim events (declared globs per stream), governance verdicts (reviewer identity), and
presence are attributed to these identities in the event ledger. The human-readable activity
trail is the projection of that ledger.
