# Testing

Tags: **G** Style Guide, **D** Style Decisions, **B** Best Practices, followed by the anchor.
Full text: `docs/sources/google-go-style/`.

## Framework (D#use-package-testing, D#assert)

- The standard `testing` package is the only test framework. No third-party frameworks.
- No assertion libraries, whether imported or home-made (`assert.Equal(t, ...)`,
  `require.NoError(t, ...)` and similar). They either stop at the first failure or drop the
  context that makes a failure useful, and they fragment how tests are written.
- Compare with `==` for simple values and with `github.com/google/go-cmp/cmp` for everything
  else. Write the check and the failure message in the test itself, in Go.

## Useful failures (D#useful-test-failures)

A failure should be diagnosable without opening the test source. It states what failed, with
which inputs, what was returned and what was expected.

- **Format:** `YourFunc(%v) = %v, want %v`. Name the function even though the test name
  implies it (D#identify-the-function).
- **Inputs:** include them when short. When large or opaque, give each case a name or
  description and print that (D#identify-the-input).
- **Got before want**, and use the words "got" and "want" rather than "actual" and
  "expected" (D#got-before-want).
- **Verbs:** `%q` for strings, `%+v` for small structs, a diff for large values
  (D#level-of-detail).
- **Diffs** (D#print-diffs): use `cmp.Diff` and always print a legend that matches the argument
  order, with a newline before the diff:

  ```go
  if diff := cmp.Diff(want, got); diff != "" {
      t.Errorf("Foo(%q) returned unexpected diff (-want +got):\n%s", input, diff)
  }
  ```
- Setup failures need less detail: `t.Fatalf("Setup: failed to set up test database: %v", err)`.
- Trigger the failure once while writing the test and read the message.

## What to compare

- **Whole structures** (D#compare-full-structures): build the expected value and compare it in
  one go; do not hand-code field-by-field checks. Use `cmpopts` to ignore or approximate
  fields. Multiple return values are compared individually; no need to wrap them in a struct.
- **Equality tools** (D#types-of-equality): `cmp.Equal` and `cmp.Diff`. Protobufs need
  `protocmp.Transform()`. Do not use `reflect.DeepEqual`; it is sensitive to unexported fields
  and implementation details. `cmp` is for tests only, never production code (it may panic).
- **Stable results** (D#compare-stable-results): do not assert on output whose exact form
  belongs to another package, such as the bytes from `json.Marshal`. Parse it and compare the
  meaning.
- **Error semantics** (D#test-error-semantics): never compare error message strings to decide
  which error occurred; that makes a change-detector test. If only presence matters, compare
  against a `wantErr bool`:

  ```go
  err := f(test.input)
  if gotErr := err != nil; gotErr != test.wantErr {
      t.Errorf("f(%q) = %v, want error presence = %v", test.input, err, test.wantErr)
  }
  ```

  For a specific error use `errors.Is`, or `cmp` with `cmpopts.EquateErrors`. Checking that a
  message from the package under test contains some property (a parameter name) is acceptable.

## Keep going (D#keep-going, B#t-fatal)

- Report mismatches with `t.Error` so one run shows every failure.
- `t.Fatal` when continuing would be meaningless or misleading: setup failed, or a later check
  depends on an earlier result.
- In a table test without subtests: `t.Error` then `continue`. Inside `t.Run`: `t.Fatal` ends
  just that subtest.
- **Never call `t.Fatal`, `t.FailNow` or their relatives from a goroutine other than the one
  running the test** (B#t-fatal-goroutine). In spawned goroutines use `t.Error` and return.

## Table-driven tests (D#table-driven-tests)

Use them when many cases share the same logic.

```go
func TestCompare(t *testing.T) {
    tests := []struct {
        a, b string
        want int
    }{
        {"", "", 0},
        {"a", "", 1},
        {"", "a", -1},
    }
    for _, test := range tests {
        got := Compare(test.a, test.b)
        if got != test.want {
            t.Errorf("Compare(%q, %q) = %v, want %v", test.a, test.b, got, test.want)
        }
    }
}
```

- Use field names in case literals once cases are long, have adjacent fields of the same type,
  or omit zero-valued fields (B#t-field-names). Omit fields irrelevant to a case.
- Never identify a case by its index ("case #3 failed"). Print the inputs or a name
  (D#table-tests-identifying-the-row).
- Keep branching out of the loop body. When cases need different logic or different setup,
  write separate test functions (for example one table for successes, one for errors) instead
  of rows that switch behaviour (D#table-tests-data-driven).

## Subtests (D#subtests)

- Optional; useful for filtering, setup and cleanup, and parallelism.
- Each subtest must be runnable alone; none may depend on another having run.
- Names (D#subtest-names): short, identifier-like, easy to type on the command line. No
  spaces, no slashes, no long prose. Put a longer description in a separate field and print it
  on failure. Still print the inputs in the failure message.

## Helpers (D#mark-test-helpers, B#test-functions, B#test-helper-error-handling)

Two kinds of function get called from tests; only one is welcome.

- **Test helpers** do setup or cleanup. A failure in one is a failure of the environment, not
  of the code under test. They take `t` (after `ctx`, before other parameters), call
  `t.Helper()` first, and on failure call `t.Fatal` with a message that says what was being
  set up. Name must-style helpers `mustXxx`. Register teardown with `t.Cleanup`. If a helper
  cannot fail the test, drop the `t` parameter.

  ```go
  func mustAddGameAssets(t *testing.T, dir string) {
      t.Helper()
      if err := os.WriteFile(path.Join(dir, "pak0.pak"), pak0, 0644); err != nil {
          t.Fatalf("Setup failed: could not write pak0 asset: %v", err)
      }
  }
  ```
- **Assertion helpers** check the code under test and fail the test. Do not write them. The
  decision to fail, and the message, belong in the `Test` function.

When many tests need the same validation:

1. Inline it, even if repetitive.
2. Or fold the cases into a table and keep the check in the loop.
3. Or write a function that returns a value, an `error`, or a `cmp.Option`, and let the test
   decide what to do with it.

Test code is still code: all other rules apply to it.

## Setup scope (B#t-common-setup-scope)

- Call setup from the tests that need it. Do not load shared fixtures in `init` or
  package-level variables; a test run alone should not pay for others' setup.
- Expensive setup needed by some tests with no teardown: a `sync.Once` inside the helper is
  acceptable (B#t-setup-amortization).
- `TestMain` only when every test in the package needs the setup *and* it needs teardown. It is
  not the first choice. Put the work in a function that can `defer`, and call `os.Exit` only
  from `TestMain` itself (B#t-custom-main).
- Tests should be hermetic; at minimum, restore any global state they change.

## Packages, contexts, doubles

- **Same package** (D#test-same-package): `foo_test.go` in `package foo`, with access to
  unexported identifiers.
- **Different package** (D#test-different-package): `package foo_test` for black-box tests,
  integration tests, or to avoid an import cycle.
- **Context** (D#contexts): start from `t.Context()`; pass it explicitly into helpers.
- **Test doubles** (B#naming-doubles): live in a `<pkg>test` package; naming rules are in
  `naming-and-docs.md`. Do not add an interface to production code only to inject a double
  (D#interfaces).
- **Real transports** (B#use-real-transports): when testing code that talks HTTP or RPC, use
  the real client against a test server or fake backend instead of hand-implementing the
  client side.
- **Acceptance tests for implementers** (B#test-validation-apis): to let others validate their
  implementation of your interface, provide a `<pkg>test` function that exercises it and
  *returns an error*, as `fstest.TestFS` does. The caller's test decides how to report it.

## Examples (D#examples)

Runnable `ExampleXxx` functions in `_test.go` files document usage, appear in godoc, and are
verified by `go test`.
