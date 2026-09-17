//
//  FilterTests.swift
//  AlexandriaTests
//
//  The filter engine (filter round, 2026-09-16): normalization,
//  validation refusals, the versioned wire format, and compilation — both
//  the SQL shapes (text + binding order) and the ratified NULL semantics
//  pinned against real records, because the complement-NOT and
//  literal-3VL rulings are exactly the kind of behavior that silently
//  inverts if an edit ever "simplifies" IS NOT TRUE back to NOT().
//

import Foundation
import GRDB
import Testing
@testable import Alexandria

struct FilterTests {

	// MARK: - Fixtures

	private func rating(_ op: FilterToken.Operator, _ value: FilterToken.Value?, negated: Bool = false) -> FilterNode {
		.token(FilterToken(field: .rating, op: op, value: value, negated: negated))
	}

	private func and(_ children: FilterNode...) -> FilterGroup {
		FilterGroup(combine: .and, children: children)
	}

	private func or(_ children: FilterNode...) -> FilterGroup {
		FilterGroup(combine: .or, children: children)
	}

	/// The compiled statement, as WorkingSetQuery would splice it.
	private func build(_ group: FilterGroup) throws -> (sql: String, arguments: StatementArguments) {
		try DatabaseQueue().read { try group.sqlPredicate().build($0) }
	}

	/// Which of the catalog's assets the predicate matches — the compiler's
	/// semantics against real records.
	private func matches(_ catalog: Catalog, _ group: FilterGroup) async throws -> Set<Identifier<Asset>> {
		let predicate = group.sqlPredicate()
		return try await catalog.reader.read { database in
			let (sql, arguments) = try SQL("SELECT id FROM assets WHERE \(predicate)").build(database)
			return try Identifier<Asset>.fetchSet(database, sql: sql, arguments: arguments)
		}
	}

	// MARK: - Normalization

	@Test func emptyRootNormalizesToNil() {
		#expect(and().normalized() == nil)
		#expect(or().normalized() == nil)
	}

	@Test func emptyDescendantGroupsPrune() {
		let token = rating(.gte, .int(3))
		let messy = and(.group(or()), token, .group(and(.group(or()))))
		#expect(messy.normalized() == and(token))
	}

	@Test func degenerateShapesSurviveNormalization() {
		// Singleton and nested same-combine groups are deliberately legal.
		let nested = and(.group(and(rating(.eq, .int(5)))))
		#expect(nested.normalized() == nested)
	}

	// MARK: - Validation

