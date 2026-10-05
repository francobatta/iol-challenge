# Red-flag catalog

One section per review lens. Each entry gives **Signals** (what it looks like in code),
**Guard** (when it is not a problem - check this before reporting) and **Fix** (the direction
to recommend). Entries marked with a star are the book's fourteen named red flags; the rest
are problems the text describes without a formal box.

Signals find candidates. A finding exists only once you can state what is hard to change or
impossible to know because of it.

## Contents

- A. Module depth and interfaces
- B. Information hiding
- C. Layers and abstractions
- D. Together or apart
- E. Errors and special cases
- F. Comments and documentation
- G. Names
- H. Consistency and obviousness
- I. Strategic health
- Cheap probes

---

## A. Module depth and interfaces (ch. 4, 6, 8, 19.6)

### Shallow Module *
- **Signals:** A wrapper whose honest documentation would be longer than its body. A function
  that renames one operation and is called from one place. A class with one public method and
  no state worth hiding. An interface comment that can only be complete by describing the
  implementation. To do one ordinary thing, callers must assemble several objects.
- **Guard:** A small unit is fine when it hides a real decision (a format, an invariant, a
  policy likely to change) or gives a name to a non-obvious idea. Some shallowness is
  unavoidable; report it when it is a pattern or sits on a busy path.
- **Fix:** Inline it into its caller. Merge it with the classes it is always used with. Raise
  the level of the interface: one operation that performs the whole task instead of one per
  step.

### Classitis
- **Signals:** Many tiny classes/files, each individually trivial, where following one feature
  means opening eight of them. Builder/factory/wrapper stacks needed for the common case.
- **Guard:** The count alone is not the problem; the test is whether the total interface a
  developer must learn is larger than the functionality warrants.
- **Fix:** Combine classes that are always used together and share knowledge; make the common
  combination the default.

### Overexposure *
- **Signals:** The usual call needs arguments, flags or setup steps that are nearly always the
  same value. Required parameters where a default would be right. A caller must learn a rarely
  used feature in order to use the common one. Callers supply values they have no good way to
  know (protocol version, buffer size).
- **Guard:** If different callers truly need different values, the parameter belongs in the
  interface.
- **Fix:** Default it; derive it from information the module already has; move rare options to
  a separate method or optional configuration so common callers never see them.

### Special-purpose interface
- **Signals:** Methods named after one caller's feature (`backspace`, `deleteSelection`,
  `exportForInvoiceScreen`). Many public methods with exactly one call site each. A low-level
  module whose signatures use types from a higher layer (UI, HTTP, a specific job).
- **Guard:** Over-generalizing is also a failure: if callers need loops and glue to do today's
  job, the interface went too far the other way.
- **Fix:** Replace the family of special methods with a few general ones (`delete(start, end)`),
  and move the caller-specific logic up into the caller.

### Complexity pushed upward
- **Signals:** A long list of configuration parameters, especially ones users cannot sensibly
  choose. Every caller repeats the same preparation or cleanup around a call. Exceptions thrown
  because the author was unsure what to do. Callers must call methods in a fixed sequence.
- **Guard:** Pull complexity down only when it belongs to the module's job, simplifies many
  callers, and makes the interface simpler. Otherwise it becomes leakage in the other direction.
- **Fix:** Have the module compute the value, perform the sequence, or handle the condition
  itself; keep an override for the rare caller who needs it.

### Exposed internals (getters, setters, returned representations)
- **Signals:** A method returns the internal mutable collection or map. A behaviour-bearing
  class has a get/set pair for most of its fields. Callers reach through an object
  (`a.getB().getC().doThing()`).
- **Guard:** Plain data records, DTOs and value types exist to carry fields; that is their job.
- **Fix:** Offer the operation the caller actually wants (`getParameter(name)` rather than
  `getParams()`), so the representation can change without touching callers.

### False abstraction
- **Signals:** The interface looks simple, but correct use depends on something it does not
  say: when data is durable, whether a call blocks, ordering requirements, a performance cliff.
  Callers are observed reading or duplicating the implementation's knowledge.
