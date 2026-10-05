# Review checklist

For validating Go code against the three documents. Work top to bottom; the sections follow
the reporting weights in SKILL.md. Each line gives the rule tag and, where useful, a search
pattern (ripgrep syntax, run over `*.go`).

A pattern finds candidates. Read the code before reporting anything: many hits are legitimate
(an HTTP `Get`, a deliberate `_`, a sentinel-first `%w`).

Mechanical checks come first and are not optional: `gofmt -l .`, `go vet ./...`,
`go build ./...`, `go test ./...`.

## 1. Must fix

### Correctness-bearing rules

| Check | Tag | Pattern |
|---|---|---|
| Error discarded without a comment explaining why | D#handle-errors | `^\s*_\s*=\s*\w.*\(` ; `,\s*_\s*:?=\s*\w.*\(` ; calls whose error result is ignored (`go vet`, `errcheck` if available) |
| Goroutine with no evident end or nobody waiting | D#goroutine-lifetimes | `\bgo\s+(func\b|\w+[\.\(])` |
| Struct with a mutex or `bytes.Buffer` copied, passed or received by value | D#copying | `sync\.(Mutex|RWMutex|WaitGroup|Once)` then check receivers and parameters; `go vet` copylocks |
| `math/rand` used for keys, tokens, secrets | D#crypto-rand | `"math/rand` |
| `t.Fatal`/`t.FailNow` called from a spawned goroutine | B#t-fatal-goroutine | `t\.(Fatal|Fatalf|FailNow)` inside `go func` |
| Exported function returns a concrete error type | D#returning-errors | `\)\s*\(?[^)]*\*\w*Error\)?\s*\{` |
| `:=` in an inner block shadowing a variable used after it (classic: `ctx`, `err`) | B#shadowing | `ctx, \w+ := context\.With` inside `if`/`for` |
| Errors told apart by message text | B#error-structure | `err\.Error\(\)` near `strings\.(Contains|HasPrefix)`, `==`, `regexp` |

### Style Guide (canonical)

| Check | Tag | Pattern |
|---|---|---|
| Not `gofmt`-clean | G#formatting | `gofmt -l .` |
| Snake case or ALL_CAPS identifiers | G#mixed-caps | `\b(const|var|func|type)\s+\w*_\w+` (ignore `Test*`, `Benchmark*`, `Example*`) |
| Lines wrapped to satisfy a column: broken signatures, broken `if` conditions, split strings | G#line-length | `^func .*[,(]\s*$` ; `(&&|\|\|)\s*$` ; `"\s*\+\s*$` |
| Code that is unclear, clever, or more abstract than its job needs | G#clarity, G#simplicity | read |
| Heavier mechanism than needed: new dependency, generic, interface or helper where a language construct or the standard library suffices | G#least-mechanism | review `go.mod` diff and new abstractions |
| A critical detail that is easy to miss, uncommented | G#maintainability | `err == nil` ; `if \w+, err = ` |
| New code spreading an existing local deviation | G#local-consistency | compare with neighbours |

### Decisions worded as must / do not / never

| Check | Tag | Pattern |
|---|---|---|
| Package named `util`, `common`, `helper`, `model`, etc., or with underscores or capitals | D#package-names | `^package (utils?|utility|common|helpers?|models?|types|misc|base|shared|lib)\b` ; `^package \w*[A-Z_]\w*` (ignore `_test`) |
| Receiver named `this`/`self`, long, or inconsistent across methods | D#receiver-names | `^func \((this|self|me)\b` ; `^func \(\w{4,} ` |
| Constants in ALL_CAPS or with a `k` prefix; named after their value | D#constant-names | `\b[A-Z][A-Z0-9]+_[A-Z0-9_]+\b` ; `\bk[A-Z]\w+\s*=` |
| Mixed-case initialisms | D#initialisms | `\w*(Id|Url|Uri|Http|Https|Json|Xml|Api|Sql|Uuid|Html|Tcp|Udp|Ip|Rpc|Grpc|Tls|Ssh|Cpu|Dns|Jwt|Ttl)([A-Z]\w*)?\b` (check each: `Identifier` is fine) |
| Exported top-level name without a doc comment, or comment not starting with the name | D#doc-comments | exported `func`/`type`/`var`/`const` not preceded by `// Name ...` |
| Missing, duplicated or detached package comment | D#package-comments | `^// Package \w+` count per package |
| Struct literal of an external type without field names | D#literal-field-names | `go vet` composites |
| Closing brace not aligned in multi-line literals | D#literal-matching-braces | `gofmt` catches most |
| Dot import | D#import-dot | `^\s*\.\s+"` |
| Blank import outside `main` and tests | D#import-blank | `^\s*_\s+"` (allow `_ "embed"`) |
| `panic` used for ordinary error handling | D#dont-panic | `\bpanic\(` |
| `Must*` helper called outside start-up or test setup | D#must-functions | `\b[Mm]ust[A-Z]\w*\(` |
| Context not first parameter, stored in a struct, or a custom context type | D#contexts, D#custom-contexts | `func .*\([^)]+,\s*ctx context\.Context` ; `^\s+\w*\s*context\.Context\s*$` inside `struct` ; `type \w*Context (struct|interface)` |
| `context.Background()`/`TODO()` outside `main`, `init`, tests | D#contexts | `context\.(Background|TODO)\(\)` |
| Flags defined outside `package main` | D#flags | `\bflag\.(String|Int|Bool|Duration|Var|\w+Var)\(` |
| Assertion library or third-party test framework | D#assert, D#use-package-testing | `testify|\bassert\.\w+\(t|\brequire\.\w+\(t|ginkgo|gomega|goconvey|gocheck|go-is|gotest\.tools` |
| Home-made assertion helper (takes `t`, checks the code under test, fails) | D#assert, B#test-functions | `func (assert|expect|check|verify|require)\w*\(t \*testing\.T` |
| Table test failure identified by index | D#table-tests-identifying-the-row | `case #?%d` ; `for i, \w+ := range (tests|cases|tt)` then `i` in a message |
| Test helper that fails without `t.Helper()` | D#mark-test-helpers | functions taking `t \*testing\.T` whose first statement is not `t.Helper()` |

