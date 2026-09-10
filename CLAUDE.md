# Alexandria — Agent Ground Rules

Native macOS DAM (Swift, GRDB/SQLite). Fresh repo, September 2026. The old Go-core
Alexandria that lived at this path is dead. If recalled memory, prior-session
instinct, or old-repo convention conflicts with this repo's files, this repo wins.

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
