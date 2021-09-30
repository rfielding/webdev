Fixing golang webdav to actually work, which had a BSD license.

Refactoring is needed to properly make a real implementation that efficiently wraps a volume mount, and respects a permission system of some kind.  There still needs to be a way to make dead properties actually work.

The existing Google implementation punts on actually implementing dead properties.  It only has an in-memory implementation of locking, which won't work when the server is scaled up to multiple instances.  And it doesn't have any kind of permission system hooks.  There are cases where the distinction between a file not found vs not allowed is crucial.
