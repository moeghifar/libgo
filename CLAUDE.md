# CLAUDE.md

## Read this first, every session

**Before doing anything in this repository — answering, exploring, planning, or editing — you MUST read `AGENTS.md` in the repo root.**

`AGENTS.md` is the single source of truth for how work is done here. It points onward to
`.specify/agents.md`, which is the entry point for the active specification workspace.

Required order, no exceptions:

1. Read `AGENTS.md` (repo root).
2. Read `.specify/agents.md` (the spec entry point it names).
3. Follow the entry point to whichever spec / research / plan documents apply to the task.

Do not propose a design, write Go code, or edit files in `pkg/` until steps 1–3 are done.

## Non-negotiables

- `.specify/**` is **local only** — it is gitignored and must never be committed or pushed.
- Never remove `.specify/` from `.gitignore`.
- Never use `git add -f` on anything under `.specify/`.
- If `AGENTS.md` and this file ever disagree, `AGENTS.md` wins on process; this file only
  guarantees that `AGENTS.md` gets read.
