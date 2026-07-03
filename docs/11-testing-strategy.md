# Testing Strategy

## Philosophy

Test behaviour, not implementation. A test that verifies "importing a folder results in the correct assets in the catalog" is durable. A test that verifies "the hasher function was called with these exact arguments" is brittle — it breaks on any internal refactor even if the behaviour is unchanged.

Tests should:
- Be fast enough to run on every save during development
- Be deterministic — no flakiness from timing, random data, or external state
- Fail clearly — the failure message should tell you what went wrong without a debugger
- Cover the behaviour that matters: ingest pipeline correctness, catalog query correctness, command undo/redo correctness

Tests should not:
- Mock the database (use real in-memory SQLite)
- Test private functions (test the public interface)
- Duplicate the implementation in assertions ("assert that function X was called" — assert on the outcome instead)

---

## Test file organisation

Go test files (`.go`) live alongside the source they test — this is required by Go for access to unexported symbols in the same package. Test fixture files (sample images, XMP files, etc.) live in a single top-level `testdata/` directory, never mixed with source code.

```
alexandria/
  internal/
    ingest/
      importer.go
      importer_test.go      -- tests for the ingest package
    catalog/
      asset_repo.go
      asset_repo_test.go
  testdata/                 -- all fixture files, one location
    images/
      sample.jpg            -- small valid JPEG (keep under 100KB)
      sample.png
      sample_with_exif.jpg  -- JPEG with populated EXIF
      sample_no_exif.jpg    -- JPEG with no EXIF
    raw/
      sample.arw            -- small valid Sony RAW (or DNG if smaller)
    video/
      sample.mp4            -- short valid video (a few seconds)
    psd/
      sample.psd            -- small valid PSD with embedded composite
    xmp/
      with_rating_4.xmp     -- XMP sidecar with rating=4, label=Red
      with_keywords.xmp     -- XMP sidecar with dc:subject populated
      empty.xmp             -- minimal valid XMP
    documents/
      sample.pdf
```

**Fixture file size:** Keep fixtures as small as possible while still being valid files that exercise real parsing code. A 100KB JPEG is sufficient to test EXIF extraction. A multi-second 4K ProRes file is not needed to test video thumbnail generation — a 1-second 240p MP4 will do.

---

## testutil package

`internal/testutil/` provides shared helpers used across all test packages. Every test package imports testutil freely. Testutil is only imported in test code (enforced by the `_test` build tag on its files).

### NewInMemoryDB

The single most important helper. Creates a fresh in-memory SQLite database, runs all migrations, and registers cleanup.

```
testutil.NewInMemoryDB(t *testing.T) *sql.DB
```

Every test that needs database access calls this. The returned DB is:
- Fresh — no rows, no state carried from other tests
- Migrated — current schema, ready to use
- Auto-closed — `t.Cleanup` handles `db.Close()` so tests cannot leak connections

**Never mock SQLite.** The point of using SQLite is that it's fast and reliable enough to use directly in tests. Mocking it would test your mock, not your queries.

### NewTempSource

Creates a temporary directory, copies the specified fixture files into it, and returns the directory path. The directory is automatically cleaned up after the test via `t.TempDir()`.

```
testutil.NewTempSource(t *testing.T, fixtures []string) string
// fixtures: paths relative to testdata/, e.g. "images/sample.jpg"
```

### NewTestImporter

Constructs an `Importer` with a real DB, real hasher, and stub thumbnailer/extractor. Used for integration tests of the full ingest pipeline.

```
testutil.NewTestImporter(t *testing.T, db *sql.DB) *ingest.Importer
```

### Stub implementations

Stub implementations of platform interfaces, used in place of the real OS-dependent ones:

```
testutil.StubThumbnailer    -- writes a 1×1 pixel PNG placeholder; thumbnailing succeeds
testutil.StubMetadataExtractor -- returns minimal metadata; no real file parsing
testutil.MockFileWatcher    -- allows tests to push FileEvents manually
testutil.MockVolumeMonitor  -- allows tests to push VolumeEvents manually
```

The stub thumbnailer writes a real (tiny) PNG file to the output path so assertions about `ThumbnailPath` work correctly.

### Assertion helpers

