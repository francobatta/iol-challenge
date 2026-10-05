# Language use and formatting

Tags: **G** Style Guide, **D** Style Decisions, **B** Best Practices, followed by the anchor.
Full text: `docs/sources/google-go-style/`.

## Formatting (G#formatting, G#line-length)

- All source matches `gofmt` output, generated code included.
- There is no line length limit. When a line feels long, refactor (extract a local, shorten a
  name) instead of wrapping it. If it is already as short as practical, leave it long.
- Do not split a line before an indentation change (function signature, `if`, `for`, `switch`)
  and do not split a long string such as a URL.

### Indentation confusion (D#indentation-confusion)

A wrapped line must not line up with the block that follows it, or it reads as part of the
body.

### Function signatures and calls (D#func-formatting)

- A function or method signature stays on one line.
- Do not wrap a call just because it is long. Shorten it by naming intermediate values:

  ```go
  local := helper(some, parameters, here)
  good := foo.Call(list, of, parameters, local)
  ```
- Wrapping a call is fine when it groups arguments meaningfully (one vertex per line), not at
  an arbitrary column.
- No inline comments labelling arguments (`42, // Port`). Use an option struct or better docs.
- Do not break a long format string. Put the string on the first line and the arguments on the
  following lines, grouped by meaning.

### Conditionals and loops (D#conditional-formatting)

- Do not break an `if` condition across lines. Extract named booleans (when short-circuiting
  is not needed) or repeated sub-expressions:

  ```go
  inTransaction := db.CurrentStatusIs(db.InTransaction)
  keysMatch := db.ValuesEqual(db.TransactionKey(), row.Key())
  if inTransaction && keysMatch {
      // ...
  }
  ```
- An `if` whose condition contains a closure or multi-line literal is fine as long as the
  braces line up.
- No artificial breaks in `for` clauses; move a condition into the body with `break` if that
  reads better.
- `switch` and `case` lines stay on one line. If a `case` list is too long, put `case` alone,
  indent every value, and follow with a blank line.
- Variable on the left, constant on the right: `if result == "foo"`. No Yoda conditions.

### `switch` and `break` (D#switch-break)

No `break` at the end of a `case`; Go does not fall through. Use a comment for an
intentionally empty case. Remember `break` inside a `switch` inside a `for` leaves only the
`switch`; use a labelled `break` to leave the loop.

## Composite literals (D#literal-formatting)

Build values with literals instead of field-by-field assignment where possible.

