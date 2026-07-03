# Coding Guidelines

Guidelines for writing Go in this repo. The goals, in priority order:

- **Atomic** — each piece does one thing.
- **Testable** — each piece can be tested without standing up the whole system.
- **Boring** — the next reader (human or model) understands it at a glance.

Most of these are derived from patterns already in `internal/catalog` and
`internal/domain`. A few call out places those packages should be *tightened*.
When the two conflict, this doc wins.

---

## 0. Organize by feature/concern, not by technical kind

This is the top-level rule; everything else lives inside it.

**Packages are the real unit of organization in Go, and they should be drawn
around a feature or domain concern — not around what kind of thing the code
is.** Prefer `catalog`, `importer`, `watcher`, `xmp` over `models`, `services`,
`controllers`, `helpers`, `utils`. The kind-based layout causes name stutter
(`models.Asset` + `service.AssetService`), pushes unrelated code together, and
invites circular dependencies. This is the consensus Go convention (Ben
Johnson's *Standard Package Layout*; the community's near-universal warning
against `utils`/`common`/`models` packages).

**Never create a package named `utils`, `common`, `base`, `helpers`, or
`models`.** If a function has "no obvious home," that's a sign the package
boundaries are wrong, not that it needs a junk drawer. (A package-*private*
`helpers.go` file inside a cohesive package — like `catalog/helpers.go` — is
fine. A top-level `helpers` *package* is not. The distinction is file vs.
package.)

### Files within a package are free — split them by concern

Go compiles every file in a package as a single unit, so file boundaries carry
*zero* semantic weight. That means:

- **Don't dump every type into one `types.go`.** Put a type in the file named
  for its concern, next to the code that operates on it. `asset.go` holds the
  `Asset` type and its methods; `source.go` holds `Source`; and so on.
- Our current `domain/types.go` (300 lines: assets, sources, tags, collections,
  settings, keybinding constants, filters, patches) should be split into
  `asset.go`, `source.go`, `collection.go`, `tag.go`, `settings.go`, etc. Same
  package, same types, easier to navigate. No code changes, pure file moves.
- `catalog` already does this well at the file level: `asset_repo.go`,
  `source_repo.go` are concern-split. Keep that.

### When is a shared "types" package OK?

Your `internal/domain` package is the *sanctioned* exception, and it's worth
understanding why so the exception isn't abused:

- Ben Johnson's layout explicitly puts **shared domain types in a dependency-free
  root package**. `domain` fills exactly that role: plain structs, enums, typed
  errors, tiny constructors — and it imports nothing but stdlib. Keep it that
  way (see §6).
- A type belongs in `domain` **only if it is truly used across many packages**
  (`Asset`, `Source`, `Tag` — referenced by catalog, importer, watcher, UI).
- A type used by essentially one package belongs **with that package**, not in
  `domain`. Example: `AssetFilter` and `AssetPatch` are query/persistence
  concerns — they exist to drive `catalog` queries. They'd read better in
  `catalog` than in `domain`. Apply the test: *"is this genuinely global, or
  just currently parked here?"*

So: shared data types → one dependency-free `domain` package, split into
concern files. Everything else → its feature package.

---

## 1. Separate pure computation from orchestration

The rule that drives testability.

**Pure computation** = "give me a path (or bytes), get back facts." No channels,
no DB, no goroutines, no logging. Hashing, MIME sniffing, metadata extraction,
thumbnailing are all pure input→output transforms.

**Orchestration** = wiring: walking directories, pushing values through
channels, calling the database, coordinating goroutines, deciding error policy.

Orchestration code *calls* pure functions; it does not *contain* the logic. This
is exactly your `file_helpers` instinct, generalized.

### The file-helpers example

Inline hashing — you can't verify the hash without running the whole pipeline:

```go
// in the pipeline walk loop
h := xxhash.New()
f, _ := os.Open(path)
io.Copy(h, f)
f.Close()
hash := h.Sum64()
```

Extracted into `file_helpers` — feed a file, assert the hash:

```go
package filehelpers

// PartialHash returns the ingest fingerprint: xxhash of the first 64KB
// concatenated with the file size. See docs/original prd/05-ingest-pipeline.md.
func PartialHash(path string) (string, error) {
    f, err := os.Open(path)
    if err != nil {
        return "", err
    }
    defer f.Close()
    info, err := f.Stat()
    if err != nil {
        return "", err
    }
    return hashReader(f, info.Size())
}

// hashReader is the pure core: no filesystem, testable with a bytes.Reader.
func hashReader(r io.Reader, size int64) (string, error) {
    h := xxhash.New()
    if _, err := io.CopyN(h, r, 64*1024); err != nil && err != io.EOF {
        return "", err
    }
    fmt.Fprintf(h, "%d", size)
    return fmt.Sprintf("%x", h.Sum64()), nil
}
```

The pipeline stage becomes one line — `hash, err := filehelpers.PartialHash(path)`
— and the interesting logic has a home where it's tested in isolation.

**Pattern: push the real work down to `(inputs) → (outputs, error)`, then split
off an even smaller core that takes an `io.Reader` instead of a path.** The
reader version needs no temp files to test. Everything that inspects a file —
`PartialHash`, `SniffMIME`, `Stat`, `ExtractMetadata` — lives in `file_helpers`,
not in the importer.

---

## 2. No package-level mutable state

