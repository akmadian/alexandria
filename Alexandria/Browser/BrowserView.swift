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
//  Collections (collections round, chunk 4): a third section, rows tagged
//  Source.collection like everything else; the verbs ride context menus
//  and name prompts, and deleting the viewed subtree retargets to the
//  library (ruling 14, via the hub). Drag-and-drop (add, re-parent) is
//  deliberately absent: its own round, by ruling (2026-09-14).
//
//  Deliberately absent, per the rounds' markers: per-node counts
//  (lens-coupled semantics unsettled), offline gray/badge (waits on a
//  volume-presence mechanism), expansion persistence (viewpoint round),
//  import-history browsing, sidebar filter over collections (the field is
//  the FOLDER filter by prompt; widening it is a future call).
//

import AppKit
import SwiftUI
import GRDB
import Logging

private nonisolated let log = Logger(label: "browser")

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
	@State private var collapsedCollections: Set<Identifier<Collection>> = []

	/// The name prompt's subject: minting (under a parent or at the top
	/// level) or renaming. One draft field serves both.
	private enum CollectionNaming {
		case create(parent: Identifier<Collection>?)
		case rename(Identifier<Collection>)
	}
	@State private var naming: CollectionNaming?
	@State private var draftName = ""

	/// The delete confirm's payload: the subtree numbers fetched up front
	/// (ruling 8 — the confirm shows what goes).
	private struct PendingDelete {
		let id: Identifier<Collection>
		let name: String
		let collections: Int
		let memberships: Int
	}
	@State private var pendingDelete: PendingDelete?

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
			Section {
				ForEach(model.tree.collections) { root in
					collectionRows(root)
				}
			} header: {
				HStack {
					Text("Collections")
					Spacer()
					Button {
						beginNaming(.create(parent: nil))
					} label: {
						Image(systemName: "plus")
					}
					.buttonStyle(.plain)
					.help("New Collection")
				}
			}
		}
		.listStyle(.sidebar)
		.searchable(text: $folderFilter, placement: .sidebar, prompt: "Filter Folders")
		.task(id: ObjectIdentifier(catalog.databaseWriter)) {
			model.start(catalog: catalog)
		}
		.alert(namingTitle, isPresented: presented($naming), presenting: naming) { naming in
			TextField("Name", text: $draftName)
			Button(namingConfirmTitle) { commit(naming) }
			Button("Cancel", role: .cancel) {}
		}
		.alert(
			"Delete \u{201C}\(pendingDelete?.name ?? "")\u{201D}?",
			isPresented: presented($pendingDelete),
			presenting: pendingDelete
		) { pending in
			Button("Delete", role: .destructive) { perform(pending) }
			Button("Cancel", role: .cancel) {}
		} message: { pending in
			// Ruling 8: the confirm shows the subtree's numbers. Records
			// speak; assets are never touched by a collection delete.
			Text("Deletes \(pending.collections) collection(s) holding \(pending.memberships) membership(s). Assets themselves are untouched.")
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

	// MARK: Collections — rows, verbs, drops (collections round, chunk 4)

	/// One collection row, recursively — same disclosure shape as folders.
	/// No drag machinery by ruling (2026-09-14): drag-to-add and
	/// drag-to-re-parent are a future round of their own, opening on the
	/// prior-art question this chunk paid for.
	private func collectionRows(_ node: BrowserTree.CollectionNode) -> AnyView {
		let row = Label(node.name, systemImage: "rectangle.stack")
			.tag(Source.collection(node.id))
			.contextMenu {
				Button("New Collection Inside") { beginNaming(.create(parent: node.id)) }
				Button("Rename\u{2026}") {
					draftName = node.name
					naming = .rename(node.id)
				}
				Button("Move to Top Level") { move(node.id, under: nil) }
				Divider()
				Button("Delete\u{2026}", role: .destructive) { prepareDelete(node) }
			}
		guard !node.children.isEmpty else { return AnyView(row) }
		return AnyView(DisclosureGroup(
			isExpanded: expansion(of: node.id, in: $collapsedCollections)
		) {
			ForEach(node.children) { child in
				collectionRows(child)
			}
		} label: {
			row
		})
	}

	private var namingTitle: String {
		switch naming {
		case .create: "New Collection"
		case .rename: "Rename Collection"
		case nil: ""
		}
	}

	private var namingConfirmTitle: String {
		switch naming {
		case .rename: "Rename"
		default: "Create"
		}
	}

	private func beginNaming(_ intent: CollectionNaming) {
		draftName = ""
		naming = intent
	}

	private func commit(_ naming: CollectionNaming) {
		let name = draftName
		let catalog = catalog
		Task {
			do {
				switch naming {
				case .create(let parent):
					try await catalog.createCollection(named: name, under: parent)
				case .rename(let id):
					try await catalog.renameCollection(id, to: name)
				}
			} catch CollectionError.emptyName {
				// The verb's floor (ruling: empty/whitespace refused); the
				// prompt simply closes having minted nothing.
				log.info("collection name rejected: empty")
			} catch {
				log.error("collection naming failed", metadata: ["error": "\(error)"])
			}
		}
	}

	private func prepareDelete(_ node: BrowserTree.CollectionNode) {
		let catalog = catalog
		Task {
			do {
				let summary = try await catalog.subtreeSummary(of: node.id)
				pendingDelete = PendingDelete(
					id: node.id, name: node.name,
					collections: summary.collections, memberships: summary.memberships
				)
			} catch {
				log.error("subtree summary failed", metadata: ["error": "\(error)"])
			}
		}
	}

	private func perform(_ pending: PendingDelete) {
		let catalog = catalog
		let viewState = viewState
		Task {
			do {
				let deleted = try await catalog.deleteCollection(pending.id)
				viewState.collectionsWereDeleted(deleted)
			} catch {
				log.error("collection delete failed", metadata: ["error": "\(error)"])
			}
		}
	}

	private func move(_ id: Identifier<Collection>, under destination: Identifier<Collection>?) {
		let catalog = catalog
		Task {
			do {
				try await catalog.moveCollection(id, under: destination)
			} catch CollectionError.wouldCreateCycle {
				// Ruling 7: refused inside the verb; the gesture answers
				// audibly, not silently.
				NSSound.beep()
				log.info("collection move refused: would create a cycle")
			} catch {
				log.error("collection move failed", metadata: ["error": "\(error)"])
			}
		}
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

	/// isPresented for an optional payload: alerts dismiss by nilling it.
	private func presented<Subject>(_ item: Binding<Subject?>) -> Binding<Bool> {
		Binding(
			get: { item.wrappedValue != nil },
			set: { if !$0 { item.wrappedValue = nil } }
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