## 2. Should fix (remaining Decisions)

| Check | Tag | Pattern |
|---|---|---|
| `Get` prefix on accessors | D#getters | `func (\(\w+ \*?\w+\) )?Get[A-Z]\w*\(` |
| Repetitive names: package in symbol, type in variable, receiver in method | D#repetition | `New<Pkg>`, `\w+(Slice|Map|String|Int|List|Array)\b` variables, `<Type>` repeated in method names |
| Dropped letters or unclear abbreviations; names too long or too short for their scope | D#variable-names | read |
| Named results that add nothing; naked returns in non-trivial functions | D#named-result-parameters | `^\s*return\s*$` in functions with results |
| Error strings capitalized or ending in punctuation | D#error-strings | `(errors\.New|fmt\.Errorf)\("[A-Z][a-z]` ; `(errors\.New|fmt\.Errorf)\("[^"]*[.!:]"` |
| In-band error values | D#in-band-errors | `return -1\b` ; `return ""$` ; docs saying "returns -1/empty/nil if" |
| `else` after a terminal branch; happy path indented | D#indent-error-flow | `return[^\n]*\n\s*\} else \{` (multiline) |
| Long-lived variable declared in an `if` initializer with an `else` | D#indent-error-flow | `if \w+, err := .*; err != nil \{` followed by `} else {` |
| `[]T{}` for an empty local slice; `== nil` to test emptiness; API distinguishing nil from empty | D#nil-slices | `:= \[\]\w+\{\}` ; `\w+ [!=]= nil` on slices |
| Repeated type names in slice/map literals | D#literal-repeated-type-names | `gofmt -s -d` |
| Zero-value fields spelled out needlessly | D#literal-zero-value-fields | `:\s*(0|""|nil|false),$` |
| Redundant `break` in `switch` | D#switch-break | `^\s*break\s*$` |
| Yoda conditions | D#conditional-formatting | `if\s+("[^"]*"|\d+|nil|true|false)\s*[!=]=` |
| Pointer to a string or to an interface as a parameter | D#pass-values | `\*string\b` ; `\*(io\.\w+|error|\w+er)\b` in parameters |
| Wrong or mixed receiver kinds on one type | D#receiver-type | list receivers per type |
| Interface defined by the producer, before a second implementation, or just for mocking; large interfaces; interface returned without reason | D#interfaces | `type \w+ interface \{` then count implementations and locate consumers |
| Generics with a single instantiation, or building a DSL | D#generics | `func \w+\[\w+ ` ; `type \w+\[\w+ ` |
| Asynchronous API where a synchronous one would do | D#synchronous-functions | functions returning channels or taking callbacks |
| Type alias that is not a migration aid | D#type-aliases | `^type \w+ = ` |
| Manual quoting instead of `%q` | D#use-percent-q | `\\"%s\\"` ; `'%s'` |
| `interface{}` in new code | D#use-any | `interface\{\}` |
| Imports not grouped std / other / proto / side-effect; needless renames | D#import-grouping, D#import-renaming | read import blocks |
| Comment paragraphs on one very long line; doc sentences unpunctuated | D#comment-line-length, D#comment-sentences | `^\s*//.{140,}` |
| Test failure messages missing function, inputs, or with want before got; "expected/actual" wording | D#useful-test-failures, D#got-before-want | `t\.(Error|Fatal)f?\("(expected|want|wrong|unexpected|mismatch|fail)` ; `expected .* (got|actual)` |
| `t.Fatal` where `t.Error` would let the test keep going | D#keep-going | `t\.Fatalf?\(` after comparisons |
| `reflect.DeepEqual` in tests | D#types-of-equality | `reflect\.DeepEqual` |
| Field-by-field struct checks instead of one comparison | D#compare-full-structures | runs of `if got\.\w+ != ` |
| Diff printed without a `(-want +got)` legend | D#print-diffs | `cmp\.Diff\(` then the message |
| Error messages string-compared in tests | D#test-error-semantics | `err\.Error\(\) [!=]=` ; `strings\.Contains\(err\.Error\(\)` |
| Assertions on serialized output from another package | D#compare-stable-results | string equality on `json.Marshal` output |
| Subtest names with spaces, slashes or prose | D#subtest-names | `t\.Run\("[^"]*[ /]` |
| Table rows that switch logic or setup inside the loop | D#table-tests-data-driven | `switch test\.` / `if test\.\w+ \{` inside the loop |

