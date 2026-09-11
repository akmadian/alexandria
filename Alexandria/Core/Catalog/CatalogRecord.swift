//
//  CatalogRecord.swift
//  Alexandria
//
//  Shared policy for every record type in the catalog. Conforming is what
//  pins a record to the ratified conventions — a record can't forget them,
//  because adopting the protocol IS the pinning.
//

import Foundation
import GRDB

/// A catalog row type. Carries only policy that is genuinely uniform across
/// all records (today: the timestamp format). Column naming stays explicit
/// per record via CodingKeys — a shared snake_case strategy would let
/// Columns drift from real column names and make filters silently match
/// nothing.
nonisolated protocol CatalogRecord: Codable, FetchableRecord, PersistableRecord {}

// GRDB 7's customization points are per-column FUNCTIONS; a `static var` of
// the same name silently witnesses nothing and GRDB falls back to its
// default format — the shape of the original no-milliseconds bug.
nonisolated extension CatalogRecord {
	static func databaseDateEncodingStrategy(for column: String) -> DatabaseDateEncodingStrategy {
		.formatted(catalogDateFormatter)
	}

	static func databaseDateDecodingStrategy(for column: String) -> DatabaseDateDecodingStrategy {
		.formatted(catalogDateFormatter)
	}
}

/// THE catalog timestamp format — ratified: ISO 8601 UTC, millisecond
/// precision, 'Z' suffix, so lexicographic order is chronological order.
/// One definition; catalogTimestamp() and CatalogRecord's date strategies
/// both ride it. DateFormatter is Sendable (thread-safe for formatting).
nonisolated let catalogDateFormatter: DateFormatter = {
	let formatter = DateFormatter()
	formatter.locale = Locale(identifier: "en_US_POSIX")
	formatter.timeZone = TimeZone(identifier: "UTC")
	formatter.dateFormat = "yyyy-MM-dd'T'HH:mm:ss.SSS'Z'"
	return formatter
}()

/// The ratified timestamp form, for writes that don't go through a record.
nonisolated func catalogTimestamp(_ date: Date = .now) -> String {
	catalogDateFormatter.string(from: date)
}
