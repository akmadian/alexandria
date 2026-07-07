# Coding Guidelines

Guidelines for writing Go in this repo. The goals, in priority order:

- **Atomic** — each piece does one thing.
- **Testable** — each piece can be tested without standing up the whole system.
- **Boring** — the next reader (human or model) understands it at a glance.

Most of these are derived from the `internal/domain`, `internal/catalog`
(ports), and `internal/sqlite` (adapter) packages. Where a rule cites a
refactor "we did," that change is already in the tree — treat it as the
reference, not a proposal.

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
boundaries are wrong, not that it needs a junk drawer. The same applies at the
*file* level: a cohesive file still deserves a name for its concern, not
`helpers.go`. We renamed `catalog/helpers.go` → `sqlite/marshal.go` because its
contents were all "SQLite value marshaling" — a real concern with a real name.
If you can't name a file better than `helpers`, its contents probably don't
belong together.

### Files within a package are free — split them by concern

Go compiles every file in a package as a single unit, so file boundaries carry
*zero* semantic weight. That means:

- **Don't dump every type into one `types.go`.** Put a type in the file named
  for its concern, next to the code that operates on it, *including its errors*:
  `SourceOfflineError` lives in `source.go`, `ErrKeybindingConflict` in
  `keybindings.go`. Only generic, cross-cutting errors (`NotFoundError`,
  `ValidationError`) stay in `errors.go`.
- We did exactly this to `domain`: the old 300-line `types.go` + `errors.go`
  became `asset.go`, `source.go`, `collection.go`, `tag.go`, `duplicate.go`,
  `asset_group.go`, `settings.go`, `opt.go`, `keybindings.go`, and a lean
  `errors.go`. Same package, same types — pure navigation win.
- `sqlite` splits by concern at the file level too: `asset_repo.go`,
  `source_repo.go`, `marshal.go`. Keep that.

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
  `domain`. We moved `AssetFilter` and `AssetPatch` out of `domain` into
  `catalog` for this reason — they're the query DTOs the repository interface
  takes, not global nouns. (They still use `domain.Opt`/`domain.FileType`, which
  *are* global.) Apply the test: *"is this genuinely global, or just currently
  parked here?"*

So: shared data types → one dependency-free `domain` package, split into
concern files. Everything else → its feature package.

### Name adapters by their dependency; keep the domain concept free of I/O