- **Fix:** Put the important fact in the interface, explicitly and documented. Hiding it
  creates obscurity, not simplicity.

---

## B. Information hiding (ch. 5)

### Information Leakage *
- **Signals:** The same format, schema, protocol, key name, magic value or business rule is
  understood in more than one module. A reader and a writer of one format live in separate
  classes. Parallel `switch`/`if` chains over the same enum in several files. Files that almost
  always change together in git history. Adding one variant (a status, a message type, a
  product kind) requires edits in a list of places.
- **Guard:** Extracting the knowledge into a new class helps only if that class can offer a
  simple interface. If it would expose most of the knowledge again, nothing is gained.
- **Fix:** Give the decision one owner. Either merge the modules that share it, or move it into
  one module the others call. Where a cross-cutting decision is unavoidable, see the
  cross-module documentation entry under lens F.

### Temporal Decomposition *
- **Signals:** Modules named for steps in a sequence (`Reader`, `Parser`, `Processor`,
  `Writer`; `step1`, `step2`; `load`/`validate`/`transform`) where several steps need the same
  knowledge. Callers must invoke A then B then C, passing intermediate results along.
- **Guard:** Stages that use different information are a sound decomposition. Order has to
  appear somewhere; the objection is only to order dictating module boundaries when knowledge
  is shared across them.
- **Fix:** Regroup by knowledge: one module that owns the format and does both reading and
  writing; one call that performs the whole operation.

### Leakage inside a class
- **Signals:** Instance fields read and written by nearly every method; private helpers that
  only make sense given the internals of other helpers.
- **Fix:** Narrow the places each field is used; have each private method own one piece of
  knowledge.

---

## C. Layers and abstractions (ch. 7)

### Pass-Through Method *
- **Signals:** The body is a single call forwarding the same arguments to a method with the
  same or a near-identical signature. Controller -> service -> manager -> repository chains
  where the middle layers add nothing. A class most of whose public methods forward.
- **Guard:** A dispatcher that chooses among targets adds function. Several implementations of
  one interface legitimately share signatures. A forwarding method that also adds real behaviour
  (validation with consequences, transactions, caching) is not a pass-through.
- **Fix:** One of three: let callers use the lower class directly; redistribute so each class
  has a distinct responsibility; or merge the classes.

### Decorator and wrapper build-up
- **Signals:** Wrappers that reimplement an entire interface to add one small behaviour; several
  stacked to get a normal configuration.
- **Fix:** Ask, in order: can the behaviour go into the underlying class (if nearly everyone
  wants it, yes)? Into the one use case that needs it? Into an existing wrapper? Or stand alone
  without wrapping at all?

### Interface mirrors implementation
- **Signals:** The public API has the same shape as the storage: line-oriented methods over
  line-oriented storage, database row shapes passed straight through to API consumers,
  transport types appearing in domain code.
- **Fix:** Design the interface for what callers do, and let the difference from the
  representation be the functionality the module provides.

### Pass-through variable
- **Signals:** A parameter appears in three or more signatures down a call chain and is used
  only at the bottom. Adding a new piece of global information means editing many signatures.
- **Guard:** A context object can decay into a grab bag with thread-safety problems; globals
  prevent multiple instances and hurt testing.
- **Fix:** Put it in an object the top and bottom already share, or in a context object held by
  the major objects and passed only through constructors. Keep its contents immutable.

---

## D. Together or apart (ch. 9)

### Repetition *
- **Signals:** The same non-trivial block, or nearly the same, in several places; the same
  handler or cleanup at multiple return points.
- **Guard:** One or two repeated lines rarely justify extraction. An extracted helper that
  needs many parameters or must return several pieces of state is not an improvement.
- **Fix:** Extract when the snippet is long and the helper's signature is simple. Otherwise
  restructure so the code runs in one place (a single exit path, a single top-level handler).
  Repetition usually means an abstraction is missing; name it.

### Special-General Mixture *
- **Signals:** A general mechanism containing branches for particular callers (`if type ==
  INVOICE`). A utility or infrastructure module importing domain types. A generic component
  that gains a method each time a new feature is added elsewhere.
