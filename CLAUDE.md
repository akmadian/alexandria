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

## Notes from Ari - ALWAYS follow these rules.
- We are writing real production code. Your task is NEVER to just get some code in the repo or get some stuff on screen that you think will get my approval.
  Internal mechanics, organization, extensability, readability, testability, etc, ALL matter and are critical.
- A lot of the things we are writing and considering are complex and deserve time, consideration, and intentional design.
  Reaching for the nearest plausible way of writing code is almost always the wrong way to do it, and will actively hurt the product.
  Consider software architecture and design principles such as SOLID when designing code.
- Challenge your assumptions, instead of working from memory all the time, you should actually pull documentation.
  A lot of the systems we are building are not new. Smarter people than us have spent a lot of time trying to figure out some of the questions we are asking. We should ground ourself in established, well trodden, battle tested codebases and software architecture patterns. They exist for a reason. Trying to rebuild them from the bottom up will only waste time, energy, and will yield an inferior result.
- Complexity is fine, but must earn its place.
- How code is structured, what things are named, where we draw the line beteween various methods or interfaces is not purely ceremony.
  These things are critical to how readable and maintainable the code is.
- SwiftUI components are strongly preferred for their visual consistency.
  AppKit may be used if there is a genuine gap in SwiftUI, or if the AppKit interface offers substantially better tooling or features than SwiftUI.
  Whenever we use a component, we should consider how to make maximal use of that component's features.
- When I ask you to explain something to me, you MAY NOT write code into the repo until I give you explicit approval.
- Always wait for my approval before committing.

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
- "How would I…", "show me", "what would this look like", or any request for an
  example is a request for CODE IN CHAT, not a grant to create or edit repo
  files. Answer inline. Touch the repo only on an explicit build instruction
  ("add it", "implement", "wire it in", "do it"). When unsure which one it is,
  it's the example.
- Research subagents return findings in chat and never write files.