	@Test func operatorOutsideFieldSetRefused() {
		let token = FilterToken(field: .flag, op: .lt, value: .option("pick"))
		#expect(throws: FilterError.operatorNotAllowed(field: "flag", op: "lt")) {
			try token.validate()
		}
	}

	@Test func isUnsetTakesNoValue() {
		#expect(throws: FilterError.valueMismatch(field: "rating")) {
			try FilterToken(field: .rating, op: .isUnset, value: .int(3)).validate()
		}
		#expect(throws: Never.self) {
			try FilterToken(field: .rating, op: .isUnset, value: nil).validate()
		}
	}

	@Test func comparisonRequiresAScalar() {
		#expect(throws: FilterError.valueMismatch(field: "rating")) {
			try FilterToken(field: .rating, op: .eq, value: nil).validate()
		}
		#expect(throws: FilterError.valueMismatch(field: "rating")) {
			try FilterToken(field: .rating, op: .eq, value: .range(.int(1), .int(2))).validate()
		}
	}

	@Test func ratingDomainRefusals() {
		// 0 is not a rating and neither is 6 — the judgment verbs' stance.
		#expect(throws: FilterError.ratingOutOfRange(0)) {
			try FilterToken(field: .rating, op: .eq, value: .int(0)).validate()
		}
		#expect(throws: FilterError.ratingOutOfRange(6)) {
			try FilterToken(field: .rating, op: .between, value: .range(.int(2), .int(6))).validate()
		}
	}

	@Test func invertedRangeRefused() {
		// lo > hi compiles to legal, always-empty SQL — refused at the fence.
		#expect(throws: FilterError.invalidRange) {
			try FilterToken(field: .rating, op: .between, value: .range(.int(4), .int(2))).validate()
		}
	}

	@Test func unknownOptionValuesRefused() {
		#expect(throws: FilterError.unknownOption(field: "flag", value: "picked")) {
			try FilterToken(field: .flag, op: .eq, value: .option("picked")).validate()
		}
		#expect(throws: FilterError.unknownOption(field: "kind", value: "")) {
			try FilterToken(field: .kind, op: .eq, value: .option("")).validate()
		}
	}

	// MARK: - Persistence

	@Test func roundTripPreservesTheTreeAndIsByteStable() throws {
		let tree = and(
			rating(.between, .range(.int(2), .int(4))),
			.token(FilterToken(field: .flag, op: .eq, value: .option("reject"), negated: true)),
			.group(or(
				.token(FilterToken(field: .kind, op: .eq, value: .option("image"))),
				rating(.isUnset, nil)
			))
		)
		let wire = try tree.serialized()
		let decoded = try FilterGroup(serialized: wire)
		#expect(decoded == tree)
		// encode → decode → encode is byte-stable: the property raw-SQL
		// storage could never have.
		#expect(try decoded.serialized() == wire)
	}

	@Test func newerVocabularyGenerationRefusedWhole() throws {
		let wire = try and(rating(.eq, .int(5))).serialized()
			.replacingOccurrences(of: "\"version\":1", with: "\"version\":2")
		#expect(throws: FilterError.unsupportedVersion(2)) {
			try FilterGroup(serialized: wire)
		}
	}

	@Test func unknownFieldIsANamedRefusalNeverASkip() {
		// Skipping an unknown token would WIDEN the matched set — the one
		// failure a filter must never have. The refusal is whole and typed.
		let wire = """
			{"root":{"children":[{"token":{"field":"filename","negated":false,"op":"eq",\
			"value":{"option":"x"}}}],"combine":"and"},"version":1}
			"""
		#expect(throws: FilterError.unknownField("filename")) {
			try FilterGroup(serialized: wire)
		}
	}

	@Test func unknownOperatorRefused() {
		let wire = """
			{"root":{"children":[{"token":{"field":"rating","negated":false,"op":"contains",\
			"value":{"int":3}}}],"combine":"and"},"version":1}
			"""
		#expect(throws: FilterError.unknownOperator("contains")) {
			try FilterGroup(serialized: wire)
		}
	}

	@Test func storedEmptyRootIsCorruptionNotANoOp() {
		let wire = #"{"root":{"children":[],"combine":"and"},"version":1}"#
		#expect(throws: FilterError.emptyFilter) {
			try FilterGroup(serialized: wire)
		}
	}

	@Test func storedTokenFailingValidationRefused() {
		// The persistence fence runs the SAME validation as setFilter — a
		// stored blob gets no trust a live gesture doesn't.
		let wire = """
			{"root":{"children":[{"token":{"field":"rating","negated":false,"op":"eq",\
			"value":{"int":9}}}],"combine":"and"},"version":1}
			"""
		#expect(throws: FilterError.ratingOutOfRange(9)) {
			try FilterGroup(serialized: wire)
		}
	}

	@Test func absentNegatedDecodesAsFalse() throws {
		let wire = """
			{"root":{"children":[{"token":{"field":"rating","op":"eq",\
			"value":{"int":3}}}],"combine":"and"},"version":1}
			"""
		#expect(try FilterGroup(serialized: wire) == and(rating(.eq, .int(3))))
	}

	// MARK: - Compilation: shapes and binding order

	@Test func comparisonCompilesQualifiedAndBound() throws {
		let built = try build(and(rating(.gte, .int(3))))
		#expect(built.sql == "(assets.rating >= ?)")
		#expect(built.arguments == [3])
	}

	@Test func negationCompilesToTheComplement() throws {
		let built = try build(and(rating(.gte, .int(3), negated: true)))
		#expect(built.sql == "((assets.rating >= ?) IS NOT TRUE)")
	}

	@Test func isUnsetCompilesToNullChecksBothWays() throws {
		#expect(try build(and(rating(.isUnset, nil))).sql == "(assets.rating IS NULL)")
		// The complement of a never-NULL expression normalizes to readable SQL.
		#expect(try build(and(rating(.isUnset, nil, negated: true))).sql == "(assets.rating IS NOT NULL)")
	}

	@Test func betweenBindsBothBounds() throws {
		let built = try build(and(rating(.between, .range(.int(2), .int(4)))))
		#expect(built.sql == "(assets.rating BETWEEN ? AND ?)")
		#expect(built.arguments == [2, 4])
	}

	@Test func nestedGroupsParenthesizeAndBindInWalkOrder() throws {
		let tree = and(
			rating(.gte, .int(3)),
			.group(or(
				.token(FilterToken(field: .flag, op: .eq, value: .option("pick"))),
				.token(FilterToken(field: .kind, op: .eq, value: .option("image")))
			))
		)
		let built = try build(tree)
		#expect(built.sql == "(assets.rating >= ? AND (assets.flag = ? OR assets.kind = ?))")
		#expect(built.arguments == [3, "pick", "image"])
	}

	@Test func emptyGroupCompilesToItsIdentityElement() throws {
		// The total-function backstop; normalized() keeps it unreachable
		// through the hub and the persistence fence.
		#expect(try build(and()).sql == "1")
		#expect(try build(or()).sql == "0")
	}

	// MARK: - Compilation: NULL semantics against real records

	/// unrated+unflagged, rated 2, rated 4 — the three-valued-logic corners.
	private func seededCatalog() async throws -> (Catalog, unrated: Identifier<Asset>, two: Identifier<Asset>, four: Identifier<Asset>) {
		let catalog = try Catalog(DatabaseQueue(path: ":memory:"))
		let unrated = try await seedAsset(catalog, at: 1_000)
		let two = try await seedAsset(catalog, at: 2_000)
		let four = try await seedAsset(catalog, at: 3_000)
		_ = try await catalog.setRating([two], to: 2)
		_ = try await catalog.setRating([four], to: 4)
		return (catalog, unrated, two, four)
	}

	@Test func positiveComparisonIsLiteralThreeValuedLogic() async throws {
		// Ratified: unrated matches NO rating comparison — never LrC's
		// unrated-as-zero conflation. isUnset is the deliberate route.
		let (catalog, _, two, _) = try await seededCatalog()
		#expect(try await matches(catalog, and(rating(.lt, .int(3)))) == [two])
	}

	@Test func negationIsTheTrueComplement() async throws {
		// NOT(rating >= 3) includes the unrated asset: complement semantics
		// (IS NOT TRUE). A naive NOT() would drop it from both sides.
		let (catalog, unrated, two, four) = try await seededCatalog()
		let complement = try await matches(catalog, and(rating(.gte, .int(3), negated: true)))
		#expect(complement == [unrated, two])
		// And the two sides partition the catalog exactly.
		let positive = try await matches(catalog, and(rating(.gte, .int(3))))
		#expect(positive == [four])
	}

	@Test func betweenIsInclusiveAndItsComplementKeepsUnrated() async throws {
		let (catalog, unrated, two, four) = try await seededCatalog()
		#expect(try await matches(catalog, and(rating(.between, .range(.int(2), .int(4))))) == [two, four])
		#expect(try await matches(catalog, and(rating(.between, .range(.int(2), .int(4)), negated: true))) == [unrated])
	}

	@Test func isUnsetMatchesExactlyTheUnjudged() async throws {
		let (catalog, unrated, _, _) = try await seededCatalog()
		#expect(try await matches(catalog, and(rating(.isUnset, nil))) == [unrated])
	}

	@Test func orGroupUnions() async throws {
		let (catalog, unrated, two, _) = try await seededCatalog()
		let either = or(rating(.isUnset, nil), rating(.eq, .int(2)))
		#expect(try await matches(catalog, either) == [unrated, two])
	}
}