A package named for a domain concept (`catalog`) should hold the *concept*, not
a database driver. When a package starts filling up with SQL strings and row
scanning, that's a leak: the name promises an idea, the contents deliver an
adapter. Split by dependency (Ben Johnson's layout):

- **`catalog`** — the *ports*: the repository interfaces (`AssetRepository`, …).
  Imports only `context` + `domain`. No SQL. This is the seam every consumer and
  test depends on.
- **`sqlite`** — the *adapter*: `AssetRepo`, `SourceRepo`, the SQL, the row
  scanning, the value marshaling. Implements `catalog`'s interfaces
  *structurally* — it doesn't even need to import `catalog`.
- **`main`** wires them: `var assets catalog.AssetRepository = &sqlite.AssetRepo{DB: db}`.

The payoff shows up at the call site: `catalog.SQLiteAssetRepo` (stutters, and
lies about being the concept) became `sqlite.AssetRepo` (says exactly what it
is). A second backend later is just a new package beside `sqlite`; nothing else
moves.

---

## 1. Separate pure computation from orchestration — accept `io.Reader`, return a struct

The rule that drives testability, and the Go-idiomatic shape for it.

**Pure computation** = bytes in, facts out: hashing, MIME sniffing, metadata
extraction. **Orchestration** = opening files, walking directories, channels,
DB, error policy. Orchestration *opens the file once and hands the bytes (or a
reader) to* the pure functions; it never buries the logic in the walk loop.

The idiom comes straight from the stdlib — `image.Decode(r io.Reader)`,
`go-exif` taking an `io.ReadSeeker`: **a decoder/extractor accepts a reader and
returns a concrete struct; the caller owns the file handle.**

```go
// orchestration owns the open; reads the head once, feeds both cheap consumers.
f, err := os.Open(path)
// ... handle err, defer f.Close()
head, _ := io.ReadAll(io.LimitReader(f, 64*1024))

hash := partialHash(head, size)   // pure: bytes in, string out
kind := filetype.Match(head)      // existing dep: bytes in, type out
```

```go
// pure core — testable with bytes.NewReader / a []byte literal, no temp file.
func partialHash(head []byte, size int64) string {
    h := xxhash.New()
    h.Write(head)
    fmt.Fprintf(h, "%d", size)
    return fmt.Sprintf("%x", h.Sum64())
}
```

**Transport vs. format are orthogonal.** `io.Reader` abstracts *where the bytes
come from* (file, socket, buffer) — the universal part. The decoder still owns
*what the bytes mean* (PNG vs. JPEG) — that stays format-specific. A reader
papers over source differences, never format differences.

**Accept the weakest reader interface the operation needs.** Hashing streams
forward → `io.Reader`. EXIF jumps around an offset table → `io.ReadSeeker`
(which is also why it can't run off a raw network stream — you'd buffer into a
`bytes.Reader` first; local files are seekable for free). The signature
documents the capability actually required.

### Corollary: the file inspector is not a package

The code that inspects files is a few pure functions living *with the concern
that owns them*, not a `file_helpers`/`probe`/`fileutil` grab-bag:

- **Hashing** → an unexported func in `importer` (it *is* the hasher stage).
- **MIME/type** → `domain.Classify` (the file-type registry — the single source
  of truth for extension→MIME/FileType; see `domain/filetype.go`).
- **Metadata (EXIF/XMP)** → its own `metadata` package **when it's actually
  built**, shaped `metadata.Extract(r io.ReadSeeker, mime string) (Metadata, error)`.

A package you hand a *path* to is `utils` in disguise, and it re-opens the file
once per call. We deleted the empty `file_helpers` package for exactly this
reason — it was a package that shouldn't exist.

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

`catalog/interfaces.go` holds the ports; `sqlite.AssetRepo` is one
implementation (§0). Code that reads/writes assets accepts
`catalog.AssetRepository`, never `*sql.DB` or `*sqlite.AssetRepo`.

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
- Log with the shared logger (below), not `fmt.Println`. Orchestration picks the
  level (warn for expected per-file failures, error for unexpected); pure helpers
  just return the error and stay silent.

### Logging

One shared logger, using `charmbracelet/log`'s default-logger convention — no
custom logging package (`SetDefault` + the default *is* the reusable instance,
the same shape as stdlib `slog`):

- **Configured once in `main`** via `log.SetDefault(...)` (`ReportCaller` and
  timestamps on). App code then calls the package-level `log.Info/Warn/Error`
  from anywhere — nothing to import or thread.
- **Components take an injected `*log.Logger` field** (main passes the default
  in) so a test can hand them a quiet logger. Libraries never reach for the
  global themselves.
- We use charm *directly*, not as an `slog` handler, because routing through
  `slog` misattributes `ReportCaller` (extra call frames).

**Log comprehensively — a running system must narrate itself.** This is an
expectation, not a nicety: when you add a flow, you add its logging in the same
change. Under-logging a new path (a silent ingest, an untraced state change) is a
review defect, the same as a missing test. The system runs headless behind a UI,
so the log is often the only window into what the engine did and why. What MUST be
logged, by level:

- **`Info` — milestones, results, and state transitions. A clean run still tells
  its story at this level.** Every one of these earns a line:
  - *Lifecycle boundaries:* a service/watcher/loop starting and stopping, a source
    subscribed, a run beginning.
  - *Workflow results:* the outcome of a unit of work, carrying its verdict/counts/
    ids — `import finished … added=N updated=N missing=N`, a single-file
    `ingested file … verdict=move asset=…`, `marked asset missing …`, a relink/
    merge. If work completed and nothing was logged, that's the defect.
  - *Significant state transitions:* connectivity online↔offline, mode
    events↔polling, degrade-to-polling, overflow→reconcile.
- **`Debug` — the per-item / per-event play-by-play.** Each event received, each
  path armed/rearmed/graduated, each skip (ignored/unsupported), each probe tick.
  Verbose by design; the dev harness runs at this level to watch the engine work.
- **`Warn` — expected, recoverable per-item failures** (a file skipped after a
  stage error → still a DLQ row; §6 error policy).
- **`Error` — the rare serious failures** (a whole reconcile failed, a sanctioned
  write failed).

**Keep each line readable — comprehensive is about coverage, not verbosity per
line:**

- One line per event, short imperative message; put the detail in key/values.
- Attach only the few key/values that matter (`"path", p, "verdict", v, "asset",
  id`, `"err", err`). Never dump whole structs or the `ExtendedMetadata` map.
- Prefer a subsystem prefix on the message so a mixed stream is greppable
  (`"watcher: source offline"`, `"write.persist: …"`).
- `ReportCaller` + timestamps are the ceiling of *decoration*. Resist adding more
  formatting — spend the effort on coverage and good key/values instead.

The balance to strike: `Info` should read like a coherent narrative of what
happened (milestones + results), `Debug` fills in every step for when that
narrative isn't enough. Choosing silence over a milestone line is the mistake this
section exists to prevent.

---

## 5. Keep `domain` dependency-free

`domain` is plain data: structs, enums, typed errors, tiny pure constructors
(`SetOpt`, `ClearOpt`). No database, filesystem, or logging imports. Every
package depends on `domain`; `domain` depends on nothing but stdlib. That
property is the whole reason a shared types package is safe here — protect it.

### Methods on domain structs: rich but pure — no Active Record

Domain structs *should* carry behavior — but only behavior computable from their
own fields. This is what keeps `domain` dependency-free while still letting the
types be useful.

**Encouraged** (pure — reads only the struct's own fields):

```go
func (a *Asset) IsRaw() bool                 { return a.FileType == FileTypeRaw }
func (a *Asset) NeedsRethumbnail(h string) bool { return a.ThumbnailPath == nil || a.PartialHash != h }
func (s *Source) Resolve(rel string) string  { return filepath.Join(s.BasePath, rel) }
```

**Forbidden** — the moment a method needs a DB handle, filesystem, or logger, it
does not belong on the struct:

```go
func (a *Asset) Save() error      // needs *sql.DB → domain would import the driver
func (a *Asset) LoadTags() error  // same
```

That's the **Active Record** pattern, and it's exactly the coupling we removed by
splitting `catalog`/`sqlite`. Persistence stays in the repository:
`assets.Update(ctx, a)`, never `a.Save()`. The test: *does this method need
anything but the struct's own fields?* No → method. Yes → repo or pipeline.

(Note: Go's "anemic model" worry doesn't apply here. Data structs + pure methods
+ side effects at the edges is idiomatic Go; Active Record is the anti-pattern,
not the absence of it.)

---

## 6. Pipeline stages: worker logic separate from channel plumbing

A stage is *(a) a transformation* and *(b) the goroutine/channel machinery that
runs it*. Keep them apart.

```go
// (a) directly testable: one file in, one file out.
func hashFile(fsys fs.FS, scanned scannedFile) (string, error) {
    // opens, reads the head, calls partialHash (§1)
}

// (b) plumbing: range the input, run the transform, forward. Dull, tested once.
func (pipe *pipeline) hash(ctx context.Context, in <-chan *pipelineItem, out chan<- *pipelineItem) error {
    for item := range in {
        hash, _, err := hashAndHead(pipe.fsys, item.scanned)
        if err != nil {
            item.rejected = true
            item.addError("hash", "read_failed", err.Error())
        } else {
            item.hash = hash
        }
        if err := pipe.emit(ctx, out, item); err != nil { // emit handles cancellation
            return err
        }
    }
    return nil
}
```

Test the transform (`hashAndHead`) with a file and an assertion. Don't re-test
the `for range` plumbing for every stage — it's the same shape each time.

### File layout for a multi-stage pipeline

Files are free (§0), so give each stage its own file rather than one grab-bag.
`internal/importer` is the reference: **one `stage_<name>.go` per step**
(`stage_scan.go`, `stage_hash.go`, `stage_match.go`, `stage_extract.go`,
`stage_thumb.go`, `stage_write.go`), with the channel wiring and run-level state
in `pipeline.go` and the item that threads through them in `item.go`. This is a
*file* split, not a *package* split: the stages share a wide mutable surface (the
`pipeline` struct and the item), which is exactly what a single package's
unexported state is for. Do **not** promote each stage to its own subdirectory —
a subdir is a separate Go package, which would force exporting that shared
surface and invite an import cycle (see §0). Reach for a sub-package only when a
stage grows a genuinely independent, narrow-interface concern.

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

Following the existing `sqlite/sqlite_test.go` style:

- External test package (`package foo_test`) — exercise the public surface.
- Plain `t.Fatalf` assertions. No test framework, no mock library.
- Use `testutil` builders (`NewTestDB`, `NewTestSource`, `NewTestAsset`) and
  extend them rather than re-deriving fixtures.
- Pure functions (`partialHash`, `metadata.Extract`) get a direct unit test
  against a `[]byte` literal or a fixture file in `tst/data`. This is the payoff
  of §1: a three-line test.

Trivial one-liners don't need a test. A parser, a branch, a hash, or anything on
the data-integrity path does.

### Where tests and fixtures live

Go's toolchain dictates this — it's not a style choice:

- **Unit tests co-locate** with the code as `*_test.go` in the *same directory*.
  `go test` finds them by that suffix, and `go build` excludes them from the
  binary automatically — the suffix does what a Java `src/test` root does, so no
  separate test directory is needed or possible for unit tests.
- **`package foo_test` vs `package foo`.** Default to the external
  `foo_test` package (tests only the public API — the discipline a separate test
  source root gives you in other languages). Drop to internal `package foo` in a
  `_test.go` file only when you must reach an unexported symbol.
- **Fixtures go in `testdata/`.** The go tool treats `testdata` as magic: it's
  ignored for builds/package matching, and `go test` runs with the working
  directory set to the package dir, so `os.Open("testdata/x.jpg")` just works.
  (We renamed the ad-hoc `tst/` → repo-root `testdata/`. Prefer a per-package
  `testdata/` once a package owns its fixtures.)
- **A separate directory is only for integration/e2e tests** that belong to no
  single package — those get a top-level `test/` or an `internal/e2e` package.

---

## 9. Name things in full — abbreviations are a readability tax

**Spell names out. Do not abbreviate or truncate identifiers.** A name is read
far more often than it is typed; shaving four characters off `metadata` to write
`md` saves the author a moment and costs every future reader (human or model) a
re-derivation. This repo treats truncated names as a defect, not a style
preference.

Concretely:

- `extractedMetadata`, not `md`. `scannedFile` variables are `scanned`, not
  `sf`. `relativePath`, not `relPath`/`fp`/`p`. `handler`, not `h`. `pattern`,
  not `pat`. `verdict` (the matrix decision), not `act`. `pipelineItem`, not
  `it`. `waitGroup`/`group`, not `wg`/`eg`. `importErrors`, not `errs`.
- Loop bodies get real names too: `for _, session := range sessions`, not
  `for _, s := range sessions`.
- A name colliding with an imported package is not a licence to abbreviate —
  pick a fuller, more specific name (`extractedMetadata` beside the `metadata`
  package; `openedCatalog` beside a `catalog` variable). The collision is
  telling you the short name was ambiguous anyway.

**The only allowed short names** are genuinely universal conventions, and
nothing else earns an exception by analogy:

- Loop indices `i`, `j`, `k` — and only as bare integer indices.
- The stdlib-idiom trio `err`, `ctx`, `ok`, plus `id`, `db`, `tx`.
- **Method receivers**, which Go convention keeps short (`func (r *AssetRepo)`,
  `func (imp *Importer)`). Keep them 1–4 chars and consistent per type. This is
  the *receiver* only — a local variable of the same type is spelled in full.

If you find yourself wanting a fifth exception, the answer is no: write the
word. Struct fields, function parameters, and locals are all in scope — see
`internal/importer/pipeline.go` and `internal/sqlite/import_repo.go` for the
reference style.

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
10. Is every name spelled in full (no `md`/`sf`/`fp`/`it`), except the sanctioned
    short set (`i/j/k`, `err`/`ctx`/`ok`/`id`/`db`/`tx`, short method receivers)?

## Sources

- Ben Johnson, *Standard Package Layout* — https://medium.com/@benbjohnson/standard-package-layout-7cdbc8391fc1
- *Let the domain guide your application structure* — https://rednafi.com/go/app-structure/
- *A Collection of Structuring Go* — https://ldej.nl/post/structuring-go/
- *How to structure Go code* — https://developer20.com/how-to-structure-go-code/