```
testutil.FindAssetByFilename(t, db, filename) *domain.Asset
testutil.AssertAssetCount(t, db, expected int)
testutil.AssertDuplicateCount(t, db, expected int)
```

These query the database directly and fail the test with a clear message if the assertion doesn't hold.

---

## Table-driven tests

The standard Go pattern for testing multiple scenarios of the same behaviour. Each test case is a struct describing inputs and expected outputs. The test function iterates cases and runs each as a named sub-test.

```
func TestImportPipeline(t *testing.T) {
    cases := []struct {
        name         string
        fixtures     []string
        wantAdded    int
        wantSkipped  int
        wantErrors   int
        wantAssets   []struct { filename string; fileType domain.FileType }
    }{
        {
            name:      "basic jpeg import",
            fixtures:  []string{"images/sample.jpg"},
            wantAdded: 1,
            wantAssets: []{ {filename: "sample.jpg", fileType: domain.FileTypeImage} },
        },
        {
            name:      "raw file is indexed correctly",
            fixtures:  []string{"raw/sample.arw"},
            wantAdded: 1,
            wantAssets: []{ {filename: "sample.arw", fileType: domain.FileTypeRaw} },
        },
        {
            name:        "reimport unchanged file is skipped",
            fixtures:    []string{"images/sample.jpg"},
            -- run import twice, verify second run skips
            wantSkipped: 1,
        },
        {
            name:      "multiple file types in one import",
            fixtures:  []string{"images/sample.jpg", "raw/sample.arw", "video/sample.mp4"},
            wantAdded: 3,
        },
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            db      := testutil.NewInMemoryDB(t)
            srcDir  := testutil.NewTempSource(t, tc.fixtures)
            imp     := testutil.NewTestImporter(t, db)

            result, err := imp.Run(context.Background(), ingest.ImportJob{
                SourcePath: srcDir,
                BatchSize:  50,
            })

            -- always log result on failure for easy diagnosis
            t.Logf("result: added=%d updated=%d skipped=%d errors=%d",
                result.Added, result.Updated, result.Skipped, len(result.Errors))
            for _, e := range result.Errors {
                t.Logf("  error [%s] %s: %v", e.Stage, e.Path, e.Err)
            }

            require.NoError(t, err)
            assert.Equal(t, tc.wantAdded, result.Added)
            assert.Equal(t, tc.wantSkipped, result.Skipped)
            assert.Equal(t, tc.wantErrors, len(result.Errors))

            for _, want := range tc.wantAssets {
                asset := testutil.FindAssetByFilename(t, db, want.filename)
                t.Logf("asset: id=%s filename=%s fileType=%s thumbnail=%v",
                    asset.ID, asset.Filename, asset.FileType, asset.ThumbnailPath)
                assert.Equal(t, want.fileType, asset.FileType)
            }
        })
    }
}
```

**Sub-test isolation:** Each `t.Run` gets its own fresh database and temp directory. Tests are fully isolated — no shared state between cases.

**`t.Logf` for diagnostics:** Log values are only printed when the test fails or when `-v` is passed. This keeps normal test output quiet while making failures easy to diagnose. Always log the full result struct and any errors before making assertions.

---

## What to test, and in what order

Build tests in this order. Each layer depends on the one before it.

### 1. testutil package (first — everything else uses it)

Test `NewInMemoryDB` by verifying it creates a schema with the expected tables. Test `NewTempSource` by verifying files are copied correctly. These tests are trivial but they validate the foundation.

### 2. Repository tests

For each repository, test the basic CRUD operations against a real in-memory SQLite database:

```
TestAssetRepository_CreateAndGet
TestAssetRepository_List_FilterByFileType
TestAssetRepository_List_FilterByRating
TestAssetRepository_List_FilterByColorLabel
TestAssetRepository_SoftDelete
TestAssetRepository_FindByHash
TestAssetRepository_BulkPatch
TestAssetRepository_FindBySourcePath
TestAssetRepository_MarkOfflineBySource
TestAssetRepository_ListKnownFiles

TestSourceRepository_FindByFilesystemUUID
TestTagRepository_Tree
TestCollectionRepository_GetAssets_SmartCollection
```

These catch schema bugs, SQL query errors, and index effectiveness. They are the lowest-risk tests to write first because they require no mocking.

