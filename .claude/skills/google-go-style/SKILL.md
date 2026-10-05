---
name: google-go-style
description: Google Go Style (Style Guide, Style Decisions, Best Practices) as this project's top-priority Go standard. Use whenever writing, generating, designing, refactoring, reviewing or validating Go code, Go packages, Go APIs or Go tests in this repository - load it before writing the first line of Go and before applying any other Go skill (including samber cc-skills-golang), whose advice it overrides on conflict.
argument-hint: "[validate <path> | validate diff | (empty = guidance for the task at hand)]"
---

# Google Go Style for this codebase

Request: `$ARGUMENTS`

This skill is the house standard for Go here. It condenses three documents, saved in full at
`docs/sources/google-go-style/` (`guide.md`, `decisions.md`, `best-practices.md`). When a rule
below is unclear or a case is not covered, read the original section there; the tags on each
rule give the document and anchor (for example `D#getters` is
`https://google.github.io/styleguide/go/decisions#getters`).

## Precedence

When two sources disagree, the higher one wins:

1. **G - Style Guide** (canonical and normative): principles and core rules.
2. **D - Style Decisions** (normative): specific rulings. Subordinate to the Guide.
3. **B - Best Practices** (advisory): recommended patterns. Follow unless there is a reason not to.
4. **samber `cc-skills-golang`** and any other Go skill: use for everything the three documents
   leave open (libraries, tooling, deeper topics). Where its advice conflicts with G, D or B,
   follow G/D/B and say in one line that you deviated and why.
5. Effective Go is the assumed baseline underneath all of it.

Conflicts to expect with third-party Go advice, all settled in Google's favour here: assertion
libraries and test frameworks (not allowed; `testing` plus `go-cmp` only), error-handling
helper frameworks, generic utility libraries where a loop or map does the job, interfaces
defined up front "for testability", functional options as a default, `Get`-prefixed accessors,
recovering panics in servers.

## The five principles, in order (G#principles)

Use these to decide anything the rules do not settle. Earlier ones win.

1. **Clarity** - purpose and rationale are clear to the reader. Judged by the reader, not the
   author. Explain *why* in comments; let names carry the *what*.
2. **Simplicity** - the simplest way that meets the goal. Reads top to bottom, no needless
   abstraction, no cleverness. *Least mechanism*: language construct first (slice, map, loop,
   struct, channel), then the standard library, then a dependency already in this repo, and
   only then a new dependency or new machinery.
3. **Concision** - high signal to noise. Use the common idioms so anything unusual stands out.
4. **Maintainability** - easy to modify correctly; APIs that can grow; few dependencies; no
   critical detail hidden where it is easy to miss; tests with actionable failures.
5. **Consistency** - looks like the code around it. A tie-breaker only; it never overrides 1-4.

## When writing or generating Go

1. Read the reference for what you are about to produce (table below). For a new package or
   exported API, read `references/api-design.md` first.
2. Decide before coding: package name and boundary, the exported surface (as small as works),
   the error contract (what callers may check with `errors.Is`/`errors.As`), where `ctx`
   flows, who owns each goroutine's lifetime, and how it will be tested through the public API.
