package fs

import (
	"context"
	"encoding/xml"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rfielding/webdev/webdav"
)

type DPFile struct {
	F   *os.File
	FS  FS
	Ctx context.Context
}

var _ webdav.File = &DPFile{}
var _ webdav.FileSystem = &FS{}

func (f *DPFile) Read(b []byte) (int, error) {
	return f.F.Read(b)
}

func (f *DPFile) Close() error {
	return f.F.Close()
}

func (f *DPFile) Seek(offset int64, whence int) (int64, error) {
	return f.F.Seek(offset, whence)
}

func (f *DPFile) Readdir(n int) ([]fs.FileInfo, error) {
	result, err := f.F.Readdir(n)
	if err != nil {
		return nil, err
	}
	// filter out what we are not allowed to see
	filteredResult := make([]fs.FileInfo, 0)
	for i := range result {
		if f.FS.Allow(f.Ctx, f.F.Name(), webdav.AllowStat) {
			filteredResult = append(filteredResult, result[i])
		}
	}
	return filteredResult, err
}

func (f *DPFile) Stat() (fs.FileInfo, error) {
	return f.F.Stat()
}

func (f *DPFile) Write(b []byte) (int, error) {
	return f.F.Write(b)
}

func (f *DPFile) DeadProps() (map[xml.Name]webdav.Property, error) {
	return map[xml.Name]webdav.Property{
		/*
			{Space: "DAV:", Local: "banner"}: {
				XMLName:  xml.Name{Space: "DAV:", Local: "banner"},
				InnerXML: []byte("UNCLASSIFIED"),
			},
		*/
	}, nil
}

func (f *DPFile) Patch([]webdav.Proppatch) ([]webdav.Propstat, error) {
	return make([]webdav.Propstat, 0), nil
}

// A FS implements FileSystem using the native file system restricted to a
// specific directory tree.
//
// While the FileSystem.OpenFile method takes '/'-separated paths, a Dir's
// string value is a filename on the native file system, not a URL, so it is
// separated by filepath.Separator, which isn't necessarily '/'.
//
// If we are not allowed to Stat the file, then that means to hide
// it from listings and say that it does not exist.  We may be
// able to Stat the file to know that it exists; but not to actually
// open it to read its contents.
//
type FS struct {
	Root         string
	AllowHandler func(ctx context.Context, name string, allow webdav.Allow) bool
}

//
// The http file system only handles the read part.
// WebDAV now handles writes, effectively extending http.FileSystem
//

func (d FS) resolve(name string) string {
	// This implementation is based on FS.Open's code in the standard net/http package.
	if filepath.Separator != '/' && strings.IndexRune(name, filepath.Separator) >= 0 ||
		strings.Contains(name, "\x00") {
		return ""
	}
	dir := d.Root
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, filepath.FromSlash(webdav.SlashClean(name)))
}

func (d FS) Allow(ctx context.Context, name string, allow webdav.Allow) bool {
	return d.AllowHandler(ctx, name, allow)
}

func (d FS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	if name = d.resolve(name); name == "" {
		return os.ErrNotExist
	}
	if !d.Allow(ctx, name, webdav.AllowMkdir) {
		return webdav.ErrNotAllowed
	}
	return os.Mkdir(name, perm)
}

func (d FS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	if name = d.resolve(name); name == "" {
		return nil, os.ErrNotExist
	}
	if !d.Allow(ctx, name, webdav.AllowStat) {
		return nil, os.ErrNotExist
	}
	if !d.Allow(ctx, name, webdav.AllowOpenFileRead) {
		return nil, webdav.ErrNotAllowed
	}
	if (flag&os.O_RDWR) != 0 && !d.Allow(ctx, name, webdav.AllowOpenFileWrite) {
		return nil, webdav.ErrNotAllowed
	}
	f, err := os.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &DPFile{F: f, FS: d, Ctx: ctx}, nil
}

func (d FS) RemoveAll(ctx context.Context, name string) error {
	if name = d.resolve(name); name == "" {
		return os.ErrNotExist
	}
	if !d.Allow(ctx, name, webdav.AllowStat) {
		return os.ErrNotExist
	}
	if !d.Allow(ctx, name, webdav.AllowRemoveAll) {
		return webdav.ErrNotAllowed
	}
	if name == filepath.Clean(d.Root) {
		// Prohibit removing the virtual root directory.
		return os.ErrInvalid
	}
	return os.RemoveAll(name)
}

func (d FS) Rename(ctx context.Context, oldName, newName string) error {
	if oldName = d.resolve(oldName); oldName == "" {
		return os.ErrNotExist
	}
	if !d.Allow(ctx, oldName, webdav.AllowStat) {
		return os.ErrNotExist
	}
	if !d.Allow(ctx, oldName, webdav.AllowRename) {
		return webdav.ErrNotAllowed
	}
	if newName = d.resolve(newName); newName == "" {
		return os.ErrNotExist
	}
	if root := filepath.Clean(d.Root); root == oldName || root == newName {
		// Prohibit renaming from or to the virtual root directory.
		return os.ErrInvalid
	}
	return os.Rename(oldName, newName)
}

func (d FS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	if name = d.resolve(name); name == "" {
		return nil, os.ErrNotExist
	}
	if !d.Allow(ctx, name, webdav.AllowStat) {
		return nil, os.ErrNotExist
	}
	return os.Stat(name)
}
