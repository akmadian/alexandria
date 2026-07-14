## What we do and don't do
- At what point do we just say that "yes, a specific external dependency is required for the app to work properly"? Exiftool is probably one of these. We can still offer to make fetching and install easy for the user, but the endless game of graceful degradation into a useless product seems like a bit of a fool's errand. There's a line between AI assisted enrichment and culling recommendations and basic file metadata handling.
- If we can reasonably and effectively do something within alexandria, we probably should. An example is maybe text file editing, markdown rendering, etc.


## Other
- Asset groups - this can probably be a background job once importer completes. It's basically a matching problem, same as the importer matcher, just on a full set of files from the import, over the db connection. Right?

AST
- We should probably have a README and directory to help people orient themselves and understand the AST. What is it? Why is it a thing? What problem does it solve? What do the go files each do? How does it fit into the overall system?

Sweeps/ Audits
- I might like to refactor FTS to FullTextSearch. It's clearer, and fits better in my mind. FTS always takes a second for me to parse. I suspect we might want to make a FullTextSearch package to handle all of that stuff, maybe. That would probably be a good time.
- Ideally, everything in coding guidelines should be enforced by linter checks. We should check the doc against the existing rules - when we have an enforced rule, the doc section about it is redundant and should be removed. Goal is to interrogate every rule and convention to the point where we completely empty the coding guidelines doc. Some things are too large conceptually for a linter (it's hard to write a linter rule for "good testability"), and that's fine. We probably won't completely empty it, but we should pare it down significantly and have it only discuss real invariants and ideas. Anything testable/lintable should be tested and linted automatically.
- Look around deeply, identify opportunities and pathways for integration tests

Settings
- Probably pull default settings sets out into a defaults.go instead of having them in settings.go. Settings.go should be machinery, probably not cluttered with data that isn't actually core to its function

- FEAT: Maybe want to support annotations? Like users can click on an image to point to a specific thing, then take notes on that thing?

Docs
- Write a contributing guide when we're getting ready to open up to contributions. This should come with issue and PR templates, branch protection, etc.
- Write feature add runbooks where standard shape exists (add new filetype, promote field from json extraMetadata blob to db column)


Dev Window for Wails
- Should open as a separate window that can be seen alongside the app content, should not cover app content. App should be fully interactive and such while the dev window is open.
- Events
    - Manually fire events from backend to trigger frontend events
    - Monitor events stream with differentiation between natural and injected events
- IPC
    - Can we see the IPC stream and inspect the calls/responses?


## App UI Thingies

### Aesthetic
- Frosted glass aesthetic doesn't necessarily require the whole app background to be glass-on-gradient. Pieces that sit on top of flat background can be, within themselves, glass-on-gradient with content on top. 
- Want tokens to selectively apply gradient background, AND glass background individually. Maybe want to allow translucent glass down to component substrate, even including the app window itself? Want to be able to selectively layer gradient and glass, as desired.

### Job Tracking
- For active jobs tracking - have a skeumorphic folder tab aesthetic thing where multiple jobs stack like the folder tabs would stack but offset naturally yknow? Could maybe display progress indicator or something on the tab, tab is clickable to slide the item with tab motif up into view for more details. https://www.cosmos.so/e/1108369543

### Grid
- I think the grid is the hardest thing to nail. It feels like there's a clash between the intended clean, dense but spacious language of the other parts of the UI, and the comparatively cluttered grid piece.


## Marketing Site UI Thingies
- Scroll from hero reveals a mess of file types and such, lines from each of them lead down into some other element that represents the idea of Alexandria bringing them all together. Kinda like https://www.cosmos.so/e/939600872 but iverted?
- A thing with the hero - "Dare I say, the most {WORD} digital asset manager out there." where word scrolls between things like "performant, private, modern, customizable, user friendly, capable, respectful, free, well designed" etc. Maybe some cheeky ones like "anti-adobe"
- "And the best part, it's free (if you want)." - leads in to the pay what you want UX.
- Can (and should) host with github pages. Dedicated dir in this package with dedicated CI/CD, should be pretty straight forward. Lemon Squeezy works for static sites.