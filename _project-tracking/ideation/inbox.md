## What we do and don't do
- At what point do we just say that "yes, a specific external dependency is required for the app to work properly"? Exiftool is probably one of these. We can still offer to make fetching and install easy for the user, but the endless game of graceful degradation into a useless product seems like a bit of a fool's errand. There's a line between AI assisted enrichment and culling recommendations and basic file metadata handling.
- If we can reasonably and effectively do something within alexandria, we probably should. An example is maybe text file editing, markdown rendering, etc.


## Backend Quick Items
- Probably pull default settings sets out into a defaults.go instead of having them in settings.go. Settings.go should be machinery, probably not cluttered with data that isn't actually core to its function
- Refactor *_errors tables to *_errors_dlq? to be clear it's a dlq
- Not so quick, maybe: refactor dependency package. Have it be a singleton that dependents get a pointer to, on startup, gathers dependency states, loads up available deps. Users of deps can request a depdendency "handle"? Or maybe the dependency singleton provides a dispatch interface where callers can provide a registered dependency and command? Or pass the dependency class? Idk, this feels like a specific pattern with a name, don't know what name is, but want to know.
- After the workflow engine has been created, why even maintain the ingest pipeline? The ingest pipeline should theoretically be able to fit into the workflow just fine, it's pretty much the same shape! The job dag for import is just a straight line that feeds into thumbnail. Refactor enrichment -> workflow. Begs the question - create a "workflow" interface that pulls several jobs together from the job registry and has a bunch of nice features on top? Invoking a workflow is the same idea as any other thing that starts any other job node. Same code path, just different source. Thinking of user defined workflows and supporting one off image resizes - the thumbnail job is really just a resize job? Can we just name it ResizeRaster and thumbnailer's raster path will just call resize raster? Can put resizeraster in image package. Image package declares resize, takes image, returns resized? Idk.
- Workflow engine optimization idea, colon. There are basically two kinds of jobs. The first is essentially just running a function, and that's it. We feed some data into a function, and we get the return value. Everything else is literally just get that data into the function, get the return value. We can handle that in one of two ways. One of them is that we can compound multiples of these calls that can run on the same data in the same job. I'm not a huge fan of that. It's compounding multiple operations or concerns into a single place, and it's all sort of lumped together. It's less modular. It's a little bit more messy. It just smells. What if we could have two different job types? One of them is sort of, like, wrapped. It's the it's the existing job model for sort of, you know, more in-depth code with logic and, you know, maybe a dependency call and all that. And then the other kind is literally just like, we call a function with a parameter, and and that's it. That would be a really interesting way to do things, and it would probably speed up those fast calls, like, by a lot because it's just a thin wrapper, whereas... and we can probably do them without a lot of the sort of orchestration machinery as well. Whereas with the longer ones that take more resources and stuff, we wanna treat those materially different you know, they run longer. They need queues. They need more performance, so they need semaphore system. The budget system but some operations are incredibly cheap, and so it would be really cool if we could sort of have, like, a fast path and a slow path for each node. Like, a node can be a fast node or a slow node. Maybe. I don't know. Interesting idea.

## Other
- Asset groups - this can probably be a background job once importer completes. It's basically a matching problem, same as the importer matcher, just on a full set of files from the import, over the db connection. Right?

AST
- We should probably have a README and directory to help people orient themselves and understand the AST. What is it? Why is it a thing? What problem does it solve? What do the go files each do? How does it fit into the overall system?

Sweeps/ Audits
- I might like to refactor FTS to FullTextSearch. It's clearer, and fits better in my mind. FTS always takes a second for me to parse. I suspect we might want to make a FullTextSearch package to handle all of that stuff, maybe. That would probably be a good time.
- Ideally, everything in coding guidelines should be enforced by linter checks. We should check the doc against the existing rules - when we have an enforced rule, the doc section about it is redundant and should be removed. Goal is to interrogate every rule and convention to the point where we completely empty the coding guidelines doc. Some things are too large conceptually for a linter (it's hard to write a linter rule for "good testability"), and that's fine. We probably won't completely empty it, but we should pare it down significantly and have it only discuss real invariants and ideas. Anything testable/lintable should be tested and linted automatically.
- Look around deeply, identify opportunities and pathways for integration tests

Dev Window for Wails
- Should open as a separate window that can be seen alongside the app content, should not cover app content. App should be fully interactive and such while the dev window is open.
- Events
    - Manually fire events from backend to trigger frontend events
    - Monitor events stream with differentiation between natural and injected events
- IPC
    - Can we see the IPC stream and inspect the calls/responses?


## App UI Thingies

- FEAT: Maybe want to support annotations? Like users can click on an image to point to a specific thing, then take notes on that thing?
- NOTE from task 19 - clear on reimport. Reimporting an asset correctly wipes observations and generated artifacts to regenerate anew. The old thumbnail file remains on disk, and the UI's cache keeps serving it. Cache not automatically busted on reimport. BAD UX path would be user reimports and all representations of the asset do not update to freshly imported and enriched values.

### Experience References
- Photos: LrC
- Fonts: MacOS FontBook

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