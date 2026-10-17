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

**Scopes:** `graph`, `weights`, `engine`, `sim`, `fleet`, `store`, `auth`, `api`, `web`, `scripts`, `deploy`.

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
  - Reading them for ideas is fine. Write the implementation fresh.
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

## 6. When to push

- Push after every few commits, and always at the end of a session.
- Use a short-lived branch for anything experimental (`feat/<name>`), then merge into `main` with a normal merge. Do not squash away the small commits.

---

## 7. If you are unsure

Stop and ask the human instead of guessing, especially about:
- licensing or attribution
- anything that touches `sumo-routing/` or `fleetbase/`
- deleting files or rewriting history
