---
name: aposd-review
description: Audit a codebase (or one path, or the current diff) against John Ousterhout's "A Philosophy of Software Design" - deep modules, information hiding, layering, error handling, comments, naming, consistency. Produces a graded report with file:line evidence and a concrete refactoring direction for every problem found. Use when asked for a design review, complexity or architecture audit, code-smell / red-flag check, "is this code up to standard", or how to simplify or refactor a module.
argument-hint: "[path | diff | empty = whole repo]"
---

# APoSD design review

Review target: `$ARGUMENTS` (empty = whole repository, a path = that subtree, `diff` = the
uncommitted/branch changes plus the modules they touch).

The standard being applied is the book's single criterion: **complexity is anything about the
structure of the code that makes it hard to understand and modify.** It shows up as change
amplification, cognitive load, and unknown unknowns, and it is caused by dependencies and
obscurity. Every finding must trace back to one of those; anything that does not is taste, and
stays out of the report.

Two reference files hold the substance. Read both before reviewing:

- `references/principles.md` - the book's concepts and actionables (the "what good looks like").
- `references/red-flags.md` - the catalog used during review: for each smell, what it looks like
  in code, when it is a false positive, and how to fix it.

## How to judge (read this first)

These rules are where this review differs from a generic lint pass, and where reviews most
often go wrong.

- **Size is not a smell.** Never flag a method, class or file for length alone, and never
  recommend a split just to make things smaller. The book's position is that developers split
  too much; a long method with a simple signature that reads top to bottom is deep, which is
  good. The fix for many findings is to *merge*, not split.
- **You are the instrument.** You are reading this code for the first time, which is exactly
  the perspective the book cares about. While reading, note every moment where your first guess
  about a name or call was wrong, where you had to open an implementation to understand a call
  site, or where you had to flip between two places to follow one idea. Those moments are the
  primary evidence; the catalog only helps you name them.
- **Weight by how often code is touched.** Complexity nobody has to work in costs little. A
  mild problem in a file changed weekly outranks an ugly corner untouched for two years.
- **Check callers before judging an interface.** "Shallow", "special-purpose", "pass-through"
  and "overexposed" are claims about how a module is used. Search for the call sites first.
- **Consistency outranks a better idea.** Do not recommend replacing an established project
  convention with a nicer one. Flag *inconsistency* (two ways of doing the same thing), and
  recommend converging on the one already dominant.
- **Translate, don't transliterate.** The book's examples are Java/C++. Apply the idea in the
  idiom of the language at hand (error values in Go, `Result` in Rust, modules and functions
  instead of classes, plain data records are fine as data). Structure a framework mandates is
  not the project's design decision; judge what the project controls.
- **Every principle has a "taking it too far".** Each catalog entry lists its guard. Apply it
  before reporting.

## Workflow

### 1. Map the system

Before judging anything, understand what the system is and how it is laid out.

- Read the README, CLAUDE.md, any design notes or architecture docs, and the build/lint config
  (this tells you which conventions are already enforced).
- List the modules/packages and the entry points. Identify the layers.
- Exclude generated code, vendored dependencies, build output, lockfiles and fixtures.
- If it is a git repository, find where the work actually happens:
  `git log --since="12 months ago" --name-only --format= | sort | uniq -c | sort -rn | head -40`
  Files that keep changing together in the same commits are candidates for information leakage.

### 2. Decide what to read

- Small codebase (roughly under 50 source files): read all of it.
- Larger: read the public interface of every module, then read in full the churn hotspots, the
  largest modules, and the modules most depended on. Then trace two or three representative
  operations end to end (a request, a command, a pipeline run) through every layer. In a
  well-layered system the abstraction changes at each call; if it looks the same from one layer
  to the next, that is a finding.
- `diff` scope: read the changed files and enough of their surroundings to answer the book's
  question for changes - does the system now have the structure it would have had if it had
  been designed with this change in mind, or was the change bolted on?

Keep track of what you did not read. The report states its coverage honestly.

### 3. Review through the lenses

Work through the lenses in `references/red-flags.md`. Each has one driving question:

