- How to make supporting new file types and such easy? I don't want to have to do a major rewrite to add support for Adobe after effects in addition to Davinci resolve projects. Asset groups are also important here. Ideally we can just let the user point to a davinci resolve project and we do the rest of the work, they don't have to manage knowledge of lots of other file types.
- Want to support markdown notes

Code Design
- Leverage polymorphism, inheritance, and interfaces where reasonable. Don't want to have to manage a billion interfaces for a billion different file types. At the same time, don't want to break interfaces too granularly. We need to right size the splits.
- LOGGING - logs should be rich but concise and readable. Colors should be used to denote log levels and source components.


"Some day" features
- More comprehensive backup system
    - background backup worker/ daemon? destinations of smb/nfs shares? s3 pattern cloud storage buckets? borg support for cost effectiveness and incremental backup speed?
    - Analyze best backup systems for media files. Most compression algos that I'm aware of don't perform particularly well with media files. Lossless compression is of course an absolute must.
    - This feels like a potential liability. May be best to just leave media backup to other tools that do it better, and let us manage just the catalog files.