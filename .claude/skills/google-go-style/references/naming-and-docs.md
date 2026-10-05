# Naming and documentation

Tags: **G** Style Guide, **D** Style Decisions, **B** Best Practices, followed by the anchor in
that document. Full text: `docs/sources/google-go-style/`.

## Contents

- Naming: case and underscores, packages, receivers, constants, initialisms, getters,
  variables, repetition, functions and methods, named results, shadowing, test doubles
- Documentation: doc comments, package comments, sentences and wrapping, what to document,
  godoc formatting, examples, signal boosting

---

## Naming

General (G#naming): names should not feel repetitive when used, should take their context into
account, and should not repeat what is already clear. Go names are shorter than in many
languages. Names should be predictable: the same concept gets the same parameter and receiver
name everywhere (G#maintainability).

### Case and underscores (G#mixed-caps, D#underscores)

- `MixedCaps` / `mixedCaps` for every multi-word name, including constants. Locals count as
  unexported for choosing the first letter.
- No underscores in identifiers. Exceptions:
  1. `Test`, `Benchmark` and `Example` function names in `*_test.go`.
  2. Package names imported only by generated code.
  3. Low-level OS/cgo interop that reuses external identifiers (rare).
- File names are not identifiers and may contain underscores.

### Package names (D#package-names, B#util-packages)

- Concise, lowercase letters and digits only, words run together: `tabwriter`, `oauth2`, `k8s`.
  Not `tabWriter`, `tab_writer`.
- Related to what the package provides. Do not use `util`, `utility`, `common`, `helper`,
  `model`, `testhelper` and the like as a whole name (they may be *part* of one). Judge by the
  call site: `spannertest.NewDatabaseFromFile` reads; `test.NewDatabaseFromFile` does not.
- Avoid names that clash with common local variables: `usercount`, not `count`.
- `_test` suffix is the allowed underscore: `package linkedlist_test` for black-box tests,
  integration test packages, and package-level examples.
- A renamed import must follow the same rules, and the same local name should be used for that
  package throughout the repository.

### Receiver names (D#receiver-names)

- Short (one or two letters), an abbreviation of the type, identical on every method of the
  type. `func (t Tray)`, `func (ri *ResearchInfo)`, `func (w *ReportWriter)`.
- Never `this` or `self`. Never `_`; omit the name when the receiver is unused.

### Constant names (D#constant-names)

- MixedCaps like everything else: `MaxPacketSize`. Not `MAX_PACKET_SIZE`, not `kMaxBufferSize`.
- Name the role, not the value. `const Twelve = 12` is wrong; if a constant has no role beyond
  its value, do not define it.

### Initialisms (D#initialisms)

An initialism keeps a single case throughout; prose spelling decides which.

| Prose | Exported | Unexported | Wrong |
|---|---|---|---|
| XML API | `XMLAPI` | `xmlAPI` | `XmlApi`, `XMLApi`, `xmlApi` |
| ID | `ID` | `id` | `Id`, `iD` |
| DB | `DB` | `db` | `Db` |
| URL | `URL` | `url` | `Url` |
| gRPC | `GRPC` | `gRPC` | `Grpc`, `grpc` |
| iOS | `IOS` | `iOS` | `Ios`, `ios` |
| DDoS | `DDoS` | `ddos` | `DDOS`, `Ddos` |
| Txn | `Txn` | `txn` | `TXN` |

### Getters (D#getters)

No `Get`/`get` prefix unless the concept itself is a "get" (HTTP GET). `Counts()`, not
`GetCounts()`. When the call computes heavily or goes remote, use a word that says so, such as
`Compute` or `Fetch`, so the reader knows it may block or fail.

### Variable names (D#variable-names)

- Length proportional to scope size, inversely proportional to how often the name is used.
  Rough scale: small scope 1-7 lines, medium 8-15, large 15-25, very large beyond a page. A
  one-letter name fine in a small scope needs more in a large one.
- A specific, well-known concept can stay short in a large scope (`db`).
- Name what the variable holds and how it is used here, not where it came from.
- Start with one word (`count`, `options`); add words only to disambiguate (`userCount`,
  `projectCount`).
- Do not drop letters to save typing: `Sandbox`, not `Sbx`.
- Leave out type words: `users`, not `userSlice`; `name`, not `nameString`; `userCount`, not
  `numUsers` or `usersInt`. A type-ish qualifier is fine when two forms coexist
  (`limitStr`/`limit`, `limitRaw`/`limit`).
- Leave out what the surrounding function, type or package already says: inside `UserCount`,
  `count` is enough.

Single letters (D#v): receivers; `r` for `io.Reader`/`*http.Request`, `w` for
`io.Writer`/`http.ResponseWriter`; `i`, `x`, `y` for indices and coordinates; short
abbreviations as loop variables in short loops (`for _, n := range nodes`). Otherwise only
where the full word is obvious and would be repetitive.

### Repetition (D#repetition)

- **Package vs symbol:** `widget.New`, not `widget.NewWidget`; `db.Load`, not
  `db.LoadFromDatabase`. A package exporting one type named after itself uses `New`.
- **Name vs type:** `var users int`, `var name string`, `var primary *Project`.
- **Name vs context:** in package `sqldb`, `Connection`, not `DBConnection`; method
  `(p *Project) Name()`, not `ProjectName()`. Evaluate from the call site.

### Function and method names (B#function-names)

- Omit from the name: input and output types (unless needed to disambiguate), the receiver's
  type, pointer-ness, the package name, parameter names, and return value names.
  `yamlconfig.Parse`, `(c *Config) WriteTo`, `Override(dest, source)`, `Transform(input)`.
- Add detail only to tell siblings apart: `WriteTextTo` / `WriteBinaryTo`.
- Functions that return something read as nouns (`JobName`); functions that do something read
  as verbs (`WriteDetail`).
- Variants differing only by type put the type last: `ParseInt`, `ParseInt64`. A clear primary
  version drops the suffix: `Marshal`, `MarshalText`.

### Named result parameters (D#named-result-parameters)

- Usually unnecessary; the function name and result types say enough.
- Name them when two or more results share a type (`left, right *Node`), when the name tells
  the caller what to do (`ctx Context, cancel func()`), or when a deferred closure must modify
  the result.
- Do not name results just to avoid a `var` in the body, to enable naked returns, or when the
  name only repeats the type (`(node *Node)`).
- Naked returns only in small functions.

### Shadowing (B#shadowing)

- Reassigning with `:=` in the same scope ("stomping") is fine when the old value is no longer
  wanted, as with `ctx, cancel := context.WithTimeout(ctx, ...)`.
- `:=` inside a new block creates a *new* variable; the outer one is unchanged afterwards. For
  a conditional reassignment, declare what is new with `var` and assign with `=`:

  ```go
  if *shortenDeadlines {
      var cancel func()
      ctx, cancel = context.WithTimeout(ctx, 3*time.Second)
      defer cancel()
  }
  ```
- Do not name variables after standard packages (`url`, `path`, `json`) outside very small
  scopes. If a package must be renamed to free a variable name, use the `pkg` suffix
  (`urlpkg`) (D#import-renaming).

### Test doubles and helper packages (B#naming-doubles)

- Helper package for `creditcard` is `creditcardtest`.
- One double for one type: name it for its kind, `creditcardtest.Stub`. Not `StubService`,
  never `StubCreditCardService`.
- Several behaviours: name by behaviour, `AlwaysCharges`, `AlwaysDeclines`.
- Doubles for several types in the package: then qualify, `StubService`, `StubStoredValue`.
- In tests, prefix the local variable so the double stands out from production values:
  `spyCC`, not `cc`.

---

## Documentation

### Doc comments (D#doc-comments, D#comment-sentences)

- Every top-level exported name has one. So do unexported types and functions whose behaviour
  is not obvious.
- A full sentence starting with the name being declared (an article may precede it):
  `// A Request represents a request to run a command.`
  `// Encode writes the JSON encoding of req to w.`
- Write unexported doc comments the same way, so exporting later is a rename.
- Complete sentences are capitalized and punctuated. Fragments, such as end-of-line comments
  on struct fields, need not be:
  `PageLength int // lines per page when printing (optional; default: 20)`
- A comment above a group of struct fields documents the group.
- Written for the user of the package; they appear in godoc and in editors.

### Package comments (D#package-comments)

- Immediately above the `package` clause, no blank line. Exactly one per package.
- Libraries: `// Package math provides basic constants and mathematical functions.`
- `main` packages name the binary: `// The seed_generator command ...`, `// Binary
  seed_generator ...`, `// Command seed_generator ...`.
- If it is long, or no file is the obvious home, use a `doc.go` containing only the comment and
  the package clause.
- File-wide notes for maintainers go after the imports; they are not package comments.

### Wrapping (D#comment-line-length)

No fixed width. Wrap long comment paragraphs so they read well in tools that do not wrap; 80
or 100 columns are common. Stay consistent within a file. Do not break a long URL or literal to
satisfy a column. A whole paragraph on one line is a poor experience.

### What to document (G#clarity, B#documentation-conventions)

Comments explain *why*, and anything a reader could not see from the code. They do not restate
it. Redundant commentary is clutter that must be maintained.

- **Parameters and fields** (B#documentation-conventions-params): only the error-prone or
  non-obvious ones, saying why they matter. "format is the format" adds nothing.
- **Contexts** (B#documentation-conventions-contexts): do not state that cancelling `ctx`
  stops the function and returns `ctx.Err()`; that is assumed. Do document when the function
  returns something else on cancellation, has other ways of being stopped, or expects something
  particular of the context (lifetime, values). Avoid designing APIs with such expectations.
- **Concurrency** (B#documentation-conventions-concurrency): readers assume read-only
  operations are safe concurrently and mutating ones are not. Document only when it is unclear
  which an operation is (a `Lookup` that mutates an LRU), when the API itself provides
  synchronization, or when an interface imposes concurrency requirements on implementers.
- **Cleanup** (B#documentation-conventions-cleanup): always state what the caller must release
  and how (`Call Stop to release...`, `Caller should close resp.Body`).
- **Errors** (B#documentation-conventions-errors): name significant sentinel values and error
  types a function returns, and whether an error type is a pointer (`*PathError`), since that
  decides how `errors.As` and comparisons behave. Put package-wide error conventions in the
  package comment.
- **Interfaces** (B#designing-effective-interfaces): document the contract, edge cases and
  expected errors; per method when there are several. Worth doing for unexported ones too.
- **Complexity** (G#simplicity): where code is deliberately complex (usually for performance),
  say so and say why, so a maintainer knows to take care.

### Godoc formatting (B#godoc-formatting, D#commentary)

- Blank comment line separates paragraphs.
- Indented lines render as preformatted text; use that for code, lists and tables. Avoid other
  decoration.
- Prefer a runnable example to code in a comment when feasible.
- Preview rendered docs (`pkgsite`, or the editor) for anything public (B#documentation-preview).

### Examples (D#examples)

Packages should show intended usage. Provide runnable `ExampleXxx` functions in a `_test.go`
file (not in production files); they appear in godoc and run as tests.

### Signal boosting (B#signal-boost, G#concision)

When a line looks like a common idiom but is not, add a short comment so the difference is not
missed:

```go
if err := doSomething(); err == nil { // if NO error
    // ...
}
```

The same care applies to anything where one character changes the meaning (`=` vs `:=`, a
stray `!`): restructure it or comment it (G#maintainability).
