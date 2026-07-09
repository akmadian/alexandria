- Prune project tracking docs - consolidate key past decisions that impact future decisions, otherwise just drop docs. Keep important things, pare down the rest. It's nice to have lots of info, but it clutters the context window from the very start of getting up to speed on the repo.
- At what point do we just say that "yes, a specific external dependency is required for the app to work properly"? Exiftool is probably one of these. We can still offer to make fetching and install easy for the user, but the endless game of graceful degradation into a useless product seems like a bit of a fool's errand. There's a line between AI assisted enrichment and culling recommendations and basic file metadata handling.
- Asset groups - this can probably be a background job once importer completes. It's basically a matching problem, same as the importer matcher, just on a full set of files from the import, over the db connection. Right?
- RE thumbs ALWAYS come before commit to db - I'm not entirely sure about this anymore tbh. As far as I remember, this was just a UX decision. I think it's fine to communicate to the user that the thumbnail for a particular asset hasn't landed yet, they'll understand. It's usually part of the workflow in almost all DAMs. We should, however, make sure that it's absolutely as fast as possible. Ideally they never notice. This lets the ingest pipeline move bleeding fast, and just fattens up the post ingest enrichment job set.

AST
- We should probably have a README and directory to help people orient themselves and understand the AST. What is it? Why is it a thing? What problem does it solve? What do the go files each do? How does it fit into the overall system?
- The AST should support not only date ranges, but time ranges. Probably we should generalize the date model already there to support times as well.
- We've defined a shitload of operators in the vocabulary file, we probably should think harder about what operators we want to have. There's probably a good pattern to follow here. Equality, numeric comparison, string comparison, membership (membership applies differently to different data structures, but that's a token mapping problem, not a reason to allow the registry to balloon)
- Maybe we should make an asset property registry? Idk, we probably duplicate the "here's all the supported asset properties" idea in several places. Might be good to tie it to a central spot. Interesting piece is that mappings to these constants probably aren't universal across all file and asset types - hello again assset type registry?

Sweeps/ Audits
- I might like to refactor FTS to FullTextSearch. It's clearer, and fits better in my mind. FTS always takes a second for me to parse. I suspect we might want to make a FullTextSearch package to handle all of that stuff, maybe. That would probably be a good time.
- Project tracking is bloated with a lot of completed impl specs and files, let's do a comprehensive cleaning run. Either move files into src for in-src docs, or consolidate and delete old content.
- Ideally, everything in coding guidelines should be enforced by linter checks. We should check the doc against the existing rules - when we have an enforced rule, the doc section about it is redundant and should be removed. Goal is to interrogate every rule and convention to the point where we completely empty the coding guidelines doc. Some things are too large conceptually for a linter (it's hard to write a linter rule for "good testability"), and that's fine. We probably won't completely empty it, but we should pare it down significantly and have it only discuss real invariants and ideas. Anything testable/lintable should be tested and linted automatically.
- Look around deeply, identify opportunities and pathways for integration tests

Settings
- Probably pull default settings sets out into a defaults.go instead of having them in settings.go. Settings.go should be machinery, probably not cluttered with data that isn't actually core to its function


Docs
- Write a contributing guide when we're getting ready to open up to contributions. This should come with issue and PR templates, branch protection, etc.