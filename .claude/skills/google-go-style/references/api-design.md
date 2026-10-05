# Package and API design

Tags: **G** Style Guide, **D** Style Decisions, **B** Best Practices, followed by the anchor.
Full text: `docs/sources/google-go-style/`.

## Contents

- Packages and files
- Interfaces
- Generics
- Function signatures: option structs and variadic options
- Contexts
- Concurrency: goroutine lifetimes, synchronous functions
- Global state
- Flags, logging, command-line interfaces

---

## Packages and files (B#package-size, D#package-names)

- Judge a package by how its names read at the call site: `bytes.Buffer`, `ring.New`. The
  package name is part of every exported identifier.
- Put things in the same package when client code needs them together, or when their
  implementations are tightly coupled (sharing unexported details keeps that coupling out of
  the public API). Test: if a user must import both packages to use either meaningfully, they
  should be one package.
- Give something its own package when it is conceptually distinct. Do not put a whole project
  in one package, and do not create many tiny packages that cannot stand on their own.
- Needing an interface to break an import cycle usually means the package boundaries are wrong.
- Files: no "one type, one file" rule. Avoid files of many thousands of lines and avoid a
  scatter of tiny files. A maintainer should be able to guess which file holds something.
  Group related code per file, as `net/http` does with `client.go`, `server.go`, `cookie.go`.
