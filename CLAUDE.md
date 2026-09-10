# Alexandria — Agent Ground Rules

Native macOS DAM (Swift, GRDB/SQLite). Fresh repo, September 2026. The old Go-core
Alexandria that lived at this path is dead. If recalled memory, prior-session
instinct, or old-repo convention conflicts with this repo's files, this repo wins.

## Notes from Ari
- We are writing real production code for a real product. Slop is not acceptable.
  We are never trying to just get some stuff on screen to get my approval.
  Internal mechanics of the code, cleanliness and readability, organization, scalability, extensability, testability etc ALL matter and are critical. Code should be written idiomatically and grounded in good software design principles such as SOLID.
- Complexity of a system is fine, but must earn its place. Arbitrary complexity is often just a surface for bugs and code rot to accumulate.
- Don't ever be biasing towards speed or resolution of an open item over all else. A lot of the work we're doing is important and deserves careful design and consideration, and we should give the work the consideration it's due.
- Naming must be descriptive. No names less than three letters are acceptable, except in special cases.
  Names must be clear about what exactly they refer to at the site of use. Abbreviations are often just pointless readability taxes.
  That said, it's not good to be overly verbose as well. Names that become really long or convoluted often indicate poor software architecture.
- On code structure - how code is structured and what things are named, where we draw the line between various methods, classes, structs, and interfaces and why, is not just ceremony. These things are critical to the readability and maintainability of the code.
- Keep comments brief and to the point.
  If you're considering adding a comment to a comment, maybe with a datestamp, you should probably just rewrite the comment fresh.
  Multi sentence comments or longer are usually only ever warranted at the top of a file.
- SwiftUI components are strongly preferred for their visual consistency and cohesiveness.
  AppKit UI components may be used if there is genuinely a gap in what SwiftUI offers.
  Usage of AppKit systems (such as NSCollectionView, for example) is fine, and often desirable as they offer behavior or features that SwiftUI components don't.

## Authorities
- `_design/` is the product's source of truth: `requirements.md` (requirements +
  settled nouns), `database.md`, `learnings.md`, `dam-landscape.md` (research).
- The old repo is archaeology. Concepts cross as prose, only when Ari invokes
  them. Code never crosses — ported code smuggles dead architecture.
- This file is the process authority. Process rulings EDIT it in place — never a
  new doc, never a parallel section. Hard cap: one page.

## Register — the prime discipline
- Research findings are delivered in chat. They never go into files.
- Files record only what Ari ratified in discussion, written on his explicit ask
  or a standing grant he gave. A conclusion written to a file without
  ratification poisons every future session; this is one of the poisons that
  killed the last repo.
- Never mint positioning frames, equations, scope razors, or feature
  prescriptions in durable files.
- Track register precisely: RATIFIED (Ari said it) / PROPOSED (yours, awaiting
  his word) / DERIVED (reasoned from general knowledge, unverified). Questions,
  "interesting", "maybe", and thinking-out-loud are NOT approvals. A trailing
  hedge ("…I don't know") un-ratifies the sentence before it.
- Ari closes rounds and ends sessions. Closure is not a build green light;
  ratifying a direction never ratifies the build that follows it.

## Design conduct
- Concepts, not schema: design docs record nouns and behavior. Every stored
  field, enum, table, or mechanism must earn its place in its own design round.
  When an unearned one is caught, strike it and leave a deliberately-unsettled
  marker so it can't silently return.
- Machinery earns its place: no speculative fields, no interfaces with one
  implementation, no config for constants. Deletion over addition.
- Ari's personal workflow validates primitives; it never becomes product
  defaults.
- Ground architecture and UX claims in fetched prior art. Search for the
  phenomenon, not the conclusion you hope for — leading-query research is a
  named defect.
- Record rough targets as rough. Never overclaim a recorded feature's scope.

## Actions
- No commits without Ari's explicit OK.
- No file writes outside ratified content and granted lanes; when in doubt, it
  goes to chat and Ari decides.
- Research subagents return findings in chat and never write files.
