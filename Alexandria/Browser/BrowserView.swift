//
//  BrowserView.swift
//  Alexandria
//
//  The sources sidebar (browser round, 2026-09-11): an intent-writer to
//  the hub and a reader of one field. Clicking a row calls setSource; the
//  highlight derives from hub.source — Source itself is the List selection
//  value, so the sidebar owns no selection state and no second vocabulary
//  for it (round review, finding 4). Tree content is the browser's private
//  observation — not shared position, so not the hub's.
//
//  Deliberately absent, per the round's markers: collections (no schema),
//  per-node counts (lens-coupled semantics unsettled), offline gray/badge
//  (waits on a volume-presence mechanism), expansion persistence
//  (viewpoint round), import-history browsing.
//

import SwiftUI
import GRDB
import Logging

/// View-local observable owning the browser's tree observation: bound to
/// one catalog at a time, rebinding if a different catalog arrives (the
/// open-catalog round's edge, guarded now). The previous tree stands until
/// the next delivery lands — same never-clear discipline as the hub.
@MainActor @Observable
final class BrowserModel {
	private(set) var tree = BrowserTree.empty

	@ObservationIgnored private var observation: AnyDatabaseCancellable?
	@ObservationIgnored private var boundCatalog: ObjectIdentifier?
	@ObservationIgnored private let log = Logger(label: "browser")

	func start(catalog: Catalog) {
		let identity = ObjectIdentifier(catalog.databaseWriter)
		guard identity != boundCatalog else { return }
		boundCatalog = identity
		observation?.cancel()
		log.debug("tree observation start")
		observation = ValueObservation
			.tracking { try BrowserTree.fetch($0) }
			.removeDuplicates()
			.start(in: catalog.reader) { [weak self] error in
				self?.log.error("tree observation stopped (dead until rebind)", metadata: [
					"error": "\(error)",
				])
			} onChange: { [weak self] tree in
				self?.log.trace("tree delivered", metadata: [
					"volumes": "\(tree.volumes.count)",
				])
				self?.tree = tree
			}
	}
}

struct BrowserView: View {
	@Environment(\.catalog) private var catalog
	@Environment(CatalogViewState.self) private var viewState

	@State private var model = BrowserModel()
	@State private var folderFilter = ""
	/// Ephemeral disclosure state, default expanded — collapsed is the
	/// exception worth remembering. Keyed by id, so it survives the filter
	/// rebuilding node values. Persistence is viewpoint territory.
	@State private var collapsedVolumes: Set<Identifier<Volume>> = []
	@State private var collapsedFolders: Set<Identifier<Folder>> = []

	var body: some View {
		List(selection: selection) {
			Section("Sources") {
				Label("All Assets", systemImage: "photo.on.rectangle")
					.tag(Source.library)
				Label("Previous Import", systemImage: "clock.arrow.circlepath")
					.tag(Source.latestImport)
			}
			Section("Folders") {
				ForEach(filteredTree.volumes) { volume in
					DisclosureGroup(isExpanded: expansion(of: volume.id, in: $collapsedVolumes)) {
						ForEach(volume.roots) { root in
							folderRows(root)
						}
					} label: {
						Label(volume.name, systemImage: "externaldrive")
					}
				}
			}
		}
		.listStyle(.sidebar)
		.searchable(text: $folderFilter, placement: .sidebar, prompt: "Filter Folders")
		.task(id: ObjectIdentifier(catalog.databaseWriter)) {
			model.start(catalog: catalog)
		}
	}

	private var filteredTree: BrowserTree {
		model.tree.filtered(by: folderFilter)
	}

	private var filterActive: Bool {
		!folderFilter.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
	}

	/// One folder row, recursively: a leaf is a plain row; a parent is a
	/// disclosure the VIEW owns state for — while a filter is live every
	/// surviving node is a match or a match's ancestor, so disclosure is
	/// forced open and the match is actually revealed (round review,
	/// finding 1).
	private func folderRows(_ node: BrowserTree.FolderNode) -> AnyView {
		let row = Label(node.name, systemImage: "folder").tag(Source.folder(node.id))
		guard !node.children.isEmpty else { return AnyView(row) }
		return AnyView(DisclosureGroup(
			isExpanded: filterActive
				? .constant(true)
				: expansion(of: node.id, in: $collapsedFolders)
		) {
			ForEach(node.children) { child in
				folderRows(child)
			}
		} label: {
			row
		})
	}

	/// The selection derives from hub.source — the sidebar holds no copy,
	/// and Source is the selection value directly. A source with no row (a
	/// specific historical import) simply highlights nothing; a nil set
	/// (List clearing selection on empty-area clicks) changes nothing —
	/// the source stands, and the highlight re-derives on the next render.
	private var selection: Binding<Source?> {
		Binding(
			get: { viewState.source },
			set: { source in
				if let source {
					viewState.setSource(source)
				}
			}
		)
	}

	private func expansion<Subject>(
		of id: Identifier<Subject>, in collapsed: Binding<Set<Identifier<Subject>>>
	) -> Binding<Bool> {
		Binding(
			get: { !collapsed.wrappedValue.contains(id) },
			set: { expanded in
				if expanded {
					collapsed.wrappedValue.remove(id)
				} else {
					collapsed.wrappedValue.insert(id)
				}
			}
		)
	}
}

#Preview {
	let catalog = try! Catalog(DatabaseQueue())
	return BrowserView()
		.environment(\.catalog, catalog)
		.environment(CatalogViewState(catalog: catalog))
}