- **Fix:** Keep the mechanism general and push the specifics up to the caller, typically by
  having callers supply objects or callbacks the mechanism treats opaquely (the book's
  `History` class holding caller-defined `Action` objects, with grouping policy decided by the
  UI layer).

### Conjoined Methods *
- **Signals:** You cannot understand one method without reading another. A helper called from
  exactly one place that takes many parameters or shares state with its caller by convention. A
  long procedure chopped into `doPart1`, `doPart2` which must run in order.
- **Fix:** Join them back together. A long method that reads straight through is better than
  fragments that must be read side by side.

### Split by size rather than by meaning
- **Signals:** Many short methods each used once; reading one operation means following a chain
  of jumps; a file per tiny class.
- **Guard:** A split is good when the child is a cleanly separable subtask - readable without
  knowing the parent, and the parent readable without reading the child - which usually means
  the child is somewhat general-purpose.
- **Fix:** Recombine until each unit does one thing completely.

### One method, several unrelated jobs
- **Signals:** Boolean or mode parameters that select between behaviours; an interface comment
  that needs "and" several times; callers that each use a different slice of the behaviour.
- **Fix:** Split into methods that each have a simpler interface than the original, such that
  most callers need only one of them.

---

## E. Errors and special cases (ch. 10)

### Unnecessary exceptions
- **Signals:** Errors raised for conditions callers routinely do not care about: removing
  something already absent, a range partly out of bounds, closing twice. Call sites that wrap
  the call only to ignore the error. Methods with many distinct error types.
- **Fix:** Redefine the operation so the condition is ordinary: "ensure X does not exist",
  "return the part of the range that exists". The interface gets simpler and the method deeper.

### Scattered handling
- **Signals:** The same `catch` block or error-check-and-return repeated at every call site.
  More error-handling lines than normal-path lines. Per-call retry logic.
- **Fix:** *Mask* at the bottom when a low-level module can deal with it for everyone (retry
  inside the client). *Aggregate* at the top when the response is uniform: let errors carry a
  descriptive message and propagate to one handler in the request loop or dispatcher.

### Hand-recovering from the unrecoverable
- **Signals:** Every allocation, config read or startup step individually checked and
  propagated, with no real recovery anywhere up the chain.
- **Guard:** Depends on the application. Libraries should not terminate their host; systems
  whose value is fault tolerance must recover.
- **Fix:** A single fail-fast helper that reports clearly and aborts.

### Special-case state
- **Signals:** A flag or null meaning "no X" with checks for it throughout (`hasSelection`,
  `if (list == null)`). Branches for empty, first or last element that mirror the general path.
- **Fix:** Choose a representation in which the normal code covers the case: an empty
  selection rather than no selection, an empty collection rather than null, a sentinel node.
  Invariants remove special cases.

### Errors hidden that callers need (the "too far" case)
- **Signals:** Catch-and-continue around operations whose failure matters to the caller: lost
  messages, failed writes, partial results returned as complete. Empty catch blocks.
- **Fix:** Expose it. What is important must be visible, even at the cost of a larger interface.

### Untested error paths
- **Signals:** Recovery code with no tests exercising it. "Code that hasn't been executed
  doesn't work."
- **Fix:** Fewer handlers (above) and tests for the ones that remain.

---

## F. Comments and documentation (ch. 12, 13, 15, 16)

Follow the documentation conventions of the language and project (docstrings, Javadoc, JSDoc,
rustdoc, godoc). Judge content, not format.

### Missing interface documentation
- **Signals:** Public modules, classes and functions with no statement of the abstraction they
  provide. Fields and parameters whose units, bounds (inclusive or exclusive), null meaning,
  ownership or invariants cannot be determined without reading usages. Side effects,
  exceptions and preconditions unstated. You had to read a body to learn how to call it.
- **Guard:** The book asks for comments on essentially everything public; where the project's
  convention is lighter, hold the line at "could I use this without reading its body?".
- **Fix:** A short interface comment written from the caller's point of view.

### Comment Repeats Code *
- **Signals:** The comment restates the adjacent line or the entity's own name
  (`// increment i`, "Gets the normalized resource name"). Test: could someone who has not seen
  the rest of the code have written this comment from the line beside it?
