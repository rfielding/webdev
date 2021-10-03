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

type Allow string

const AllowMkdir = Allow("Mkdir")
const AllowOpenFileRead = Allow("OpenFileRead")
const AllowOpenFileWrite = Allow("OpenFileWrite")
const AllowRemoveAll = Allow("RemoveAll")
const AllowStat = Allow("Stat")

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
		permissions := f.FS.AllowHandler(f.Ctx, f.F.Name())
		if f.FS.Allow(f.Ctx, permissions, AllowStat) {
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
		{Space: "DAV:", Local: "banner"}: {
			XMLName:  xml.Name{Space: "DAV:", Local: "banner"},
			InnerXML: []byte("UNCLASSIFIED"),
		},
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
	AllowHandler func(ctx context.Context, name string) map[string]interface{}
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

func (d FS) Allow(ctx context.Context, permissions map[string]interface{}, allow Allow) bool {
	v, ok := permissions[string(allow)].(bool)
	if ok {
		return v
	}
	return false
}

func (d FS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	if name = d.resolve(name); name == "" {
		return os.ErrNotExist
	}
	permission := d.AllowHandler(ctx, name)
	if !d.Allow(ctx, permission, AllowMkdir) {
		return webdav.ErrNotAllowed
	}
	return os.Mkdir(name, perm)
}

func (d FS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	if name = d.resolve(name); name == "" {
		return nil, os.ErrNotExist
	}
	permission := d.AllowHandler(ctx, name)
	if !d.Allow(ctx, permission, AllowStat) {
		return nil, os.ErrNotExist
	}
	if !d.Allow(ctx, permission, AllowOpenFileRead) {
		return nil, webdav.ErrNotAllowed
	}
	if (flag&os.O_RDWR) != 0 && !d.Allow(ctx, permission, AllowOpenFileWrite) {
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
	permission := d.AllowHandler(ctx, name)
	if !d.Allow(ctx, permission, AllowStat) {
		return os.ErrNotExist
	}
	if !d.Allow(ctx, permission, AllowRemoveAll) {
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
	permission := d.AllowHandler(ctx, oldName)
	if !d.Allow(ctx, permission, AllowStat) {
		return os.ErrNotExist
	}
	if !d.Allow(ctx, permission, AllowOpenFileRead) {
		return webdav.ErrNotAllowed
	}

	// if the name DOES exist, then rename is not allowed
	if newName = d.resolve(newName); newName != "" {
		return webdav.ErrNotAllowed
	}

	permission = d.AllowHandler(ctx, newName)
	if !d.Allow(ctx, permission, AllowOpenFileWrite) {
		return webdav.ErrNotAllowed
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
	permission := d.AllowHandler(ctx, name)
	if !d.Allow(ctx, permission, AllowStat) {
		return nil, os.ErrNotExist
	}
	return os.Stat(name)
}
