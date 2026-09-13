//
//  OrderKey.swift
//  Alexandria
//
//  The manual-order key math (collections round, 2026-09-12; rebuilt to
//  the FULL reference algorithm after round review): fractional indexing,
//  Figma/fractional-indexing as the reference. A key is an integer part
//  (variable length, magnitude encoded in the head letter) plus an
//  optional base-62 fraction — all compared as plain bytes, so Swift
//  String order, SQLite's BINARY collation, and this math agree on `<`.
//
//  Why the integer part exists (the review's finding): with fractions
//  alone, APPEND is "insert into the top gap" and key length grows
//  linearly with the collection — a 10k bulk add minted 2,000-char keys.
//  Here append is an integer increment ("a0" → "a1" → … → "az" → "b00"),
//  so length grows logarithmically: 10k appends top out at 4 characters.
//  Fractions appear only on genuine insert-BETWEEN, where a reorder
//  touches one row instead of renumbering the tail (the LrC integer
//  lesson), at arbitrary precision (the LrC float-mantissa lesson).
//
//  Pure: no GRDB, no AppKit — same discipline as GridImaging.
//
//  PERF: repeated insert-between in the SAME gap grows the fraction
//  (~one digit per ~6 inserts). The named answer, if a real collection
//  ever gets there, is a rebalance — re-mint one collection's keys evenly
//  in one transaction. Trigger: max key length past a bound. Not built.
//

