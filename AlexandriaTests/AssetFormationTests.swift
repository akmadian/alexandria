//
//  AssetFormationTests.swift
//  AlexandriaTests
//

import Foundation
import Testing
import GRDB
@testable import Alexandria

/// The formation engine's ratified behavior (_design/grouping.md): pairing
/// with evidence guards, sidecar attachment, transitive clusters,
/// cross-batch joins, the floor, and idempotency — against an in-memory
/// catalog through the real record + form pipeline.
struct AssetFormationTests {

	private func shot(
		at seconds: TimeInterval? = nil,
		camera: (make: String, model: String)? = nil
	) -> FileMetadata {
		var metadata = FileMetadata()
		metadata.capturedAt = seconds.map(Date.init(timeIntervalSince1970:))
		metadata.cameraMake = camera?.make
		metadata.cameraModel = camera?.model
		return metadata
	}

	private func assetAndRule(of name: String, in context: ImportContext) async throws -> (asset: String?, rule: String?) {
		let row = try await context.catalog.reader.read { database in
			try Row.fetchOne(
				database,
				sql: "SELECT asset_id, formation_rule FROM files WHERE name = ?",
				arguments: [name]
			)
		}
		let found = try #require(row)
		return (found["asset_id"], found["formation_rule"])
	}

	private func assetCount(in context: ImportContext) async throws -> Int {
		try await context.catalog.reader.read { database in
			try Int.fetchOne(database, sql: "SELECT COUNT(*) FROM assets") ?? -1
		}
	}

