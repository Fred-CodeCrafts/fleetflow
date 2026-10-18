# AGENTS.md

Instructions for AI coding agents (Claude Code, Copilot, Cursor, etc.) working in this repository.

**Project:** FleetFlow, an open-source fleet navigation platform with dynamic weights.
**Context:** Road To DevFest Jakarta 2026 submission. The git history is part of the evidence that this project was built from scratch after 11 October 2026, so it must be clean, frequent and honest.

---

## 1. Core rule: commit in small steps, always

- Commit after **every single logical change**. Do not batch work.
- One commit = one idea. If the message needs the word "and", split it into two commits.
- Never leave working changes uncommitted at the end of a task.
- Target size: **under ~100 changed lines** and **1 to 3 files** per commit. Larger is allowed only for generated files, lockfiles, or an initial scaffold, and the message must say so.
- Every commit must leave the repo in a **working state**: it builds, and existing tests pass.
- Commit **before** starting a risky refactor, so there is always a clean point to return to.

### Workflow for each task

1. Check state: `git status` and `git diff`.
2. Make one small change.
3. Run the build and tests for what you touched.
4. Stage only the relevant files: `git add <paths>`. Never use `git add .` or `git add -A` blindly.
5. Commit with a good message (see section 2).
6. Repeat from step 2.

If a task is large, write a short plan first, then commit one step of the plan at a time.

---

## 2. Commit message format

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <short summary in imperative mood>

<optional body: why, not what>
```

**Types:** `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `perf`, `build`, `ci`.

**Scopes:** `graph`, `weights`, `engine`, `sim`, `fleet`, `store`, `auth`, `api`, `web`, `config`, `domain`, `arch`, `scripts`, `deploy`.

**Rules:**
- Summary line: 72 characters or fewer, lowercase after the colon, no trailing period.
- Imperative mood: "add decay to edge state", not "added" or "adds".
- Explain **why** in the body when the reason is not obvious.
- No vague messages such as "update", "fix stuff", "wip", or "changes".

**Examples:**

```
feat(weights): add per-edge exponential moving average
test(weights): cover decay of edges with no recent report
fix(router): load metric pointer once per query
docs: add setup instructions to README
chore: add Apache-2.0 license
```

---

## 3. Suggested order of work (small-commit friendly)

Break features down like this instead of one large commit:

1. Scaffold: `chore: initialize go module`
2. Data types: `feat(weights): add edge state struct`
3. One function at a time, each with its test
4. API handler skeleton, then each endpoint separately
5. Wire components together
6. Docs and cleanup

Tests go in the same commit as the code they cover, or in the commit immediately before or after it.

---

## 4. Project rules (do not break)

- **Originality:** all code in this repo must be written new for this project.
  - Do **not** copy files or code from `../sumo-routing/` (thesis prototype) or `../fleetbase/`.
  - Do **not** paste thesis text into the repo.
  - **Studying a reference implementation is allowed and encouraged.** Read
    `../fleetbase/` and `../sumo-routing/` to learn their information architecture,
    page layout, interaction patterns, naming and feature set, then write this
    repo's version yourself. Matching a screen's structure and behaviour is fine;
    copying its source, templates, styles or assets is not.
  - **Why the line sits there:** Fleetbase is AGPL-3.0 and this repo is Apache-2.0,
    so copied front-end code would attach AGPL obligations to the whole console
    and break the license. Reproducing a layout from observation is not a
    derivative work; pasting a component is.
  - **No attribution is owed for studying a reference.** AGPL-3.0's attribution and
    notice duties (sections 4, 5 and 13) attach only when you convey the Program
    or a work based on it. Learning a layout from `../fleetbase/` and writing your
    own implementation conveys nothing, so do **not** add a credit line, a `NOTICE`
    entry or a commit-body acknowledgement for it. Credit is owed for third-party
    code and data you actually ship, not for reference material you read.
- **Third-party code and data:** only add dependencies with licenses compatible with this repo's license. Record attribution in `README.md` or `NOTICE` (for example OpenStreetMap data, ODbL).
- **No history rewriting:** do not use `git rebase`, `git commit --amend` on pushed commits, `git push --force`, or `git filter-branch`. Do not backdate commits or change author dates. The history must be an honest record. If you made a mistake, fix it in a new commit.
- **No secrets:** never commit API keys, tokens, credentials, or `.env` files. Use environment variables and keep `.env.example` with placeholders.
- **Deployment:** the final system must run on Cloud Run. Keep the Dockerfile working.
- **Concurrency:** route queries must never block on weight updates (immutable metric snapshot swapped atomically). Run `go test -race ./...` before committing concurrency changes.

---

## 5. Before every commit: checklist

- [ ] The change is one logical unit.
- [ ] `go build ./...` passes.
- [ ] `go test ./...` passes (and `-race` if concurrency was touched).
- [ ] Code is formatted (`gofmt`) and `go vet ./...` is clean.
- [ ] Only the intended files are staged.
- [ ] No secrets, no large binaries, no copied code from other folders.
- [ ] The commit message follows section 2.