### 3. Ingest pipeline tests

Test the full pipeline end-to-end with real fixture files:

```
TestImportPipeline_BasicJPEG
TestImportPipeline_RawFile
TestImportPipeline_VideoFile
TestImportPipeline_MultipleFileTypes
TestImportPipeline_DuplicateLoggedAndBothIngested
TestImportPipeline_MovedFileRelinked_MetadataPreserved
TestImportPipeline_MovedFileDifferentName_NotAutoRelinked
TestImportPipeline_Idempotency_UnchangedFile
TestImportPipeline_Reimport_ChangedFile
TestImportPipeline_CorruptFileDoesNotAbortPipeline
TestImportPipeline_Cancellation
TestImportPipeline_EmptyDirectory
```

These are the highest-value tests. The ingest pipeline is the riskiest code and exercises the most dependencies.

### 4. Command and undo stack tests

Test the command pattern and undo/redo behaviour with mock commands:

```
TestStack_ExecuteAndUndo
TestStack_ExecuteAndRedo
TestStack_RedoClearedAfterNewExecute
TestStack_MaxSizeRespected
TestStack_UndoEmptyStack_NoError

TestSetRatingCommand_ExecuteAndUndo_SingleAsset
TestSetRatingCommand_ExecuteAndUndo_BulkMixedPreviousValues
TestSetColorLabelCommand_ExecuteAndUndo
TestSoftDeleteCommand_ExecuteAndUndo
```

### 5. XMP sync tests

Test with real XMP fixture files:

```
TestXMPSync_ReadRatingFromSidecar
TestXMPSync_ReadLabelFromSidecar
TestXMPSync_ReadKeywordsFromSidecar
TestXMPSync_NormalisesLabelCase
TestXMPSync_ConflictResolution_XMPWins
TestXMPSync_ConflictResolution_CatalogWins
TestXMPSync_KeywordsMerge_DoesNotRemoveUserTags
TestXMPSync_SkipsWriteProtectedFormats
TestXMPSync_NoSidecarFile_Skips
TestXMPSync_CorruptSidecar_LogsAndContinues
```

### 6. Watcher service tests

Test with the mock file watcher and volume monitor:

```
TestWatcher_FileCreated_TriggersIngest
TestWatcher_FileModified_TriggersReingest
TestWatcher_FileDeleted_MarksAssetMissing
TestWatcher_FileRenamed_UpdatesAssetPath
TestWatcher_Debounce_MultipleEventsCollapsed
TestWatcher_VolumeMount_ReconnectsKnownSource
TestWatcher_VolumeUnmount_MarksSourceOffline
TestWatcher_SourceOffline_DoesNotCrash
```

---

## Running tests

```bash
# run all tests
go test ./...

# run all tests, verbose output
go test ./... -v

# run a specific package
go test ./internal/ingest/... -v

# run a specific test
go test ./internal/ingest/... -v -run TestImportPipeline

# run a specific sub-test
go test ./internal/ingest/... -v -run TestImportPipeline/basic_jpeg_import

# run with race detector (catches data races — run this before committing)
go test ./... -race
```

The `-race` flag is important for Alexandria because the ingest pipeline is heavily concurrent. Run it regularly, not just on CI.

---

## What not to test

- Private/internal functions (test the public behaviour they produce)
- The Wails app layer (it's thin; test the underlying services)
- Platform-specific implementations (FileWatcher, DriveIdentifier) — these require real OS APIs and are better covered by manual testing and CI on the target platform
- Exact log output — log messages change, behaviour doesn't
- Timing-sensitive behaviour — write tests that are deterministic regardless of CPU speed

---

## CI considerations

Tests should run in CI on every push. The test suite runs on macOS and Linux (primary platforms). Windows CI is secondary.

The in-memory SQLite approach means no CI infrastructure is needed for database tests — everything runs in the process.

The real thumbnailer and extractor implementations shell out to `ffmpeg`/`ffprobe`, `exiftool`, and friends as subprocesses. In CI these are a package-manager install (`brew install` / `apt-get install`) — no cgo toolchain setup. Tests for the real implementations skip (`t.Skip`) when a required tool is not on PATH, so the suite still passes in minimal environments using the stubs.