- **Fix:** Delete it, or replace it with what is not visible: units, the reason, the intent.
  Use different words from the name.

### Implementation Documentation Contaminates Interface *
- **Signals:** An interface comment describing internal data structures, private helpers,
  algorithms or internal constants that a caller does not need.
- **Fix:** Move those details to comments inside the body. If the interface cannot be described
  without them, the module is shallow; report that instead.

### Hard to Describe *
- **Signals:** A method or variable needs a long, qualified comment to be described accurately;
  the description is full of "except when" and "unless".
- **Fix:** This is a design finding, not a documentation finding. Look for the simpler
  definition: split the variable, redefine the method, remove the special cases.

### Imprecise or mis-aimed variable comments
- **Signals:** "Current offset" (current as of what?). Comments that narrate how a variable is
  updated rather than what it represents.
- **Fix:** Describe the noun: what it holds and what is always true of it.

### Undocumented cross-module decisions
- **Signals:** A change in one place requires matching changes elsewhere, and nothing at the
  place a developer would start says so. Protocols implemented on two sides with no shared
  description. Rationale that exists only in commit messages or a ticket.
- **Fix:** One authoritative note at the natural starting point (the enum, the schema, or a
  design-notes file with labelled sections) and one-line pointers from the other sites.

### Stale, distant or duplicated comments
- **Signals:** Comments contradicting the code. One block at the top of a function describing
  every phase in detail. The same explanation copied to several places. A call site explaining
  what the callee does internally.
- **Fix:** Put each comment at the narrowest scope covering what it describes; state each
  decision once; reference rather than copy external documentation.

---

## G. Names (ch. 14)

### Vague Name *
- **Signals:** `data`, `info`, `item`, `obj`, `tmp`, `val`, `count`, `flag`, `status`, `type`
  with nothing saying of what. `result` in a function that returns nothing. Boolean names that
  are not predicates (`blinkStatus` vs `cursorVisible`). Coordinate-like names (`x`, `y`) for
  things that are not coordinates. Catch-all nouns such as `Manager`, `Handler`, `Helper`,
  `Util`, `Processor` on types whose responsibility is otherwise unclear.
- **Guard:** Short generic names are fine when the whole lifetime is visible in a few lines
  (`i`, `j` in a short loop). The further a use is from the declaration, the more the name must
  carry. Respect the language community's norms.
- **Fix:** Name what it is specifically: `numIndexlets`, `fileBlock` vs `diskBlock`.

### Inconsistent naming
- **Signals:** One name used for two different things (the book's `block` bug: file blocks and
  disk blocks). Two or more names for the same concept in different places (`user`, `account`,
  `member`, `customer`).
- **Fix:** One term per concept, used everywhere and for nothing else; add a prefix where
  several of the same kind coexist (`srcFileBlock`, `dstFileBlock`).

### Name too specific
- **Signals:** A general operation whose parameter is named for one use (`delete(Range
  selection)`).
- **Fix:** Name it for what the code actually accepts.

### Hard to Pick Name *
- **Signals:** Names joined with `And`/`Or`, or very long compound names; an entity renamed
  repeatedly in history; a name that no longer matches what the thing does.
- **Fix:** Treat as a design signal: the entity is probably doing more than one thing or has no
  clear definition. Refactor, then the name becomes easy.

---

## H. Consistency and obviousness (ch. 17, 18)

### Inconsistent approaches
- **Signals:** Two ways of doing the same thing in one codebase: two HTTP clients, two error
  conventions, two date libraries, two config mechanisms, mixed naming styles. A newer style
  introduced without migrating the old.
- **Guard:** Dissimilar things should look different; forcing one pattern onto a case that does
  not fit is its own problem.
- **Fix:** Converge on the dominant existing convention, write it down, and enforce it with a
  formatter, linter or CI check so it does not depend on memory.