- **Field names** (D#literal-field-names): required for struct types from another package.
  Optional for package-local types, but use them when the struct has many fields or it reads
  better.
- **Braces** (D#literal-matching-braces): the closing brace sits at the indentation of the
  line holding the opening brace. In multi-line literals every element line ends with a comma
  and the closing brace is on its own line.
- **Cuddled braces** (D#literal-cuddled-braces): `}, {` between elements is allowed only when
  indentation still matches and the elements are themselves literals.
- **Repeated type names** (D#literal-repeated-type-names): omit them inside slice and map
  literals (`[]*Type{{A: 42}, {A: 43}}`). `gofmt -s` does this.
- **Zero-value fields** (D#literal-zero-value-fields): leave them out when nothing is lost;
  it draws attention to what is being set. In table tests, name the fields and omit the ones
  irrelevant to the case; spell a zero value out only when it is the point of the case.

## Declarations and initialization (B#vardecls)

- New variable with a non-zero value: `i := 42`, not `var i = 42`. (B#vardeclinitialization)
- A value meant to start empty and be filled later: zero-value declaration, `var coords
  Point`, `var primes []int`. Not `Point{X: 0, Y: 0}` or `[]int(nil)`. Typical before an
  unmarshal. (B#vardeclzero)
- Initial contents known: composite literal. (B#vardeclcomposite)
- Pointer to a zero value: `new(T)` or `&T{}`, either is fine.
- A struct containing a mutex or similar keeps it as a value field (`mu sync.Mutex`) so the
  zero value works; the struct is then always handled by pointer with pointer receivers.
- If a composite will be returned or always addressed, declare it as a pointer from the start
  (`c := new(Counter)`), not `var c Counter ... return &c`.
- Maps must be initialized before writing; reading a nil map is fine.
- **Size hints** (B#vardeclsize): `make([]T, 0, n)` / `make(map[K]V, n)` only with evidence
  that it matters, or when the final size is plainly known. Most code should let the runtime
  grow. Over-allocation wastes memory.
- **Channel direction** (B#decl-chan): specify it in signatures wherever possible
  (`<-chan int`, `chan<- int`); the compiler then catches misuse and ownership is clearer.

Not decided (D#non-decisions), choose freely and stay locally consistent: `var i int` vs
`i := 0`; `&File{}` vs `new(File)`; `map[K]V{}` vs `make(map[K]V)`; argument order in
`cmp.Diff` (always print a legend); `errors.New` vs `fmt.Errorf` for a plain string.

## Nil slices (D#nil-slices)

- Declare an empty slice as `var t []string`, not `t := []string{}`.
- Test emptiness with `len(s) == 0`, not `s == nil`.
- Never design an API in which nil and empty slices mean different things.

## Copying, values and pointers

- **Copying** (D#copying): do not copy a value whose methods are on the pointer type. Never
  copy a `sync.Mutex`, `bytes.Buffer`, or a struct containing one. Types with such fields are
  created, passed and returned by pointer.
- **Pass values** (D#pass-values): do not pass a pointer just to save bytes. No `*string`, no
  pointer to an interface. Large structs, structs likely to grow, and protobuf messages are
  passed by pointer.
- **Receiver type** (D#receiver-type). Correctness first:
  - Pointer: the method mutates the receiver; the struct has fields that must not be copied;
    the struct is large; it holds pointers to things that may be mutated; or you are unsure.
  - Value: maps, functions, channels; slices when the method does not reslice or reallocate;
    built-in types that are not modified; small plain-data structs with no mutable fields or
    pointers (`time.Time`).
  - Keep all methods of a type on the same kind of receiver.
  - Do not choose on performance without a realistic benchmark.

## Small rulings

- **`any`** (D#use-any): use `any`, not `interface{}`, in new code.
- **`%q`** (D#use-percent-q): for quoted strings in human-facing output, not `"\"%s\""` or
  `'%s'`. Makes empty strings and control characters visible.
- **Type aliases** (D#type-aliases): `type T1 T2` defines a new type. `type T1 = T2` is rare
  and mainly for migrations; do not use it otherwise.
- **`crypto/rand`** (D#crypto-rand): for keys and tokens of any kind. Never `math/rand`.

## Imports (D#imports)

- **Grouping** (D#import-grouping), in this order, separated by blank lines:
  1. standard library
  2. other packages (project and third party)
  3. protocol buffer imports
  4. side-effect imports (`_ "..."`)
- **Renaming** (D#import-renaming): avoid it. Rename only to resolve a collision (rename the
  most local or project-specific one), for generated proto packages (drop underscores, add a
  `pb` suffix, or `grpc` for service stubs; prefer descriptive names like `foosvcpb` over bare
  `pb`), or for an uninformative name such as `v1`. Use the same local name everywhere.
- **Blank imports** (D#import-blank): only in `package main` or in tests that need them, not in
  libraries. Exception: `_ "embed"` in a file using `//go:embed`.
- **Dot imports** (D#import-dot): never.

## String building (B#string-concat)

- A few pieces: `+`.
- Formatting: `fmt.Sprintf`. Writing to an `io.Writer`: `fmt.Fprintf` directly, not `Sprintf`
  then write.
- Built up piece by piece in a loop: `strings.Builder`.
- Constant multi-line text: a backtick raw string.
- More complex output: `text/template` (or a safe HTML templating package for HTML).

## Least mechanism in practice (G#least-mechanism)

- A set of strings is `map[string]bool` unless real set operations are needed.
- In a test, set the variable a flag is bound to rather than calling `flag.Set`.
- Prefer a loop over a helper library, a concrete type over an interface, a plain function over
  a generic one, until there is a demonstrated need.

## Local consistency (G#local-consistency)

Where the style documents are silent, follow the surrounding file and package. Valid local
choices include `%s` vs `%v` for errors, or buffered channels vs mutexes. Not valid as local
style: line length limits, or assertion libraries in tests. If a change would spread an
existing deviation to more files or more API, local consistency no longer justifies it.
