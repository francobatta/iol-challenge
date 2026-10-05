# Errors

Tags: **G** Style Guide, **D** Style Decisions, **B** Best Practices, followed by the anchor.
Full text: `docs/sources/google-go-style/`.

## Returning errors (D#returning-errors)

- Signal failure with `error`, as the last result. `nil` means success.
- Exported functions return the `error` interface type, never a concrete type such as
  `*os.PathError`: a nil concrete pointer stored in an interface is a non-nil error.
- When an error is returned, callers must treat the other results as unspecified unless the
  documentation says otherwise.
- A function that takes a `context.Context` should usually return an `error`, so the caller
  can tell whether it was cancelled.

## Error strings (D#error-strings)

- Not capitalized (unless starting with an exported name, proper noun or acronym), no trailing
  punctuation, because they get embedded in other messages:
  `fmt.Errorf("something bad happened")`.
- Full messages shown to a human (log lines, test failures, API responses) are a different
  thing and are normally capitalized: `log.Errorf("Operation aborted: %v", err)`.

## Handle every error (D#handle-errors, B#error-handling)

Make a deliberate choice each time. Options:

1. Handle it here.
2. Return it to the caller.
3. In exceptional situations, terminate (fatal log, or `panic` if truly necessary).

Discarding with `_` is rarely right. When it is (a call documented never to fail), say so:

```go
n, _ := b.Write(p) // never returns a non-nil error
```

Ask whether the current function is the one best placed to handle the error before blindly
passing it on. When running several related operations where only the first failure matters,
`errgroup` is the standard tool.

## In-band errors (D#in-band-errors)

Do not return `-1`, `""` or `nil` to mean "failed" or "not found". Return an extra value:

```go
// Lookup returns the value for key or ok=false if there is no mapping for key.
func Lookup(key string) (value string, ok bool)
```

This also stops callers from writing `Parse(Lookup(key))` and blaming the wrong function. Use
`bool` when no explanation is needed, `error` otherwise; it is the last result.

## Indent error flow (D#indent-error-flow)

Deal with the error and leave; keep the normal path unindented.

```go
// Good:
if err != nil {
    // error handling
    return // or continue, etc.
}
// normal code
```

No `else` after a branch that ends in `return`, `continue`, `panic` or a fatal call. If a
value is used for more than a few lines, do not declare it in the `if` initializer:

```go
// Good:
x, err := f()
if err != nil {
    return err
}
// lots of code that uses x
```

## Error structure (B#error-structure)

If callers need to tell errors apart, give them something to test programmatically. Never make
them match on the message text.

- Simplest: package-level sentinel values.

  ```go
  // ErrDuplicate occurs if this animal has already been seen.
  var ErrDuplicate = errors.New("duplicate")
  ```

  Callers compare with `==`, or `errors.Is` if the error may be wrapped.
- When the caller needs extra data, use a struct type with fields (as `os.PathError` carries
  the path), inspected with `errors.As`.
- A project error type with a code and detail, or gRPC `status` with canonical codes, are also
  acceptable where they fit.

```go
// Bad:
if regexp.MatchString(`duplicate`, err.Error()) { ... }
```

## Adding information (B#error-extra-info)

- Add what you know that the underlying error does not. Do not repeat what it already
  contains (the `os` package already includes the path).

  ```go
  // Good:
  return fmt.Errorf("launch codes unavailable: %v", err)
  // Bad: duplicates the path that os.Open already reports
  return fmt.Errorf("could not open settings.txt: %v", err)
  ```
- Do not annotate just to say something failed. `fmt.Errorf("failed: %v", err)` should be
  `return err`.

### `%v` or `%w`

The question is whether callers should be able to inspect the underlying error.

**`%v`** flattens the cause into text. Use it:
- for plain annotation where no caller will inspect the cause;
- when the error is headed for a log or a person;
- at system boundaries (RPC, IPC, storage), where internal errors should be translated into
  the boundary's own error space (for example canonical status codes) rather than leaked.

**`%w`** keeps the chain for `errors.Is` / `errors.As`. Use it:
- inside the application, in helpers that add context while callers still need to recognise
  the underlying sentinel or type;
- when the wrapped error is a documented, tested part of the package's contract.

Wrapping makes the underlying error part of your API. Do it on purpose.

### Placement of `%w` (B#error-percent-w)

Put `%w` at the end, `"...: %w"`, so the printed text reads newest to oldest, matching the
chain:

```go
// Good:
err2 := fmt.Errorf("err2: %w", err1)   // prints "err2: err1"
// Bad:
err2 := fmt.Errorf("%w: err2", err1)
err2 := fmt.Errorf("err2-1 %w err2-2", err1)
```

Exception, sentinel categories (B#error-percent-w-sentinel-placement): when wrapping a
sentinel that classifies the failure, put it first so the category is the first thing read:

```go
var ErrParse = fmt.Errorf("parse error")
var ErrParseInvalidHeader = fmt.Errorf("%w: invalid header", ErrParse)

return fmt.Errorf("%w: invalid character in header: %v", ErrParseInvalidHeader, err)
```

## Logging errors (B#error-logging)

- Do not both log and return an error. Return it and let the caller decide; that avoids
  duplicate log lines.
- A log message should say what went wrong and include what is needed to diagnose it.
- Keep personal data out of logs.
- Error level is for things someone must act on, not simply "worse than a warning"; it can be
  expensive.
- Guard expensive arguments to verbose or debug logging so they are not computed when the
  level is off (B#vlog).

## Program initialization (B#program-init)

Start-up errors (bad flags, bad configuration) propagate up to `main`, which exits with a
message that tells the operator how to fix the problem. A stack trace is not useful there.

## Panics

- **Do not panic for normal error handling** (D#dont-panic). Return errors.
- **Invariant violations** (B#checks-and-panics): terminating is appropriate only when
  internal state is unrecoverable. Prefer a fatal log over `panic`, since deferred functions
  running during a panic can deadlock or corrupt state further. Libraries should return an
  error rather than abort, especially for anything transient.
- **Do not recover panics to stay alive.** The state after a panic is unknown; locks may be
  held. Let it crash and fix the bug. (`net/http` recovering handler panics is regarded as a
  historical mistake.)
- **Acceptable uses** (B#when-to-panic):
  - API misuse of the kind the standard library panics on, which review and tests should catch.
  - An internal mechanism inside one package, with a matching `recover` at the exported
    boundary that turns it into a returned error, and re-panics anything that is not its own.
    Such panics must never cross a package boundary. Take care to release resources.
  - After a call the compiler cannot see is terminal: `panic("unreachable")`.
  - In `init` or a `Must` function when dying at start-up is the intent.

## `Must` functions (D#must-functions)

- A helper that stops the program on failure is named `MustXxx` / `mustXxx`.
- Call them only at program start-up, typically to initialise package-level values:
  `var DefaultVersion = MustParse("1.2.3")`. Never on user input or in request handling, where
  ordinary error returns belong.
- In tests the same convention applies to setup helpers, which call `t.Helper()` and
  `t.Fatal` instead of panicking; this lets them be used as values in table entries.

## Errors in tests

See `testing.md`: test for semantics with `errors.Is` or `cmpopts.EquateErrors`, not message
text; often checking only whether an error occurred is enough.
