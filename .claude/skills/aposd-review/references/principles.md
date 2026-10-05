# A Philosophy of Software Design - concepts and actionables

Digest of John Ousterhout's book (1st ed., 2018), organized for use as a review standard.
Chapter numbers are given so findings can cite them.

## The thesis

Software design is about one thing: managing complexity. The limit on what we can build is
our ability to understand it. There are two ways to fight complexity: **eliminate** it (make
code simpler and more obvious) and **encapsulate** it (modular design, so a developer faces
only a small part at a time). Design is never finished; it happens continuously as the system
evolves.

## Complexity (ch. 2)

**Definition.** Anything related to the structure of a system that makes it hard to understand
and modify. It is not size or feature count. It is judged by readers, not writers: if others
find your code complex, it is.

**Weighting.** Overall complexity is each part's complexity weighted by the time developers
spend there. Isolating complexity where it is rarely seen is nearly as good as removing it.

**Three symptoms**

| Symptom | Meaning |
|---|---|
| Change amplification | A simple change requires edits in many places. |
| Cognitive load | A developer must know a lot to make a change safely. Fewer lines is not automatically less load. |
| Unknown unknowns | It is not obvious what must be changed or known. The worst of the three: you find out from bugs. |

**Two causes**

- **Dependencies** - code that cannot be understood or changed in isolation. They cannot be
  eliminated, so make them few, simple and obvious.
- **Obscurity** - important information is not obvious (vague names, undocumented units,
  hidden dependencies, inconsistency).

**Complexity is incremental.** It accumulates from hundreds of small "no big deal" choices.
The only defence is zero tolerance for small additions.

## Mindset (ch. 3, 16, 19)

- **Working code is not enough.** Tactical programming optimizes for finishing the current
  task and leaves a little complexity behind each time. Strategic programming treats the
  design as the product; working code follows.
- **Invest continually.** Spend roughly 10-20% of development time on design improvement,
  proactively (find a clean design, write the documentation) and reactively (fix design
  problems when discovered instead of patching around them). It pays back within months.
- **When modifying code,** leave the system with the structure it would have had if designed
  with the change in mind. If you are not making the design better, you are probably making it
  worse. "Smallest possible change" is a tactical instinct.
- **Increments should be abstractions, not features.** Design an abstraction properly when you
  first need it. Test-driven development tends to pull toward feature-at-a-time hacking; unit
  tests themselves are essential because they make refactoring safe. Write the test first when
  fixing a bug.

## Modules should be deep (ch. 4)

A module is anything with an interface and an implementation: a function, class, package,
service. The interface is everything a user must know, **formal** (signatures, types) and
**informal** (behaviour, ordering constraints, side effects - only expressible in comments).

- **Deep module:** a lot of functionality behind a simple interface. Cost of a module is its
  interface; benefit is its functionality. Unix file I/O (five calls hiding a file system) and
  a garbage collector (no interface at all) are the models.
- **Shallow module:** interface nearly as complex as the implementation. It adds something to
  learn and hides little.
- **Classitis:** the belief that more, smaller classes are better. It produces many shallow
  modules and a large total interface. Java's stream wrappers are the example.
- **Abstraction** omits unimportant detail. It fails two ways: including detail that does not
  matter (higher cognitive load) or omitting detail that does (a *false abstraction*, which
  creates obscurity).
- **Make the common case simple.** An interface's effective complexity is the complexity of
  what most callers need. Provide defaults; "do the right thing" without being asked.

## Techniques for creating deep modules

**Information hiding (ch. 5).** Each module should own a few design decisions that appear in
its implementation and nowhere in its interface. *Information leakage* - the same decision
reflected in several modules - is the central red flag; back-door leakage (two modules that
both know a file format) is worse than leakage through an interface because it is invisible.
*Temporal decomposition* - structuring modules by the order things happen (read, then parse,
then write) - is a common cause. Design around knowledge, not sequence. `private` is not
information hiding if getters and setters expose the same facts. Making a class a little
larger often hides more.

**General-purpose interfaces (ch. 6).** Make modules *somewhat* general-purpose: functionality
that meets today's needs, behind an interface general enough for other uses. General
interfaces come out simpler and deeper, and keep the knowledge of specific callers out of the
module. Ask: what is the simplest interface that covers all current needs? In how many
situations will this method be used? Is it easy to use for today's needs?

**Different layer, different abstraction (ch. 7).** Each layer should offer something its
neighbours do not. Pass-through methods, decorators that mostly forward, interfaces that
mirror the internal representation, and variables threaded through many signatures all add
interface without adding function. Every design element must pay for itself by removing more
complexity than it introduces.

**Pull complexity downward (ch. 8).** It matters more that a module has a simple interface
than a simple implementation: a module has more users than developers. Do not hand hard
problems to callers through exceptions or configuration parameters you could have resolved
yourself. Limits: only when the complexity belongs to the module's job, simplifies many
callers, and simplifies the interface.

**Better together or better apart (ch. 9).** Subdividing has costs: more components, more
interfaces, glue code, separation of things that must be read together, duplication. Bring
code together when it shares information, when combining simplifies the interface, or when it
removes duplication. Keep general-purpose mechanisms apart from special-purpose uses of them;
push special-purpose code up. For methods: each should do one thing and do it completely;
length alone is never a reason to split. Split only when a cleanly separable subtask exists.