Package-level `var`s holding changing data leak between calls and between tests,
and block concurrent use.

Avoid (current `importer.go`):

```go
var entryMap = make(map[string]AssetDetails) // shared across every Run()
```

Own state in a struct or a local passed explicitly:

```go
func (imp *Importer) Run(ctx context.Context, job ImportJob) (ImportResult, error) {
    known := map[string]domain.FileStat{} // local to this run
    ...
}
```

Package-level `const`, and the `sync.Once` logger in `logger.go`, are fine —
they hold no per-operation state.

---

## 3. Depend on interfaces at boundaries, not concrete types

`catalog/interfaces.go` already does this: repositories are interfaces,
`SQLiteAssetRepo` is one implementation. Code that reads/writes assets accepts
`catalog.AssetRepository`, never `*sql.DB`.

```go
type Importer struct {
    Assets  catalog.AssetRepository
    Sources catalog.SourceRepository
    Dups    catalog.DuplicateRepository
    Log     *log.Logger
}
```

A test injects a fake repo, or the real SQLite repo against an in-memory DB
(`testutil.NewTestDB`), without the importer knowing which.

Don't invent an interface for something with one implementation and no test seam
(see §9). Interfaces earn their place at I/O boundaries; elsewhere they're
ceremony.

---

## 4. Return errors; don't print or swallow them

Current importer does `kind, _ := filetype.MatchFile(...)` and returns `0` from
`computeHash` on failure. Both hide problems the pipeline must report.

- A function that can fail returns `error`. The caller decides.
- Never `_ =` an error unless a comment explains why it's safe.
- Use the typed errors in `domain/errors.go` and check with `errors.As`, as the
  tests do.
- Log with the injected `*log.Logger`, not `fmt.Println`. Orchestration picks the
  level (warn for expected per-file failures, error for unexpected); pure helpers
  just return the error and stay silent.

---

## 5. Keep `domain` dependency-free

`domain` is plain data: structs, enums, typed errors, tiny pure constructors
(`SetOpt`, `ClearOpt`). No database, filesystem, or logging imports. Every
package depends on `domain`; `domain` depends on nothing but stdlib. That
property is the whole reason a shared types package is safe here — protect it.

---

## 6. Pipeline stages: worker logic separate from channel plumbing

A stage is *(a) a transformation* and *(b) the goroutine/channel machinery that
runs it*. Keep them apart.

```go
// (a) directly testable: one file in, one file out.
func hashOne(sf ScannedFile) (HashedFile, error) {
    h, err := filehelpers.PartialHash(sf.AbsPath)
    return HashedFile{ScannedFile: sf, PartialHash: h}, err
}

// (b) plumbing: fan-out, ctx cancellation, error channel. Dull, tested once.
func hashStage(ctx context.Context, in <-chan ScannedFile, out chan<- HashedFile, errc chan<- ImportError) {
    for sf := range in {
        if ctx.Err() != nil {
            return
        }
        hf, err := hashOne(sf)
        if err != nil {
            errc <- ImportError{Path: sf.AbsPath, Stage: "hashing", Err: err}
            continue
        }
        out <- hf
    }
}
```

Test `hashOne` with a file and an assertion. Don't re-test the `for range`
plumbing for every stage — it's the same shape each time.

---

## 7. Use the standard library instead of hand-rolling

- `filepath.Join(a, b)` — not `fmt.Sprintf("%s/%s", a, b)` (breaks on Windows,
  double slashes, trailing separators).
- `filepath.WalkDir` — not hand-written recursive `os.ReadDir`; it handles
  recursion, ordering, and error propagation.
- `context.Context` as the first arg of anything cancellable or I/O-bound — the
  repos already take it; the importer must too.

---

## 8. Every non-trivial function leaves one test behind

Following the existing `catalog_test.go` style:

- External test package (`package foo_test`) — exercise the public surface.
- Plain `t.Fatalf` assertions. No test framework, no mock library.
- Use `testutil` builders (`NewTestDB`, `NewTestSource`, `NewTestAsset`) and
  extend them rather than re-deriving fixtures.
- Pure helpers (`PartialHash`, `SniffMIME`) get a direct unit test against a
  fixture file in `tst/data`. This is the payoff of §1: a three-line test.

Trivial one-liners don't need a test. A parser, a branch, a hash, or anything on
the data-integrity path does.

---

## Quick checklist for a new file

1. Is this package named for a *feature/concern*, not a technical kind
   (`utils`, `models`, `helpers`)?
2. Am I putting a type in the file for its concern, not a catch-all `types.go`?
3. Does this type belong in `domain` (truly global) or with its one package?
4. Could this logic be a pure `(input) → (output, error)` helper? If so, extract it.
5. Any package-level mutable `var`? Remove it.
6. Depending on `*sql.DB` where a `catalog` interface would do?
7. Ignoring an `error` with `_`? Justify or return it.
8. Does `domain` still import nothing but stdlib?
9. One small test that fails if I break the logic?

## Sources

- Ben Johnson, *Standard Package Layout* — https://medium.com/@benbjohnson/standard-package-layout-7cdbc8391fc1
- *Let the domain guide your application structure* — https://rednafi.com/go/app-structure/
- *A Collection of Structuring Go* — https://ldej.nl/post/structuring-go/
- *How to structure Go code* — https://developer20.com/how-to-structure-go-code/
