//
//  VolumeIdentityTests.swift
//  AlexandriaTests
//

import Foundation
import Testing
@testable import Alexandria

struct VolumeIdentityTests {

	// MARK: The ladder

	@Test func uuidOutranksTheRemountURL() throws {
		let identity = VolumeIdentity(
			uuid: "0FA1-UUID",
			remountURL: URL(string: "smb://nas.local/photos")
		)
		#expect(identity == .filesystemUUID("0FA1-UUID"))
	}

	@Test func remountURLIdentifiesWhenThereIsNoUUID() throws {
		let identity = VolumeIdentity(
			uuid: nil,
			remountURL: URL(string: "smb://nas.local/photos")
		)
		#expect(identity == .remount("smb://nas.local/photos"))
	}

	@Test func absentEvidenceIsNilIdentity() throws {
		#expect(VolumeIdentity(uuid: nil, remountURL: nil) == nil)
	}

	// MARK: Canonicalization — the ratified column form

	/// User and password strip (the share is the identity, not the account),
	/// and the whole form case-folds.
	@Test func remountFormStripsTheUserAndFoldsCase() throws {
		let identity = VolumeIdentity(
			uuid: nil,
			remountURL: URL(string: "SMB://Ari:hunter2@NAS.Local/Photos")
		)
		#expect(identity == .remount("smb://nas.local/photos"))
	}

	@Test func remountFormDropsTrailingSlashes() throws {
		let identity = VolumeIdentity(
			uuid: nil,
			remountURL: URL(string: "smb://nas.local/photos/")
		)
		#expect(identity == .remount("smb://nas.local/photos"))
	}

	/// A remounting URL without a scheme and host identifies nothing — the
	/// volume stays unidentified rather than minting a junk identity.
	@Test func schemelessRemountURLIsNoIdentity() throws {
		#expect(VolumeIdentity(uuid: nil, remountURL: URL(string: "/just/a/path")) == nil)
	}

	/// The destructive case the round review caught: a file: URL's host is ""
	/// (non-nil), and accepting it mints an identity that IS the mount path —
	/// remount at "/Volumes/X 1" and the volume re-mints, orphaning its
	/// files. Better unidentified (NULL, never collides) than that.
	@Test func fileURLIsNoIdentity() throws {
		#expect(VolumeIdentity(uuid: nil, remountURL: URL(string: "file:///Volumes/X")) == nil)
		#expect(VolumeIdentity(uuid: nil, remountURL: URL(string: "smb:///share")) == nil)
	}

	/// The ratified form is portless: a spelled-out port must not mint a
	/// second identity for the same share.
	@Test func remountFormDropsThePort() throws {
		let identity = VolumeIdentity(
			uuid: nil,
			remountURL: URL(string: "smb://nas.local:445/photos")
		)
		#expect(identity == .remount("smb://nas.local/photos"))
	}

	/// The form is built from decoded components: percent-encoding never
	/// reaches the identity (encoded and plain spellings agree), and
	/// non-ASCII names genuinely case-fold.
	@Test func remountFormDecodesBeforeFolding() throws {
		let encoded = VolumeIdentity(
			uuid: nil,
			remountURL: URL(string: "smb://nas/Fotos%C3%84rchiv/")
		)
		#expect(encoded == .remount("smb://nas/fotosärchiv"))
	}

	// MARK: The column form — self-describing by shape

	@Test func rawValueRoundTripsBothRungs() throws {
		let uuid = VolumeIdentity.filesystemUUID("0FA1-UUID")
		let remount = VolumeIdentity.remount("smb://nas.local/photos")
		#expect(VolumeIdentity(rawValue: uuid.rawValue) == uuid)
		#expect(VolumeIdentity(rawValue: remount.rawValue) == remount)
	}

	@Test func rawValueDispatchesByShape() throws {
		#expect(VolumeIdentity(rawValue: "0FA1-UUID") == .filesystemUUID("0FA1-UUID"))
		#expect(VolumeIdentity(rawValue: "nfs://host/export") == .remount("nfs://host/export"))
	}
}
