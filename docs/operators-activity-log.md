# Operators activity log

Running, timestamped record of the multi-user build. Purpose: after the fact, analyse **what
happened, when, by which operator, and whether we hit conflict**. The coordinator (driver)
appends here as the burst runs. Times are UTC.

Columns: **Time (UTC)** | **Operator** | **Action** | **AI-gile / git command** | **Outcome / notes**

## Conventions
- "Operator" is the signed identity acting (git `user.email` local-part = login; claims signed
  with that login's keystore key).
- Conflict rows are marked **⚠ CONFLICT**.
- Decisions carry a `CD-` / condition tag.

## Burst: build-v1

| Time (UTC) | Operator | Action | Command | Outcome / notes |
|---|---|---|---|---|
| (setup) | coordinator | audit log created | (Write) | this file |