| Lens | Driving question |
|---|---|
| A. Module depth and interfaces | Is each interface much simpler than what it hides, and is the common case trivial to use? |
| B. Information hiding | Does each design decision live in exactly one place? |
| C. Layers and abstractions | Does each layer add a different abstraction, or just forward? |
| D. Together or apart | Is related code together and unrelated code apart, or was it split by size or by step order? |
| E. Errors and special cases | How many places must handle an exceptional condition, and could it be zero? |
| F. Comments and documentation | Can a module be used from its declarations and comments alone? |
| G. Names | Does each name create an accurate picture, used the same way everywhere? |
| H. Consistency and obviousness | Is a first-time reader's quick guess correct? |
| I. Strategic health | Is the design getting better or worse with each change, and can it be refactored safely? |

Record strengths as well as problems. A review that only lists faults gives the team no idea
what to preserve.

### 4. Verify, then cut

For each candidate finding:

1. Confirm it against the code: open the file, check the callers, check the guard.
2. State the concrete cost as a scenario: "to add a new export format you must edit these four
   files", "a caller cannot know the buffer must be flushed without reading `store.py`". If you
   cannot write that sentence, drop the finding.
3. Merge repeated instances into one finding with a count and two or three examples.

Prefer ten findings you would defend to forty you would not. Nits that a formatter or linter
would catch get one line in total, not one each.

### 5. Rate

Severity of a finding = how bad the symptom is x how often the code is touched.

- **High** - unknown unknowns (a change can silently break something nobody could know about),
  or heavy change amplification / cognitive load in frequently modified code.
- **Medium** - clear cost, but localized or in code that changes occasionally.
- **Low** - real but cheap to live with; fix when next in the file.

Rate each lens **Strong / Adequate / Weak / Not assessed**, then give an overall verdict:

- **Up to standard** - no High findings; no lens Weak.
- **Up to standard with reservations** - isolated High findings or one or two Weak lenses,
  with a sound overall structure.
- **Below standard** - High findings in the core or hot paths, or structural problems
  (leakage, shallow layering) that recur across the codebase.

The verdict is a judgment; justify it in one or two sentences rather than by counting.

### 6. Report

Reply with the report in this shape. Write it for an engineer who has not read the book: name
the red flag, but explain the cost in plain terms.

```markdown
# Design review: <scope>

**Verdict:** <Up to standard | Up to standard with reservations | Below standard> - <why, 1-2 sentences>
**Coverage:** <what was read in full, what was sampled, what was skipped>

## Scorecard
| Lens | Rating | Why |
|---|---|---|
| A. Module depth and interfaces | ... | one line |
| ... | | |

## What is working
- <strength, with file reference - things to keep doing>

## Findings
### F1. <short title> - High - <red flag name> (APoSD ch. N)
- **Where:** `path/file.ext:line` (+ N similar: `a.ext:10`, `b.ext:42`)
- **What:** <what the code does today>
- **Cost:** <the concrete scenario: what is hard to change or impossible to know>
- **Fix:** <the target design; sketch the interface after the change when it helps>
- **Effort / risk:** <S | M | L> - <test coverage of the affected code, blast radius>
- **Caveat:** <only if there is a legitimate reason it may be this way>

### F2. ...

## Refactoring roadmap
1. **Quick wins** - <findings that are small, local and safe>
2. **Structural** - <ordered so each step leaves the system working; note prerequisites such as tests>
3. **Needs a decision** - <findings whose fix depends on intent only the team knows>

## Open questions
- <things the code could not tell you that would change a finding>
```

Only write the report to a file if asked.

### 7. After the report

Reviewing does not include changing code. Offer to implement the roadmap, and when asked to:

- Make sure tests cover the area first; without them structural change is unsafe. For a bug,
  write the failing test before the fix.
- For any change to an interface, sketch two genuinely different designs and compare them on
  ease of use for callers before picking one ("design it twice").
- Write the interface comment before the body. If it will not come out short and complete, the
  design is not right yet.
- Take one finding at a time, follow the conventions already in the codebase, and update the
  comments that the change invalidates.
