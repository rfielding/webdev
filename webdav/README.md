webdav
======

The official experimental repo

```
	"golang.org/x/net/webdav"
```

is not quite in a finished state.  It has various problems such as returning
error values that are equality checked internally, yet private.  There isn't 
an example that is ready to use over a real volume mount, that has
an actual user doing the request, while evaluated versus a permission system.

Other ergonomic issues such as showing directory listings when you hit
a directory with a browser, and were not given an index.html, etc.  

So, this is a refactor and cleanup of the official webdev repo.  I may
reach a point where it is appropriate to submit it back into the Go tree.

The original project has a "BSD-Like" license.