## 3. Consider (Best Practices)

| Check | Tag | Pattern |
|---|---|---|
| Function names repeating package, receiver, parameter or return types | B#function-names | read signatures |
| Variable named after a standard package | B#shadowing | `\b(url|path|json|time|errors|strings|bytes|http|context|filepath|log|sort|sync)\s*:?=` |
| Wrap adds nothing or repeats the cause | B#error-extra-info | `Errorf\("(failed|error|err)[: ]*%[vw]"` ; `"(could not|failed to|unable to) (open|read|write) ` wrapping `os` errors |
| `%w` where callers should not see the cause (boundaries), or `%v` where they need it | B#error-extra-info | every `%w`; every `fmt.Errorf` at RPC/HTTP/storage edges |
| `%w` not at the end (and not a leading sentinel) | B#error-percent-w | `Errorf\("[^"]*%w[^"]+"` |
| Error both logged and returned | B#error-logging | log call immediately before `return .*err` |
| Start-up errors handled with a stack trace or deep in the call tree instead of in `main` | B#program-init | fatal calls outside `main` |
| `recover()` used to keep running; panic crossing a package boundary | B#checks-and-panics, B#when-to-panic | `recover\(\)` |
| Docs restating parameters, context cancellation or obvious concurrency; cleanup duties or returned error types undocumented | B#documentation-conventions | read doc comments of constructors and I/O functions |
| `var x = v` for non-zero values; explicit zero composite literals | B#vardeclinitialization, B#vardeclzero | `^\s*var \w+ = ` inside functions ; `= \w+\{\}$` |
| Size hints with no evidence | B#vardeclsize | `make\(\[\]\w+, 0, ` ; `make\(map\[.*\], ` |
| Channel parameters without direction | B#decl-chan | `\(\w+ chan \w+` |
| Long parameter lists; boolean parameters side by side | B#funcargs | signatures with 5+ parameters ; `, \w+, \w+ bool` |
| Variadic options where an option struct would do; presence-style options | B#option-structure, B#variadic-options | `func With\w+\(\) ` ; `func Enable\w+\(\) ` |
| Package-level mutable state, registries, singletons in library code; `init` with side effects | B#globals | `^var \w+ (=|\w)` outside `main` (exclude errors and constants-in-spirit) ; `^func init\(\)` ; `sync\.Once` guarding a global client |
| Package too large, too fragmented, or an interface used to break a cycle | B#package-size | `go list ./...` and import graph |
| String building: `+` in loops, `Sprintf` then write to a Writer, `+`-joined multi-line constants | B#string-concat | `\+= ` on strings in loops ; `Write\w*\(.*fmt\.Sprintf` |
| Shared fixtures in `init`/package variables; `TestMain` without need | B#t-common-setup-scope, B#t-custom-main | `func TestMain` ; `func init\(\)` in `_test.go` |
| Test helper returning `error` for setup instead of failing; missing context in helper failures | B#test-helper-error-handling | `func \w+\(t \*testing\.T.*\) error` |
| Positional fields in long table-test literals | B#t-field-names | read |
| Hand-written client fake where a real client against a test server is possible | B#use-real-transports | fakes implementing client interfaces |
| Test doubles named or placed against convention | B#naming-doubles | `(Mock|Fake|Stub|Spy)\w+` and their packages |

## Cross-cutting questions

Ask these of the change as a whole; they are where the principles apply and no pattern helps.

1. Could a reader new to the package follow each function top to bottom without holding
   earlier code in their head? (Clarity, Simplicity)
2. Is every abstraction (interface, generic, option type, helper package) paying for itself
   today? (Least mechanism)
3. Does anything unusual stand out *because* the rest is idiomatic, and is it commented?
   (Concision)
4. Can the API grow without breaking callers, and is the exported surface as small as it can
   be? (Maintainability)
5. Do the tests fail with messages a maintainer could act on without reading the test?
6. Does the new code look like its neighbours where the documents are silent? (Consistency)
