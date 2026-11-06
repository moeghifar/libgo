# AGENTS.md

Working agreement for any agent or human contributing to **libgo**.

## 1. Entry point

**The specification workspace lives at `.specify/`. Start by reading `.specify/agents.md`.**

That file is the index: it names the active spec, the research notes, and the current
implementation plan. Everything below is the standing context that does not change per task.

```
AGENTS.md  ──▶  .specify/agents.md  ──▶  .specify/research/*.md
                                    └──▶  .specify/spec/*.md   (when a spec is written)
```

`.specify/` is gitignored on purpose. It is a private working area — never commit it.

## 2. What libgo is

A collection of small, independent Go packages under `pkg/`. Each package is a *toolset*,
not a framework: importable on its own, no cross-package coupling, no init-time magic.

Shipped today:

| Package      | Purpose                                   |
| ------------ | ----------------------------------------- |
| `pkg/envy`   | environment variable loader               |
| `pkg/climd`  | lightweight CLI builder (Cobra alternative) |
| `pkg/xttp` | Fiber v3 routes + generated OpenAPI 3.1 (no handler wrapping) |

In design: `pkg/httpd` — a lightweight, typed HTTP API builder in the spirit of
[Huma](https://github.com/danielgtaylor/huma), but materially smaller and faster.
See `.specify/research/` for the plan.

## 3. Design rules (apply to every package)

1. **Standard library first.** A third-party dependency must be justified in writing in the
   spec before it is added. `go.mod` currently has exactly one direct dependency; keep that
   number as close to zero as the problem allows.
2. **Lightweight is the product.** Binary size, allocation count, and cold-start cost are
   features, not afterthoughts. If a feature cannot pay for its weight, it does not ship.
3. **Reflection at startup, never in the hot path.** Registration time may reflect; request
   time must run precomputed closures.
4. **No global state.** No package-level mutable vars, no `init()` side effects.
5. **Explicit structs over builder chains.** Follow the `climd.AppConfig` shape: a plain
   struct literal describes the whole app.
6. **Errors are values.** Return them; never `panic` in library code outside of programmer
   error detected at registration time.
7. **Every exported symbol is documented.** Package comment on the top of the primary file.
8. **Every package ships a `README.md`** in its own directory, in the style of
   `pkg/climd/README.md`.
9. **Tests live beside the code** (`*_test.go`), stdlib `testing` only, table-driven.

## 4. Workflow

1. Read `.specify/agents.md` and the documents it points to.
2. Research and plan **before** writing code; the plan goes in `.specify/`, not in the repo.
3. Implement in small, reviewable steps. Tests alongside.
4. Verify: `go build ./... && go vet ./... && go test ./...`. Report real output; never
   claim green without running it.
5. Update the package `README.md` and the root `README.md` feature list.

## 5. Commit hygiene

- Conventional-commit prefixes, matching history: `feat:`, `fix:`, `docs:`, `refactor:`.
- Never stage `.specify/`, `.env`, or build artifacts.
- Do not commit or push unless explicitly asked.
