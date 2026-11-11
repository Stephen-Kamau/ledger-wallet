<!-- AIGILE:START -->
## This repo is governed by AI_GILE

AI_GILE is the operating model this repo is built under. The skills, slash
commands, agents and git hooks it uses live in one canonical repo on each
developer's machine and are **not in this repo** - what you see here is the
governance record it produces: `AIGILE_CHARTER.md`, `AIGILE_PLAN/`,
`AIGILE_STATUS.md`, `AIGILE_HISTORY.md`, `AIGILE_DECISIONS.md`.

**Without canonical installed, nothing fires and nothing breaks:
commits work normally.**
The git hooks are machine-generated shims that hold absolute paths,
so they are untracked and never arrive with a clone; `core.hooksPath` is set in
`.git/config`, which is also never cloned; `.aigile/` is gitignored. A clone
without canonical is inert, not broken. You lose the tooling, not the ability to
work.

### Install the tooling

```tabs:os
### macOS
git clone git@github.com:p33-ai/aigile-canonical.git ~/.claude/aigile-canonical
export PATH="$HOME/.claude/aigile-canonical/scripts:$PATH"   # add to ~/.zshrc
ag-setup
cd <this repo> && ag-init

### Linux
git clone git@github.com:p33-ai/aigile-canonical.git ~/.claude/aigile-canonical
export PATH="$HOME/.claude/aigile-canonical/scripts:$PATH"   # add to ~/.bashrc (or ~/.zshrc)
ag-setup
cd <this repo> && ag-init

### Windows (WSL)
git clone git@github.com:p33-ai/aigile-canonical.git ~/.claude/aigile-canonical
export PATH="$HOME/.claude/aigile-canonical/scripts:$PATH"   # add to ~/.bashrc
ag-setup
cd <this repo> && ag-init
```

`ag-setup` runs once per machine; `ag-init` runs once per repo and is
idempotent.

### Remove AI_GILE from this repo entirely

Nothing here is load-bearing for the code, so removal is a delete plus one git
setting:

```bash
git config --unset core.hooksPath          # stop the hook shims running
rm -rf .husky .aigile                      # untracked machinery
rm -rf AIGILE_PLAN docs/papers
rm -f AIGILE_CHARTER.md AIGILE_STATUS.md AIGILE_HISTORY.md \
      AIGILE_DECISIONS.md AIGILE_DESIGN.md
```

Then delete the `AIGILE:START` / `AIGILE:END` block from `CLAUDE.md`, and
delete this file, `AIGILE_README.md`. Your own `README.md` was never touched.

### Learn it

The wiki ships with canonical: open
`~/.claude/aigile-canonical/docs/wiki/index.html` in a browser.
<!-- AIGILE:END -->
