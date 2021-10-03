Fixing golang webdav to actually work, which had a BSD license.

Refactoring is needed to properly make a real implementation that efficiently wraps a volume mount, and respects a permission system of some kind.  There still needs to be a way to make dead properties actually work.

The existing Google implementation punts on actually implementing dead properties.  It only has an in-memory implementation of locking, which won't work when the server is scaled up to multiple instances.  And it doesn't have any kind of permission system hooks.  There are cases where the distinction between a file not found vs not allowed is crucial.

```
# login as user "rob" with any password to test volume mounting
./run
```


> This is stable on Linux.  Under MacOS, the mount tends to freeze after stopping and starting it a few times.

Log in as any user/password you want.  This default implementation isn't checking anything yet.

- Under Linux go map `davs://localhost:8000` to your file explorer, and ensure that you can do full editing on the directory, creating files, launching files.

- Under MacOS go map `https://localhost:8000` to your file explorer.