	@Test func rawAndRenditionFormOnePairSameFolder() async throws {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/_DSF0796.RAF"),
			context.prepared("/Volumes/Test/Shoot/_DSF0796.JPG"),
		])
		let outcome = try await context.form()
		#expect(outcome.assetsMinted == 1)
		#expect(outcome.filesFormed == 2)

		let raw = try await assetAndRule(of: "_DSF0796.RAF", in: context)
		let jpeg = try await assetAndRule(of: "_DSF0796.JPG", in: context)
		#expect(raw.asset != nil)
		#expect(raw.asset == jpeg.asset)
		#expect(raw.rule == "raw_rendition_pair")
		#expect(jpeg.rule == "raw_rendition_pair")
	}

	@Test func rawJpegAndSidecarClusterIntoOneAsset() async throws {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/_DSF0796.xmp"),
			context.prepared("/Volumes/Test/Shoot/_DSF0796.JPG"),
			context.prepared("/Volumes/Test/Shoot/_DSF0796.RAF"),
		])
		let outcome = try await context.form()
		#expect(outcome.assetsMinted == 1)
		#expect(outcome.filesFormed == 3)

		let raw = try await assetAndRule(of: "_DSF0796.RAF", in: context)
		let jpeg = try await assetAndRule(of: "_DSF0796.JPG", in: context)
		let sidecar = try await assetAndRule(of: "_DSF0796.xmp", in: context)
		#expect(raw.asset == jpeg.asset)
		#expect(sidecar.asset == raw.asset)
		#expect(sidecar.rule == "sidecar_attach")
		#expect(raw.rule == "raw_rendition_pair")
	}

	@Test func unclaimedNonSidecarsFallToTheFloor() async throws {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/lone.ORF"),
			context.prepared("/Volumes/Test/Shoot/holiday.mp4"),
		])
		let outcome = try await context.form()
		#expect(outcome.assetsMinted == 2)

		let raw = try await assetAndRule(of: "lone.ORF", in: context)
		let video = try await assetAndRule(of: "holiday.mp4", in: context)
		#expect(raw.rule == "one_asset_per_file")
		#expect(video.rule == "one_asset_per_file")
		#expect(raw.asset != video.asset)

		let kinds = try await context.catalog.reader.read { database in
			try String.fetchSet(database, sql: "SELECT kind FROM assets")
		}
		#expect(kinds == ["image", "video"])
	}

	@Test func twoRenditionsNeverPairWithoutARaw() async throws {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/export.jpg"),
			context.prepared("/Volumes/Test/Shoot/export.png"),
		])
		_ = try await context.form()
		let count = try await assetCount(in: context)
		#expect(count == 2)
	}

	@Test func lateArrivingRenditionJoinsTheExistingAsset() async throws {
		let context = try await ImportContext.make()
		// Batch 1: the raw forms as a floor singleton.
		try await context.record([context.prepared("/Volumes/Test/Shoot/_DSF0796.RAF")])
		_ = try await context.form()
		// Batch 2: the jpeg finds the formed raw and joins its asset.
		try await context.record([context.prepared("/Volumes/Test/Shoot/_DSF0796.JPG")])
		let second = try await context.form()
		#expect(second.assetsMinted == 0)
		#expect(second.filesFormed == 1)

		let raw = try await assetAndRule(of: "_DSF0796.RAF", in: context)
		let jpeg = try await assetAndRule(of: "_DSF0796.JPG", in: context)
		#expect(raw.asset == jpeg.asset)
		// Provenance is historical: the raw keeps its floor admission.
		#expect(raw.rule == "one_asset_per_file")
		#expect(jpeg.rule == "raw_rendition_pair")
		let count = try await assetCount(in: context)
		#expect(count == 1)
	}

	@Test func crossFolderPairingRequiresPositiveCorroboration() async throws {
		let context = try await ImportContext.make()
		// Stripped metadata on both sides: abstention is not corroboration —
		// the archive acid case must NOT pair.
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/2015/IMG_1234.CR2", metadata: shot(at: 1_400_000_000)),
			context.prepared("/Volumes/Test/Shoot/2024/IMG_1234.JPG"),
		])
		_ = try await context.form()
		#expect(try await assetCount(in: context) == 2)

		// Agreeing capture time IS corroboration: trip/raw + trip/jpeg pairs.
		let corroborated = try await ImportContext.make()
		try await corroborated.record([
			corroborated.prepared("/Volumes/Test/Shoot/raw/IMG_9.ARW", metadata: shot(at: 1_700_000_100)),
			corroborated.prepared("/Volumes/Test/Shoot/jpeg/IMG_9.JPG", metadata: shot(at: 1_700_000_101)),
		])
		_ = try await corroborated.form()
		#expect(try await assetCount(in: corroborated) == 1)
	}

	@Test func disagreeingEvidenceRefutesEvenInTheSameFolder() async throws {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/IMG_1.NEF", metadata: shot(at: 1_700_000_000)),
			context.prepared("/Volumes/Test/Shoot/IMG_1.JPG", metadata: shot(at: 1_700_003_600)),
		])
		_ = try await context.form()
		#expect(try await assetCount(in: context) == 2)

		let cameras = try await ImportContext.make()
		try await cameras.record([
			cameras.prepared("/Volumes/Test/Shoot/IMG_2.NEF", metadata: shot(camera: ("Nikon", "Z8"))),
			cameras.prepared("/Volumes/Test/Shoot/IMG_2.JPG", metadata: shot(camera: ("FUJIFILM", "X-T5"))),
		])
		_ = try await cameras.form()
		#expect(try await assetCount(in: cameras) == 2)
	}

	@Test func orphanSidecarStaysFormationPending() async throws {
		let context = try await ImportContext.make()
		try await context.record([context.prepared("/Volumes/Test/Shoot/ghost.xmp")])
		let outcome = try await context.form()
		#expect(outcome.assetsMinted == 0)
		#expect(outcome.filesFormed == 0)
		#expect(outcome.sidecarsPending == 1)

		let sidecar = try await assetAndRule(of: "ghost.xmp", in: context)
		#expect(sidecar.asset == nil)
		#expect(sidecar.rule == nil)
	}

	@Test func exactFormSidecarPicksItsNamedSubject() async throws {
		let context = try await ImportContext.make()
		// The pair is refuted (different cameras), so raw and jpeg hold
		// separate assets — the sidecar's exact form must pick the raw.
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/photo.raf", metadata: shot(camera: ("FUJIFILM", "X-T5"))),
			context.prepared("/Volumes/Test/Shoot/photo.jpg", metadata: shot(camera: ("Canon", "R5"))),
			context.prepared("/Volumes/Test/Shoot/photo.raf.xmp"),
		])
		_ = try await context.form()

		let raw = try await assetAndRule(of: "photo.raf", in: context)
		let jpeg = try await assetAndRule(of: "photo.jpg", in: context)
		let sidecar = try await assetAndRule(of: "photo.raf.xmp", in: context)
		#expect(raw.asset != jpeg.asset)
		#expect(sidecar.asset == raw.asset)
	}

	/// Batch invariance (ratified): a rendition corroborating two same-stem
	/// raws proves the scaffolding singletons are one work — a later pass
	/// MERGES them, exactly as an all-at-once pass would have unioned them.
	@Test func bridgingRenditionMergesScaffoldingAcrossPasses() async throws {
		let context = try await ImportContext.make()
		// Pass 1: two same-stem raws in different folders form as singletons
		// (raw↔raw never pairs directly).
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/a/IMG_7.ARW", metadata: shot(at: 1_700_000_000, camera: ("Sony", "A7IV"))),
			context.prepared("/Volumes/Test/Shoot/b/IMG_7.CR3", metadata: shot(at: 1_700_000_000, camera: ("Sony", "A7IV"))),
		])
		_ = try await context.form()
		#expect(try await assetCount(in: context) == 2)

		// Pass 2: the bridging rendition corroborates both.
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/c/IMG_7.JPG", metadata: shot(at: 1_700_000_000, camera: ("Sony", "A7IV")))
		])
		let second = try await context.form()
		#expect(second.assetsMinted == 0)
		#expect(second.filesFormed == 1)
		#expect(second.assetsAbsorbed == 1)
		#expect(try await assetCount(in: context) == 1)

		let arw = try await assetAndRule(of: "IMG_7.ARW", in: context)
		let cr3 = try await assetAndRule(of: "IMG_7.CR3", in: context)
		let jpeg = try await assetAndRule(of: "IMG_7.JPG", in: context)
		#expect(arw.asset != nil)
		#expect(arw.asset == cr3.asset)
		#expect(jpeg.asset == arw.asset)
		// Provenance is historical: the raws keep their floor admissions.
		#expect(arw.rule == "one_asset_per_file")
		#expect(cr3.rule == "one_asset_per_file")
		#expect(jpeg.rule == "raw_rendition_pair")
	}

	/// The ratified invariant itself: forming after every single file vs
	/// forming once at the end yields the identical partition of files
	/// into assets.
	@Test func batchBoundariesDoNotChangeTheOutcome() async throws {
		let paths = [
			"/Volumes/Test/Shoot/a/IMG_7.ARW",
			"/Volumes/Test/Shoot/b/IMG_7.CR3",
			"/Volumes/Test/Shoot/c/IMG_7.JPG",
			"/Volumes/Test/Shoot/a/IMG_7.xmp",
			"/Volumes/Test/Shoot/a/lone.png",
		]
		func prepared(_ context: ImportContext, _ path: String) -> PreparedFile {
			path.hasSuffix("xmp") || path.hasSuffix("png")
				? context.prepared(path)
				: context.prepared(path, metadata: shot(at: 1_700_000_000, camera: ("Sony", "A7IV")))
		}

		let allAtOnce = try await ImportContext.make()
		try await allAtOnce.record(paths.map { prepared(allAtOnce, $0) })
		_ = try await allAtOnce.form()

		let perFile = try await ImportContext.make()
		for path in paths {
			try await perFile.record([prepared(perFile, path)])
			_ = try await perFile.form()
		}

		func partition(_ context: ImportContext) async throws -> Set<Set<String>> {
			let rows = try await context.catalog.reader.read { database in
				try Row.fetchAll(database, sql: "SELECT name, asset_id FROM files WHERE asset_id IS NOT NULL")
			}
			var byAsset: [String: Set<String>] = [:]
			for row in rows {
				byAsset[row["asset_id"], default: []].insert(row["name"])
			}
			return Set(byAsset.values)
		}
		#expect(try await partition(allAtOnce) == partition(perFile))
	}

	@Test func crossFolderPairsOnCameraIdentityAlone() async throws {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/raw/IMG_5.NEF", metadata: shot(camera: ("Nikon", "Z8"))),
			context.prepared("/Volumes/Test/Shoot/jpeg/IMG_5.JPG", metadata: shot(camera: ("Nikon", "Z8"))),
		])
		_ = try await context.form()
		#expect(try await assetCount(in: context) == 1)
	}

	/// The abstain half of the acid case: one-sided evidence in the same
	/// folder is NOT refutation — the pair still forms on the stem.
	@Test func oneSidedEvidenceAbstainsInsteadOfRefuting() async throws {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/IMG_3.NEF", metadata: shot(at: 1_700_000_000, camera: ("Nikon", "Z8"))),
			context.prepared("/Volumes/Test/Shoot/IMG_3.JPG"),  // stripped
		])
		_ = try await context.form()
		#expect(try await assetCount(in: context) == 1)
	}

	/// Short-form tie-break, raw-preference branch: with the pair refuted
	/// (two assets), the sidecar must pick the raw.
	@Test func shortFormSidecarPrefersTheRawSubject() async throws {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/photo.raf", metadata: shot(camera: ("FUJIFILM", "X-T5"))),
			context.prepared("/Volumes/Test/Shoot/photo.jpg", metadata: shot(camera: ("Canon", "R5"))),
			context.prepared("/Volumes/Test/Shoot/photo.xmp"),
		])
		_ = try await context.form()

		let raw = try await assetAndRule(of: "photo.raf", in: context)
		let jpeg = try await assetAndRule(of: "photo.jpg", in: context)
		let sidecar = try await assetAndRule(of: "photo.xmp", in: context)
		#expect(raw.asset != jpeg.asset)
		#expect(sidecar.asset == raw.asset)
	}

	/// Short-form tie-break, lowest-file-id branch: no raw among the
	/// subjects, so the elder file wins.
	@Test func shortFormSidecarFallsBackToLowestFileId() async throws {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/export.jpg"),
			context.prepared("/Volumes/Test/Shoot/export.png"),
			context.prepared("/Volumes/Test/Shoot/export.xmp"),
		])
		_ = try await context.form()

		let expected = try await context.catalog.reader.read { database in
			try String.fetchOne(database, sql: """
				SELECT asset_id FROM files
				WHERE kind != 'sidecar' AND name LIKE 'export%'
				ORDER BY id LIMIT 1
				""")
		}
		let sidecar = try await assetAndRule(of: "export.xmp", in: context)
		#expect(sidecar.asset != nil)
		#expect(sidecar.asset == expected)
	}

	/// Decisions are a pure function of their input.
	@Test func formationIsDeterministic() async throws {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/_DSF0796.RAF"),
			context.prepared("/Volumes/Test/Shoot/_DSF0796.JPG"),
			context.prepared("/Volumes/Test/Shoot/_DSF0796.xmp"),
			context.prepared("/Volumes/Test/Shoot/lone.ORF"),
			context.prepared("/Volumes/Test/Shoot/ghost.xmp"),
		])
		let files = try await context.catalog.files(inImport: context.importId)

		func normalized(_ resolution: AssetFormation.Resolution) -> Set<String> {
			Set(resolution.clusters.map { cluster in
				let destination: String = switch cluster.destination {
				case .mint(let kind): "mint:\(kind)"
				case .join(let id): "join:\(id.rawValue)"
				}
				let members = cluster.members
					.map { "\($0.file.rawValue):\($0.rule)" }
					.sorted()
					.joined(separator: ",")
				return destination + "|" + members
			})
		}
		let first = AssetFormation.form(files: files)
		let second = AssetFormation.form(files: files)
		#expect(normalized(first) == normalized(second))
		#expect(first.sidecarsPending == second.sidecarsPending)
	}

	/// The pipeline step itself — read, decide, persist — exercised as
	/// ImportRun composes it, not re-implemented.
	@Test @MainActor func formAssetsComposesReadDecidePersist() async throws {
		let context = try await ImportContext.make()
		let run = ImportRun(
			id: .mint(), folderUrl: context.rootURL, rootFolderId: context.rootFolderId,
			volume: ObservedVolume(
				identity: .filesystemUUID("0FA1-BATCH-FIXTURE"), name: "Test",
				kind: .external, volumeRootURL: URL(fileURLWithPath: "/Volumes/Test")
			),
			catalog: context.catalog,
			store: ThumbnailStore(catalogDirectory: FileManager.default.temporaryDirectory),
			resuming: false
		)
		try await context.catalog.recordImportStarted(id: run.id, folderId: context.rootFolderId)
		_ = try await context.catalog.recordNewFileBatch(
			[
				context.prepared("/Volumes/Test/Shoot/_DSF0796.RAF"),
				context.prepared("/Volumes/Test/Shoot/_DSF0796.JPG"),
			],
			importId: run.id,
			rootFolderId: context.rootFolderId, rootUrl: context.rootURL
		)
		let resolution = try await run.formAssets()
		#expect(resolution.assetsMinted == 1)
		#expect(resolution.filesFormed == 2)

		let persisted = try await context.catalog.reader.read { database in
			try Int.fetchOne(database, sql: "SELECT COUNT(*) FROM files WHERE asset_id IS NOT NULL")
		}
		#expect(persisted == 2)
	}

	@Test func formationPassesAreIdempotent() async throws {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/_DSF0796.RAF"),
			context.prepared("/Volumes/Test/Shoot/_DSF0796.JPG"),
			context.prepared("/Volumes/Test/Shoot/ghost.xmp"),
		])
		_ = try await context.form()
		let second = try await context.form()
		#expect(second.assetsMinted == 0)
		#expect(second.filesFormed == 0)
		#expect(second.sidecarsPending == 1)
		#expect(try await assetCount(in: context) == 1)
	}

	@Test func metadataColumnRoundTripsForTheGuards() throws {
		var metadata = FileMetadata()
		metadata.capturedAt = Date(timeIntervalSince1970: 1_700_000_000)
		metadata.cameraMake = "FUJIFILM"
		metadata.cameraModel = "X-T5"
		metadata.iso = 400
		let json = try metadata.databaseJSON()
		let decoded = try #require(FileMetadata(databaseJSON: json))
		#expect(decoded == metadata)
	}
}