nonisolated enum OrderKey {

	/// The digit alphabet, in exactly ASCII order. No character sorts
	/// below '0', so there is no insert-before-the-boundary renumber storm
	/// (LrC v10's '-' bug).
	private static let digits = Array("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
	private static let minDigit: Character = "0"
	private static let maxDigit: Character = "z"

	/// The most negative integer part. Reserved as an exclusive lower
	/// bound by the reference algorithm — never a valid key.
	private static let smallestInteger = "A" + String(repeating: "0", count: 26)

	// MARK: - The public surface

	/// A key strictly between `a` and `b`, where nil means the open end
	/// (nil `a` = before everything, nil `b` = after everything). Append
	/// is `between(last, nil)`; prepend is `between(nil, first)`; the
	/// first key in an empty collection is `between(nil, nil)` ("a0").
	/// Inputs must be well-formed and ordered — violations are minting
	/// bugs, and crash loudly rather than corrupt an ordering quietly.
	/// (Callers holding DATABASE-read keys validate with `isWellFormed`
	/// first and refuse, instead of feeding corruption to a precondition.)
	static func between(_ a: String?, _ b: String?) -> String {
		if let a { precondition(isWellFormed(a), "malformed order key: \(a)") }
		if let b { precondition(isWellFormed(b), "malformed order key: \(b)") }

		switch (a, b) {
		case (nil, nil):
			return "a0"

		case (nil, .some(let b)):
			// Prepend: any well-formed key below b. The bare integer part
			// sorts before b when b carries a fraction; otherwise step the
			// integer down one.
			let integer = integerPart(of: b)
			if integer == smallestInteger {
				// Below the last integer there is only fraction space.
				return integer + midpoint("", Substring(b.dropFirst(integer.count)))
			}
			if integer < b {
				return integer
			}
			guard let stepped = decrementInteger(integer) else {
				preconditionFailure("unreachable: smallestInteger was handled above")
			}
			return stepped

		case (.some(let a), nil):
			// Append: an integer increment — the whole point of the
			// integer part. Falls back to fraction space only past the
			// maximal integer ("z" + 26 max digits, 62^26 keys away).
			let integer = integerPart(of: a)
			if let stepped = incrementInteger(integer) {
				return stepped
			}
			return integer + midpoint(Substring(a.dropFirst(integer.count)), nil)

		case (.some(let a), .some(let b)):
			precondition(a < b, "OrderKey.between requires a < b (got \(a) >= \(b))")
			let integerA = integerPart(of: a)
			let integerB = integerPart(of: b)
			if integerA == integerB {
				// Same integer: a true insert-between, settled in the
				// fraction at arbitrary precision.
				return integerA + midpoint(
					Substring(a.dropFirst(integerA.count)),
					Substring(b.dropFirst(integerB.count))
				)
			}
			guard let stepped = incrementInteger(integerA) else {
				preconditionFailure("unreachable: a < b means a's integer part is not maximal")
			}
			if stepped < b {
				return stepped
			}
			return integerA + midpoint(Substring(a.dropFirst(integerA.count)), nil)
		}
	}

	/// Whether a key parses: a head letter with its declared integer
	/// length present, every character from the alphabet, no trailing
	/// minimum digit in the fraction, and not the reserved smallest
	/// integer. The verbs run DATABASE-read keys through this and refuse
	/// (a named error) instead of crashing — a corrupt catalog is an I/O
	/// fact, not a math bug.
	static func isWellFormed(_ key: String) -> Bool {
		guard let head = key.first, let length = integerLength(head: head) else { return false }
		guard key.count >= length else { return false }
		guard key.dropFirst().allSatisfy({ digitIndex($0) != nil }) else { return false }
		let fraction = key.dropFirst(length)
		if fraction.last == minDigit { return false }
		return key != smallestInteger
	}

	// MARK: - Integer part

	/// Total integer-part length (head included) declared by the head
	/// letter: 'a'–'z' = 2…27 (positive), 'Z'–'A' = 2…27 (negative). The
	/// scheme keeps byte order equal to numeric order across lengths.
	private static func integerLength(head: Character) -> Int? {
		guard let ascii = head.asciiValue else { return nil }
		switch head {
		case "a"..."z": return Int(ascii - Character("a").asciiValue!) + 2
		case "A"..."Z": return Int(Character("Z").asciiValue! - ascii) + 2
		default: return nil
		}
	}

	private static func integerPart(of key: String) -> String {
		String(key.prefix(integerLength(head: key.first!)!))
	}

	/// The next integer up, carrying across digits and head levels
	/// ("a1" → "a2", "az" → "b00", "Zz" → "a0"); nil past the maximum.
	private static func incrementInteger(_ x: String) -> String? {
		let head = x.first!
		var body = Array(x.dropFirst())
		var carry = true
		var i = body.count - 1
		while carry && i >= 0 {
			let next = digitIndex(body[i])! + 1
			if next == digits.count {
				body[i] = minDigit
			} else {
				body[i] = digits[next]
				carry = false
			}
			i -= 1
		}
		if carry {
			if head == "Z" { return "a0" }
			if head == "z" { return nil }
			let nextHead = Character(UnicodeScalar(head.asciiValue! + 1))
			if nextHead > "a" {
				body.append(minDigit)   // positive: one level longer
			} else {
				body.removeLast()       // negative: shorter, toward zero
			}
			return String(nextHead) + String(body)
		}
		return String(head) + String(body)
	}

	/// The mirror: "a0" → "Zz", "b00" → "az"; nil past the minimum.
	private static func decrementInteger(_ x: String) -> String? {
		let head = x.first!
		var body = Array(x.dropFirst())
		var borrow = true
		var i = body.count - 1
		while borrow && i >= 0 {
			let previous = digitIndex(body[i])! - 1
			if previous < 0 {
				body[i] = maxDigit
			} else {
				body[i] = digits[previous]
				borrow = false
			}
			i -= 1
		}
		if borrow {
			if head == "a" { return "Z" + String(maxDigit) }
			if head == "A" { return nil }
			let previousHead = Character(UnicodeScalar(head.asciiValue! - 1))
			if previousHead < "Z" {
				body.append(maxDigit)   // negative: one level longer
			} else {
				body.removeLast()       // positive: shorter, toward zero
			}
			return String(previousHead) + String(body)
		}
		return String(head) + String(body)
	}

	// MARK: - Fraction part

	/// The fraction midpoint (the reference algorithm): strip the common
	/// prefix (padding the lower fraction with virtual '0's), then split
	/// the first differing digit pair — midpoint digit when there's room,
	/// one more digit of precision when they're adjacent. Every branch
	/// returns a non-empty suffix that never ends in '0', so fractions
	/// stay canonical by induction.
	private static func midpoint(_ a: Substring, _ b: Substring?) -> String {
		if var upper = b {
			var lower = a
			var prefix = ""
			while (lower.first ?? minDigit) == upper.first {
				prefix.append(upper.removeFirst())
				if !lower.isEmpty { lower.removeFirst() }
			}
			if !prefix.isEmpty {
				return prefix + midpoint(lower, upper)
			}
		}
		let digitA = a.first.map { digitIndex($0)! } ?? 0
		let digitB = (b?.first).map { digitIndex($0)! } ?? digits.count
		if digitB - digitA > 1 {
			// Room at this digit: the middle, rounded up per the reference.
			return String(digits[(digitA + digitB + 1) / 2])
		}
		// Adjacent digits. If the upper fraction has more precision, its
		// first digit alone already sits strictly between (above every
		// extension of the lower, below the full upper because canonical
		// fractions never end in '0').
		if let b, b.count > 1 {
			return String(b.prefix(1))
		}
		// Otherwise extend the lower fraction one digit deeper.
		return String(digits[digitA]) + midpoint(a.dropFirst(), nil)
	}

	private static func digitIndex(_ character: Character) -> Int? {
		guard let ascii = character.asciiValue else { return nil }
		switch character {
		case "0"..."9": return Int(ascii - Character("0").asciiValue!)
		case "A"..."Z": return Int(ascii - Character("A").asciiValue!) + 10
		case "a"..."z": return Int(ascii - Character("a").asciiValue!) + 36
		default: return nil
		}
	}
}
