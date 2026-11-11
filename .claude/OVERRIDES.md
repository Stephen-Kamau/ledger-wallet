# Overrides

Skills and agents for this repo come from the global AI-gile
canonical installation (~/.claude/ managed by ag-setup).

## To override a skill for this repo only

1. Create the override directory and copy the canonical SKILL.md:

   mkdir -p .claude/skills/ag-plan
   cp ~/.claude/skills/ag-plan/SKILL.md .claude/skills/ag-plan/SKILL.md

2. Edit the local copy
3. Commit it (team members get the override via git)

The local copy shadows the global version for this repo only.
Claude Code loads `.claude/skills/<name>/SKILL.md` in preference to
`~/.claude/skills/<name>/SKILL.md` when both exist.

## To promote an override back to canonical

Use `/ag-contribute` - `submit` opens a tested pull request from a canonical clone,
and `report` files a structured GitHub Issue (or prints its body on stdout
when it cannot reach GitHub), so the improvement flows through governance rather
than a direct edit. Direct
`cp` to `~/.claude/aigile-canonical/` bypasses governance and edits the served
clone's working tree, not a branch; use the skill instead.

## To see what is available globally

    ls ~/.claude/skills/
    ls ~/.claude/agents/