- With Go modules, one package per directory.
- If code is useful both as a library and a binary, keep the library separate and make the CLI
  one of its clients (B#complex-clis).

Maintainability checks for any API (G#maintainability): can it grow without breaking callers;
does it state its assumptions; do its abstractions follow the problem rather than the code
layout; does it avoid needless coupling and unused features; are dependencies minimal.

## Interfaces (D#interfaces, B#interfaces)

**Default: do not create one.** Write the concrete type first.

- Designing a "service" or "repository" does not mean declaring `type Service interface`.
- Do not wrap a generated RPC client in a hand-written interface for abstraction or testing;
  test against a real transport with a test server (B#use-real-transports).
- Do not export test doubles or back doors alongside the real implementation. Each exported
  type is more for the reader to learn. Design so the real implementation can be tested
  through its public API.

**Create one when:**

1. Two or more concrete types must be handled by the same logic.
2. Two packages must be decoupled to avoid a cycle (treat as a warning sign, see above).
3. A function needs one or two methods of a type with a very large surface.

**Ownership and shape:**

- The *consumer* defines the interface, containing only the methods it uses. Keep interfaces
  small; the bigger the interface, the weaker the abstraction.
- Keep it unexported if it is used only inside the package.
- The *producer* exports the interface when the interface is the product, a protocol many
  implementations follow (`io.Writer`, `hash.Hash`), or when many packages would otherwise each
  redeclare an identical one. For large systems it may live in its own implementation-free
  package.
- Document every interface as the user manual for the abstraction: contract, edge cases,
  expected errors, concurrency requirements.

**Accept interfaces, return concrete types.** A concrete return gives the caller everything
the type offers, and it can still be passed wherever an interface is wanted. Returning an
interface is right for:

- `error` (always).
- Encapsulation, when exposing the concrete type would let callers break invariants or would
  prevent changing the implementation later. Ask whether that risk is real; do not encapsulate
  by rote.
- Factories, strategies, commands and chaining APIs that return one of several concrete types
  chosen at run time.
- Breaking an import cycle.

Prefer one robust concrete type where it can absorb the variation, as `database/sql.DB` does.

Cost to keep in mind (G#maintainability): an interface removes information. Editors can jump
to a concrete method and its docs; with an interface the reader must work out the
implementation.

## Generics (D#generics)

- Allowed where they meet a real requirement. Often slices, maps, interfaces and plain
  functions do the job without the extra complexity.
- If only one type is ever instantiated, write the code for that type. Adding a type parameter
  later is easy; removing an unneeded abstraction is not.
- Not justified merely because an algorithm does not care about its element type.
- Several types sharing a useful interface: model with the interface. Otherwise prefer
  generics to `any` with heavy type switching.
- Never use generics to build a DSL, an error-handling framework, or an assertion library.
- Exported generic APIs need thorough documentation and, ideally, runnable examples.

## Function signatures (B#funcargs)

Keep parameter lists short. Many parameters, especially adjacent ones of the same type, are
hard to read and easy to swap at the call site. Options, in order of weight:

1. Split a highly configurable function into several simpler ones sharing an unexported
   implementation.
2. **Option struct** (B#option-structure): a struct holding some or all arguments, passed last.

   ```go
   type ReplicationOptions struct {
       Config              *replicator.Config
       PrimaryRegions      []string
       ReadonlyRegions     []string
       ReplicateExisting   bool
       ReplicationInterval time.Duration
   }

   func EnableReplication(ctx context.Context, opts ReplicationOptions) { ... }
   ```

   Self-documenting at the call site, defaults by omission, per-field docs, grows without
   breaking callers. Prefer it when all callers set at least one option, many callers set
   several, or the options are shared between functions. A context is never a field of it.
3. **Variadic options** (B#variadic-options): `func WithX(...) Option` closures passed to a
   `...Option` parameter. This costs a good deal of extra code, so use it only when most of
   these hold: most callers pass no options; there are many options, rarely used; options take
   arguments, can fail, or need lengthy documentation; other packages may supply options.
   Rules when used:
   - Options take parameters instead of signalling by presence: `FailFast(enable bool)`, not
     `EnableFailFast()`.
   - Applied in order; last one wins for non-cumulative options.
   - The options struct they mutate is normally unexported.

These apply mainly to exported APIs. For internal code, balance against least mechanism.

## Contexts (D#contexts)

- `context.Context` is the first parameter of any function that takes one, named `ctx`. In
  test helpers it comes before `t`.
- Never store a context in a struct. Pass it to each method that needs it. (Sole exception:
  matching a signature dictated by a standard or third-party interface.)
- Never define a custom context type or accept anything other than `context.Context`. No
  exceptions (D#custom-contexts). Application data travels as parameters, on the receiver, or
  where it truly belongs as a context value.
- Only entry points create a root context with `context.Background()`: `main`, `init`. Tests
  use `t.Context()`. HTTP handlers use `req.Context()`; streaming RPC handlers use the
  stream's context; cobra commands use `cmd.Context()`. Code in the middle of a call chain
  takes the context from its caller. If library code seems to need `context.Background()`,
  stop and ask the user.
- Contexts are immutable; passing the same one to several calls is fine.

## Concurrency

### Goroutine lifetimes (D#goroutine-lifetimes)

Whenever a goroutine is started it must be clear when, or whether, it stops. A goroutine
blocked on a channel is never collected; one left running can race on inputs the caller thinks
it owns again, or send on a closed channel and panic.

- Keep concurrency inside the function that starts it: spawn, wait, return.
- Tie lifetimes to a `context.Context` and wait before returning:

  ```go
  func (w *Worker) Run(ctx context.Context) error {
      var wg sync.WaitGroup
      for item := range w.q {
          wg.Add(1)
          go func() {
              defer wg.Done()
              process(ctx, item) // returns at latest when ctx is cancelled
          }()
      }
      wg.Wait() // prevent spawned goroutines from outliving this function
      return nil
  }
  ```
- A bare `go process(item)` with nothing waiting for it and no way to stop it is the pattern to
  reject.
- If the exit condition is still not obvious from the code, document when and why it exits.

### Synchronous functions (D#synchronous-functions)

Prefer functions that return their results directly and have finished all callbacks and
channel operations by the time they return. Callers can add concurrency by calling them in a
goroutine; they cannot remove concurrency that is built in. Synchronous code is easier to test
and to reason about.

### Channels

Declare direction in signatures (B#decl-chan). Document concurrency guarantees only where a
reader would guess wrong (see `naming-and-docs.md`).

## Global state (B#globals)

Libraries must not make clients depend on package-level state. If functionality has state, let
clients create instances and pass them as explicit dependencies (constructor parameters,
function parameters, struct fields).

```go
// Good:
package sidecar

type Registry struct{ plugins map[string]*Plugin }

func New() *Registry { return &Registry{plugins: make(map[string]*Plugin)} }

func (r *Registry) Register(name string, p *Plugin) error { ... }
```

Forms to avoid (B#globals-forms): package-level variables that control behaviour, exported or
not; a global service locator; global registries of callbacks; lazily initialised singleton
clients for backends, storage or other resources.

Why: tests interfere with each other and become order-dependent; nothing can run in parallel
or use two independent configurations in one process; registration collisions and
call-before-init ordering have no good answer.

Existing code that does this is not a precedent.

**Litmus tests** (B#globals-litmus-tests). Package state is unsafe when independent functions
or tests interact through it, when users want to swap it for a test double, or when it has
ordering requirements (`init`, flag parsing). It can be acceptable when it is logically
constant, when the package's observable behaviour is stateless (an invisible cache), when it
does not reach outside the process, or when no predictable behaviour is promised.

**Default instance** (B#globals-default-instance). If a convenience package-level API is
really wanted: the package must still offer isolated instances; the global API is a thin proxy
to an instance (as `http.Handle` is to `http.DefaultServeMux`); only binaries use it, never
libraries; its invariants are documented and enforced and it can be reset for tests.

## Flags (D#flags)

- Define flags only in `package main`. Libraries are configured through their Go API
  (parameters, struct fields), never by registering flags as an import side effect.
- With the standard `flag` package: flag names in snake_case, the Go variable in mixedCaps.

  ```go
  var pollInterval = flag.Duration("poll_interval", time.Minute, "Interval to use for polling.")
  ```
- Group flag variables in their own `var` block after the imports.

## Logging (D#logging, B#error-logging)

- Use the project's chosen logger consistently. (The source documents assume Google's `log`
  package, open-sourced as `glog`; its `log.Exit` exits without a stack trace and `log.Fatal`
  with one.)
- Use the non-formatting call when there is nothing to format.
- See `errors.md` for what and when to log.

## Command-line interfaces (B#complex-clis)

- For subcommands, the documents suggest `github.com/google/subcommands` as the simplest
  option, with cobra as the common richer alternative. This is a recommendation, not a ruling;
  follow what the project has already chosen.
- With cobra, obtain the context from `cmd.Context()`; do not create a new root context.
- Subcommands do not each need their own package.