### Nonobvious Code *
- **Signals:** Behaviour that cannot be understood from a quick read. In particular:
  - *Indirect control flow* - events, callbacks, reflection, dependency-injection wiring,
    decorators or metaclasses that register things - where you cannot tell who calls a
    function or when.
  - *Generic containers* crossing a module boundary: pairs, tuples, or maps with string keys
    returned from functions, so callers write `result[0]` or `result.getKey()`.
  - *Surprises:* constructors or property reads that do I/O, start threads or mutate state;
    functions whose name promises less than they do.
  - *Declared type differs from actual type* in a way that affects behaviour.
  - *Dense formatting* that hides structure.
- **Fix:** In order of preference: remove the need for the information (simpler design);
  follow the convention readers expect; otherwise supply the information where the reader is
  (a named record type instead of a tuple; a comment on each handler saying when it is
  invoked; a note at the surprising call).

---

## I. Strategic health (ch. 3, 16, 19, 20)

### Tactical residue
- **Signals:** Density of `TODO`, `FIXME`, `HACK`, `XXX`, "temporary", "workaround".
  Commented-out code. Leftover debug output. Recent changes that each add a flag or a
  special-case branch to existing functions (`if customer == "acme"`).
- **Fix:** Point to the design change that would have absorbed those cases, and recommend it
  for the next time the area is touched.

### No safety net for refactoring
- **Signals:** Little or no automated testing around the code most in need of restructuring;
  bug-fix commits without accompanying tests.
- **Fix:** Tests come first on the roadmap for any structural finding in that area. This is a
  prerequisite, not a separate nicety.

### Implementation inheritance
- **Signals:** Deep hierarchies; subclasses reading and writing parent fields; overrides that
  only make sense with knowledge of the parent's body; changes to a base class that require
  checking every subclass.
- **Fix:** Composition with small helper classes; if inheritance stays, have the parent manage
  its own state and expose it read-only or through methods.

### Pattern for its own sake
- **Signals:** A factory with one product, a strategy with one strategy, an interface with one
  implementation and no test double, layers present because the template had them.
- **Guard:** Do not flag structure the framework requires.
- **Fix:** Remove the indirection until there is a second case that needs it.

### Unjustified performance complexity
- **Signals:** Hand-rolled caches, pooling, bit tricks or unusual structures with no
  measurement, benchmark or comment justifying them. Conversely: many shallow layers and
  repeated special-case checks on a known hot path; expensive operations (network, disk,
  allocation) inside loops where an equally simple alternative exists.
- **Fix:** Measure first. Keep optimizations that measure, remove the rest. On a real hot path,
  identify the minimum code the common case needs and design around it, with one test up front
  routing special cases elsewhere.

---

## Cheap probes

Searches that surface candidates quickly. Adapt the patterns to the language. A hit is a place
to look, not a finding.

| Looking for | Probe |
|---|---|
| Tactical residue | `TODO\|FIXME\|HACK\|XXX\|workaround\|temporary` |
| Swallowed errors | empty `catch` blocks; `except ...: pass`; `_ = err`; `.catch(() => {})` |
| Scattered handling | count of `try`/`catch` (or `if err != nil`) per file against file size; identical handler bodies |
| Exposed internals | density of `get*`/`set*` on non-record classes; methods returning a field that is a collection |
| Generic containers | `Pair<`, `Tuple`, `tuple[`, `Map<String, Object>`, `dict[str, Any]`, `[0]`/`[1]` on call results |
| Vague names | `\b(data\|info\|tmp\|temp\|obj\|val\|flag\|status\|result)\b` in declarations; types ending `Manager\|Helper\|Util\|Handler\|Processor` |
| Temporal decomposition | file/class names `*Reader`, `*Parser`, `*Loader`, `*Processor`, `*Writer`, `step\d` clustered around one data format |
| Leakage via enums | every usage of each enum or type-tag constant; several `switch`/`match` over the same one |
| Leakage via history | the churn command in SKILL.md; then pairs of files that recur in the same commits |
| Pass-through variables | a parameter name appearing in many signatures within one call chain |
| Pass-through methods | one-statement bodies that return a call with the same argument list |
| Config sprawl | size of config/option structs and env-var reads; options with no documented guidance |
| Special-case branches | comparisons against specific tenants, customers, feature names or type strings inside shared code |
| Inconsistency | more than one library for the same job in the dependency manifest |