**Running the race detector on this machine.** Windows has no C compiler, so
`go test -race` fails there with `cgo: C compiler "gcc" not found`. WSL
(`kali-linux`) has gcc/clang and Go 1.25.5 in `~/tools/go`; run the suite from
PowerShell with:

```powershell
wsl.exe bash -lc "cd '/mnt/c/Users/frede/documents/fleet project/Fleetflow'; ~/tools/go/bin/go test -race -count=1 ./..."
```

Do not tick a task whose "done when" requires `-race` until this command has
actually been run and passed.

---

## 6. Branches and pushing

Three levels, and a change lands on the lowest one it belongs to and moves up only
when it is meant to:

- **`main` — stable.** The released version: what is deployed and what a judge
  clones. Only a release lands here, and always as a merge from `dev`, tagged
  (`v0.1.0`, T67). Work does not happen on `main`.
- **`dev` — development.** The integration branch. Every change reaches `dev` first,
  and a release is `dev` merged into `main` with a normal merge.
- **Feature branches — named after the change, like the commit types.** Branch from
  `dev` and name it for the kind of work: `feat/dynamic-weights`,
  `fix/decay-underflow`, `refactor/route-seam`, `docs/branch-model`, `test/...`,
  `chore/...`. A feature branch is short-lived — one piece of work — and is merged
  back into `dev` with a normal merge, then deleted. Do not squash away the small
  commits: the history is evidence (section 1).

Parallel workers each get their own worktree and feature branch, and their merges
into `dev` are serialized, one at a time (section 8).

**Pushing**

- Push after every few commits, and always at the end of a session: `git push origin dev`.
- `git push origin --all` pushes every branch; `git push origin --tags` pushes tags.
- Never force-push, and never rebase or amend a pushed branch (section 4). A mistake
  is fixed in a new commit; published history is not edited.

---

## 7. If you are unsure

Stop and ask the human instead of guessing, especially about:
- licensing or attribution
- anything that touches `sumo-routing/` or `fleetbase/`
- deleting files or rewriting history

---

## Database
This project uses Postgres. The dev database is local, so the database MCP
(`toolbox-postgres`) is **enabled** in the project `opencode.json` and reachable
only through the "db" subagent.
- Development and production are **separate databases with separate
  credentials**. That separation, not a reduced privilege set, is what makes the
  broad dev access safe.
- The dev entry pins `POSTGRES_HOST` to `127.0.0.1` and authenticates as the
  local dev role, so the agent may read **and write** (DDL and DML) there while
  developing.
- Production is a different database reached only through the deploy
  environment's own `DATABASE_URL`. Production credentials never belong in this
  workspace; never connect to a non-loopback host.
- For live data needs (schema inspection, row counts, checking real values), delegate to the db subagent. If it's unavailable or disabled, tell me: "This needs the DB MCP. Please enable it."
- Don't guess schema or data. Use migration files and ORM models in the repo for anything that doesn't need live data.
- For code-only work, never use or ask for the MCP.
- Before querying, say you're about to use the MCP and why.
- Schema changes go through the project's migration tooling so they stay
  reproducible; the MCP is for inspection and dev iteration, not the source of
  truth.
- The migration tooling is **Flyway** and the schema lives in `db/migration/` as
  `V<version>__<description>.sql`. Applied migrations are immutable: fix a
  mistake with the next migration, never by editing an old one or by altering a
  running database. To apply them:
  `docker compose --profile tools run --rm migrate`. See `db/README.md`.
- Keep it off outside development: in any other project set `"disabled": true` and restart. Never claim it's off until a new session confirms it.
  - OpenCode V2 uses `"disabled"`, not `"enabled"`. The global config keeps `"disabled": true`; only the project config is toggled for on/off.

---

## 8. Agent orchestration

The policy itself lives in the global `~/.config/opencode/AGENTS.md`
("Agent Orchestration and Efficiency") and applies here unchanged. These are the local
consequences of it.

- **Parallel workers get their own worktree and branch.** A branch can be checked out in
  only one worktree, and merges into a shared branch must be serialized: merge workers
  one at a time. `dev` is the integration branch every change lands on first, and
  `main` is the stable, released branch (section 6).
- **The history is evidence (section 1), so keep scaffolding out of it.** A scratch
  ledger or experiment artifact is deleted in its own commit, never left behind.
- **Verification debt, honestly recorded.** `internal/driver` and `internal/trip` were
  accepted on their own tests plus `internal/arch`, without a line-by-line review. That
  debt is settled when `internal/store` and the API layer make them reachable; until
  then, do not describe them as reviewed.

### Trial evidence (2026-10-17)

The first branch-merge trial produced T39's driver and trip service slices: two workers,
two worktrees, and one deliberate merge conflict that the worker resolved itself.

| measure | tokens |
|---|---|
| workers, total | 708,044 |
| of which: merge + conflict resolution | 55,939 |
| saving over the lead reading the diff | ~2,600 |
| lead session, cumulative | 59,714,220 |

The conflict cost the worker 55,939 tokens in a context that was then discarded; the lead
read 45 words. The merge-versus-review choice was ~0.004% of the lead's session, so the
real levers are delegating at all and keeping the lead session short — not merge versus
review.