**Define errors out of existence (ch. 10).** Exception handling is a major source of
complexity and of bugs, and exceptions are part of the interface. Reduce the number of places
that must handle them, in order of preference:

1. *Define the error away* - redefine the operation so the condition is normal ("ensure it no
   longer exists" rather than "delete it"; clamp an out-of-range slice).
2. *Mask* it at a low level so callers never see it (retransmission inside the transport).
3. *Aggregate* - let many exceptions propagate to one handler near the top.
4. *Crash* on errors that are rare and not worth recovering from.

Apply the same thinking to special cases generally: design the normal path so it covers them.
Limit: an error callers genuinely need to know about must be exposed.

**Design it twice (ch. 11).** Sketch at least two substantially different designs for any
significant decision and compare them, mainly on ease of use for the layer above.

## Comments (ch. 12, 13, 15, 16)

- Code cannot be fully self-documenting: the informal half of every interface, the rationale,
  the units and the invariants exist only in comments. If users must read a method's code to
  use it, there is no abstraction.
- **Comments describe what is not obvious from the code.** They work at a different level of
  detail: *lower* for precision (units, inclusive/exclusive bounds, meaning of null, ownership
  of resources, invariants), *higher* for intuition (what this block is trying to do, and why).
  Comments at the same level as the code just repeat it.
- **Four kinds:** interface comments, data-structure member comments, implementation comments,
  cross-module comments. The first two matter most: every class, every public method, every
  field.
- **Interface comments** describe behaviour as seen by a caller: arguments, return value, side
  effects, exceptions, preconditions - and nothing about the implementation. If an interface
  comment must explain the implementation, the module is shallow.
- **Implementation comments** say *what* and *why*, not *how*.
- **Cross-module decisions** need one home (the central declaration, or a design-notes file)
  with short pointers from the places affected.
- **Write comments first**, as a design tool. A comment that has to be long, or is hard to
  write, signals a design problem while it is still cheap to fix.
- **Maintaining them:** keep comments next to the code they describe; document each decision
  once; do not leave the explanation only in a commit message; reference external docs instead
  of copying them; check the diff before committing.

## Names (ch. 14)

A name should create an accurate image of the thing, read in isolation. Names must be
**precise** (not `data`, `count`, `status`, `result` where there is no return value; booleans
as predicates) and **consistent** (one name for one concept everywhere, never reused for a
different one). Length should grow with the distance between declaration and use. Difficulty
finding a good name is a sign the thing being named is not well defined.

## Consistency and obviousness (ch. 17, 18)

- Consistency gives leverage: learn it once, recognize it everywhere. Applies to names, style,
  interfaces, patterns, invariants. Document conventions, enforce them with tools, and follow
  what is already there. A better idea is not a sufficient reason to introduce inconsistency.
- Obvious code is code where a reader's quick first guess is right. Software should be designed
  for ease of reading, not ease of writing. Things that reduce obviousness and need
  compensating documentation: event-driven control flow, generic containers such as pairs and
  tuples, declared types that differ from the allocated type, and anything that violates reader
  expectations (a constructor that starts threads).

## Trends, evaluated against complexity (ch. 19)

- **Inheritance:** interface inheritance gives leverage; implementation inheritance couples
  parent and subclasses. Prefer composition; otherwise keep parent state managed by the parent.
- **Design patterns:** good where they fit, harmful when forced.
- **Getters and setters:** shallow, and they expose implementation. Avoid exposing instance
  variables at all.

## Performance (ch. 20)

Clean design and speed are compatible; simple code is usually fast. Know which operations are
inherently expensive (network, storage I/O, allocation, cache misses) and pick naturally
efficient designs that are equally simple. Otherwise measure before changing anything, prefer
fundamental fixes (a cache, a better algorithm), and when redesigning, build around the
critical path with the special cases moved off it. Back out optimizations that do not measure.

## The fifteen principles

1. Complexity is incremental: sweat the small stuff.
2. Working code is not enough.
3. Make continual small investments to improve system design.
4. Modules should be deep.
5. Design interfaces so the most common usage is as simple as possible.
6. A simple interface matters more than a simple implementation.
7. General-purpose modules are deeper.
8. Separate general-purpose and special-purpose code.
9. Different layers should have different abstractions.
10. Pull complexity downward.
11. Define errors (and special cases) out of existence.
12. Design it twice.
13. Comments should describe things that are not obvious from the code.
14. Design for ease of reading, not ease of writing.
15. The increments of development should be abstractions, not features.

## The fourteen named red flags

Shallow Module, Information Leakage, Temporal Decomposition, Overexposure, Pass-Through
Method, Repetition, Special-General Mixture, Conjoined Methods, Comment Repeats Code,
Implementation Documentation Contaminates Interface, Vague Name, Hard to Pick Name, Hard to
Describe, Nonobvious Code. Detection and fixes are in `red-flags.md`.

## Moderation

The book's own caveat: reducing complexity is the goal, and it outranks any individual rule.
Every principle here has a point past which it does harm. If applying one does not make the
code easier to understand and change, do not apply it.
