---
name: go-style-precommit
description: "Use for 提交前整理 and before every commit in this repository. Tidy the changed Go code and tests to Google Go Style, keep the change behavior-preserving, and run the repository's checks. Trigger on pre-commit cleanup, diff tidy, or staged changes."
---

# Go style before commit

Tidy only the change about to be committed. Rename, comment, and reshape
the diff. Preserve behavior, output, and persisted state. When the tidy
uncovers a defect or a missing feature, stop and split it into its own
commit with the matching bug-fix or feature workflow.

The reference is https://google.github.io/styleguide/go/.

## Scope

Tidy the Go files the change touches, staged and unstaged. Do not reformat
untouched files.

```bash
git status --short
git diff --name-only --diff-filter=ACM -- '*.go'
git diff --cached --name-only --diff-filter=ACM -- '*.go'
git ls-files --others --exclude-standard -- '*.go'
```

Read the whole diff before editing.

## Code

- `gofmt` the changed files. `gofmt -l` must print nothing.
- Give every exported name a doc comment that starts with the name and
  ends with a period. An unexported comment explains a non-obvious why.
- Spell initialisms as `ID`, `URL`, `HTTP`, `JSON`, and `API`.
- Give a helper a specific name when it would collide with a method. For
  example, `CurrentBranchURL` sits beside `Host.BranchURL`.
- Remove one-caller wrappers, dead exported helpers, and boolean
  parameters that select two behaviors. Use two functions or a typed
  option.
- Make invalid states unrepresentable. The zero value is the only none,
  and the constructor sets every field of a valid value.
- Start an error string lowercase, leave off trailing punctuation, and
  name the action and the object.
- Group imports as standard library, this module, then third party. No dot
  imports.
- A comment states a non-obvious why. Delete phase narration and lines
  that restate the code.
- For `ui` lifetimes, handles, or worker captures, follow
  `.agents/skills/mygo-maintenance/SKILL.md`.

## Tests

- Name a test for the behavior it protects.
- Use `t.Run` and table tests. Put the input in the failure message.
- Assert what a caller observes. Do not assert private fields.
- Use `t.Helper()` in helpers and `t.Cleanup()` for state. Do not sleep to
  synchronize.
- Cover the new branch and the boundary that can regress. Delete a test
  that only restates the implementation.

## Checks

```bash
gofmt -l .
go tool mygo vet .
CGO_ENABLED=0 go test -count=1 ./...
git diff --check
```

Run the narrow package first when the suite is slow, then the whole suite
before committing. An app change also follows the build and install step
in `AGENTS.md`.

## Commit

Read `git diff --cached` after staging. Amend the just-made unpushed
commit when the tidy belongs to it. Otherwise make a separate commit with
the repository's sentence-case imperative subject.

Report the renames, the removed code, the checks that ran, and any
behavior left alone.