3. Write it. Match the surrounding package where the style documents are silent
   (G#local-consistency), but never extend an existing deviation into new files or new API.
4. Run the validation commands below on what you wrote before reporting it done.

| Producing | Read |
|---|---|
| Names, packages, doc comments | `references/naming-and-docs.md` |
| Error creation, wrapping, handling, panics, logging of errors | `references/errors.md` |
| Statements, literals, declarations, imports, formatting | `references/language.md` |
| Packages, interfaces, generics, options, contexts, concurrency, global state, flags | `references/api-design.md` |
| Tests, test helpers, test doubles | `references/testing.md` |
| A review or validation pass | `references/review-checklist.md` |

## Rules that come up in almost every file

These are the high-frequency ones. The references hold the rest.

**Format and names**
- `gofmt` output, always. No fixed line length: refactor a long line rather than wrapping it,
  and never break before an indentation change or split a long string/URL. (G#formatting, G#line-length)
- `MixedCaps` everywhere, constants included: `MaxLength`, not `MAX_LENGTH` or `kMaxLength`.
  No underscores except in `Test_`/`Benchmark`/`Example` function names. (G#mixed-caps, D#underscores)
- Initialisms keep one case: `userID`, `HTTPClient`, `XMLAPI`, `gRPC`/`GRPC`; never `Id`, `Url`, `Http`. (D#initialisms)
- Packages: short, lowercase, no underscores, named for what they provide. Never `util`,
  `common`, `helper`, `model`. (D#package-names)
- No `Get` prefix: `Counts()`, not `GetCounts()`. Use `Fetch`/`Compute` when it is expensive
  or remote. (D#getters)
- Do not repeat context: `widget.New`, not `widget.NewWidget`; `users`, not `userSlice`;
  `(c *Config) WriteTo`, not `WriteConfigTo`. (D#repetition)
- Receivers: one or two letters, abbreviating the type, the same on every method; never
  `this`/`self`. (D#receiver-names)
- Name length grows with scope size and shrinks with frequency of use. (D#variable-names)

**Documentation**
- Every exported top-level name has a doc comment: a full sentence starting with the name.
  One package comment per package, directly above `package`. (D#doc-comments, D#package-comments)
- Document what is not obvious: cleanup duties, returned sentinel errors and error types,
  concurrency only where it is surprising. Do not restate that `ctx` cancels. (B#documentation-conventions)

**Errors**
- Return `error` as the last result, typed as `error` (never a concrete error type). (D#returning-errors)
- Error strings: lower case, no trailing punctuation. (D#error-strings)
- Never discard an error silently. Handle it, return it, or in a truly exceptional case stop
  the program. A deliberate `_` needs a comment saying why it is safe. (D#handle-errors)
- Error first, early return, no `else` after a terminal branch; the happy path stays at the
  left margin. (D#indent-error-flow)
- No in-band errors (`-1`, `""`, `nil` meaning failure): return `(value, ok)` or `(value, error)`. (D#in-band-errors)
- Add context that the wrapped error lacks; do not repeat what it already says; do not wrap
  with a bare "failed". `%w` only when callers are meant to inspect the cause, placed at the
  end (`"...: %w"`); `%v` otherwise and at system boundaries. (B#error-extra-info, B#error-percent-w)
- Callers distinguish errors with sentinels or types via `errors.Is`/`errors.As`, never by
  matching strings. (B#error-structure)
- No `panic` for normal error handling. `MustXxx` helpers only at program start-up or in test
  setup. Do not `recover` to keep a process alive. (D#dont-panic, D#must-functions, B#checks-and-panics)

**Design**
- No interface until there is a real need. The consumer defines it, small, with only the
  methods it uses. Accept interfaces, return concrete types. (D#interfaces)
- `context.Context` is the first parameter, never a struct field, never a custom type. Only
  entry points (`main`, `init`, tests via `t.Context()`) create a root context. (D#contexts)
- Every goroutine has an evident end. Prefer synchronous functions and let the caller add
  concurrency. (D#goroutine-lifetimes, D#synchronous-functions)
- No package-level mutable state in library code; clients create instances and pass
  dependencies explicitly. (B#globals)
- Generics only when they meet a real requirement; never to build a DSL. (D#generics)
- Long parameter lists become an option struct (or, when justified, variadic options). (B#funcargs)
- Flags are defined only in `package main`. (D#flags)
- `crypto/rand` for anything key-like, never `math/rand`. (D#crypto-rand)

**Tests**
- Only the standard `testing` package. No assertion libraries, no third-party frameworks.
  Compare with `cmp.Equal`/`cmp.Diff` from `go-cmp`. (D#use-package-testing, D#assert)
- Failure messages: `YourFunc(%v) = %v, want %v`; got before want; include the inputs. (D#useful-test-failures)
- Prefer `t.Error` and keep going; `t.Fatal` only when continuing is pointless. (D#keep-going)
- Table-driven tests with field names; identify rows by name or input, never by index. (D#table-driven-tests)

## Reading the rules outside Google

The source documents describe Google's monorepo. Apply them here as follows:

- "Consistent with the Google codebase" means consistent with this repository.
- `log.Exit` and `log.Fatal` in the sources are Google's logging package (open-sourced as
  `glog`), not the standard `log`. Keep the intent with whatever logger this project uses:
  initialization errors propagate to `main`, which exits with an actionable message and no
  stack trace; a fatal-with-stack is for broken invariants only.
- Bazel details (`go_library`, `testonly`, several packages per directory) do not apply with Go
  modules. The naming rules attached to them still do.
- Protocol buffer rules apply only where protos are used.
- Where a source says to consult the Go style mailing list (for example before using
  `context.Background()` in library code), stop and ask the user instead.
- Flag names in snake_case assume the standard `flag` package. If the project adopts cobra,
  its getopt-style convention applies to flag names; the rule that flags live only in `main`
  does not change.

## Validating Go code

Use this when asked to validate, review or check Go code, and on your own output before
calling it done. Scope: `validate diff` means changed files; `validate <path>` that subtree;
with nothing given, the Go files you touched.

1. **Mechanical checks.** Run what is available and report real output:
   - `gofmt -l .` (must print nothing) and `gofmt -s -d <files>` for simplifications
   - `go vet ./...`
   - `go build ./...` and `go test ./...`
   - `staticcheck ./...` or `golangci-lint run` if the project already has them configured
2. **Style pass.** Work through `references/review-checklist.md`. It lists the checks in
   precedence order with search patterns for finding candidates. A pattern hit is a place to
   look, not a finding; confirm each one by reading the code.
3. **Report** findings grouped by weight, each with `file:line`, the rule tag, and the fix:

```markdown
# Go style validation: <scope>

**Result:** <Conforms | Conforms with minor issues | Does not conform> - <one sentence>
**Checks run:** gofmt <ok/N files>, go vet <ok/N>, build <ok/fail>, tests <ok/fail/not run>

## Must fix (Guide and "must" Decisions)
- `path/file.go:42` - <what is wrong> (D#initialisms). Fix: `userId` -> `userID`.

## Should fix (Decisions)
- ...

## Consider (Best Practices)
- ...

## Deviations kept on purpose
- <local-consistency cases or intentional exceptions, with the reason>
```

Weighting:
- **Must fix:** anything from the Guide; Decisions worded as "must", "do not" or "never";
  anything that is also a correctness risk (discarded error, leaked goroutine, copied mutex,
  `math/rand` for keys, `t.Fatal` from a goroutine).
- **Should fix:** the remaining Decisions.
- **Consider:** Best Practices.

Keep proportion (index#about): the documents are not a licence for large rewrites. In
existing code, fix what you touch and what is near it; do not churn a whole package to remove
a style difference, and do not nit-pick every instance. For a deviation that is confined to
one file and would be out of scope to fix, note it rather than fixing it. New code gets the
full standard.

When asked to fix: apply Must and Should findings, re-run the mechanical checks, and report
what changed and what was left.
