package fs

import (
	"context"
	"encoding/xml"
	"encoding/json"
	"fmt"
	"github.com/rfielding/webdev/webdav"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	//ixml "github.com/rfielding/webdev/webdav/internal/xml"

)

/*
   These are the expected types
*/
var _ webdav.File = &DPFile{}
var _ webdav.FileSystem = &FS{}

/*
  There are a few actions that we need permission for
*/
type Allow string

const AllowCreate = Allow("Create")
const AllowRead = Allow("Read")
const AllowWrite = Allow("Write")
const AllowDelete = Allow("Delete")
const AllowStat = Allow("Stat")

/*
  At a minimum, we need to know what kind of change we are making to which file
*/
type Action struct {
	Action Allow  `json:"action"`
	Name   string `json:"name"`
}

/*
 This is a file object that can support DeadProperties
*/
type DPFile struct {
	F   *os.File
	FS  FS
	Ctx context.Context
}

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
		permissions := f.FS.PermissionHandler(f.Ctx, Action{Name: f.F.Name(), Action: AllowStat})
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

// Encapsulate naming conventions for files that are attachments to real files
func NameFor(name, ftype string) string {
	d := path.Dir(name)
	b := path.Base(name)
	theFile := name
	if strings.HasPrefix(".__", b) {
		// ignore
	} else {
		if d == "." {
			theFile = fmt.Sprintf("%s/.__%s", b, ftype)
		} else {
			s, err := os.Stat(name)
			if err != nil {
				log.Printf("WEBDAV: stat on %s file %v", ftype, err)
				return ""
			} else {
				if s.IsDir() {
					theFile = fmt.Sprintf("%s/.__%s", name, ftype)
				} else {
					theFile = fmt.Sprintf("%s/.__%s.%s", d, b, ftype)
				}	
			}
		}
	}
	return theFile
} 

func (f *DPFile) DeadProps() (map[xml.Name]webdav.Property, error) {
	// To avoid xml serialization hassles, just store the dead properties as json
	// xml handling is too much of a mess at the moment
	name := f.F.Name()
	// No dead properties on metadata files.
	if strings.HasPrefix(path.Base(name), ".__") {
		return map[xml.Name]webdav.Property{}, nil	
	}

	// If the file doesn't exist, then return empty properties
	retval := make(map[xml.Name]webdav.Property)
	propertiesFile := NameFor(name, "deadproperties.json")
	if _,err := os.Stat(propertiesFile); os.IsNotExist(err) {
		return retval,nil
	}
	bytes, err := ioutil.ReadFile(propertiesFile)
	if err != nil {
		log.Printf("error opening properties file %s: %v", propertiesFile, err)
		return retval, nil
	}
	var propertiesMap map[string]string 
	err = json.Unmarshal(bytes,&propertiesMap)
	if err != nil {
		log.Printf("error unmarshalling json %s: %v", propertiesFile, err)
		return retval, nil
	}
	for k := range propertiesMap {
		log.Printf("set: %s -> %s", k, propertiesMap[k])
		retval[xml.Name{Space: "DAV:", Local: k}] = webdav.Property{
            XMLName:  xml.Name{Space: "DAV:", Local: k},
            InnerXML: []byte(propertiesMap[k]),
		}
	}
	return retval, nil
}

// TODO: figure out what needs to be serialized.  I don't think there
// is any standard.
func (f *DPFile) Patch(p []webdav.Proppatch) ([]webdav.Propstat, error) {
	// Update the properties struct and return val
	retval := make([]webdav.Propstat, 0)
	current, err := f.DeadProps()
	if err != nil {
		return retval, nil
	}
	var writeVal map[string]string
	for k := range current {
		writeVal[k.Local] = string(current[k].InnerXML)
	}
	for i := range p {
		for j := range p[i].Props {
			v := p[i].Props[j]
			k := v.XMLName.Local
			s := string(v.InnerXML)
			retval[0].Props = append(retval[0].Props, webdav.Property{
				XMLName:  xml.Name{Space: "DAV:", Local: k},
				InnerXML: []byte(s),	
			})
			writeVal[k] = s
			retval[0].Status = 200
		}
	}
	// Persist it back to disk as json
	data, err := json.MarshalIndent(writeVal, "", "  ")
	if err != nil {
		return nil, err
	}
	propertiesFile := NameFor(f.F.Name(), "deadproperties.json")
	err = ioutil.WriteFile(propertiesFile, data, 0744)
	if err != nil {
		return nil, err
	}
	return retval, nil
}

// A FS implements FileSystem using the native file system restricted to a
// specific directory tree.
type FS struct {
	Root              string
	Locks webdav.LockSystem
	PermissionHandler func(ctx context.Context, action Action) map[string]interface{}
}

//
// The http file system only handles the read part.
// WebDAV now handles writes, effectively extending http.FileSystem
// JWT claims handle authorization
// OpenPolicyAgent calculates permission based on the JWT claims
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

// Convenience function for extracting a boolean permission once the calculation is done for the file in context
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
	permission := d.PermissionHandler(ctx, Action{Name: path.Base(name), Action: AllowCreate})
	if !d.Allow(ctx, permission, AllowCreate) {
		return webdav.ErrNotAllowed
	}
	return os.Mkdir(name, perm)
}

func (d FS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	if name = d.resolve(name); name == "" {
		return nil, os.ErrNotExist
	}
	_, err := os.Stat(name)
	// on create, ask parent if we can modify it
	if os.IsNotExist(err) {
		permission := d.PermissionHandler(ctx, Action{Name: path.Dir(name), Action: AllowCreate})
		if (flag&os.O_RDWR) != 0 && !d.Allow(ctx, permission, AllowCreate) {
			return nil, webdav.ErrNotAllowed
		}
	} else {
		// on update, ask file if it can be modified
		permission := d.PermissionHandler(ctx, Action{Name: name, Action: AllowWrite})
		if !d.Allow(ctx, permission, AllowStat) {
			return nil, os.ErrNotExist
		}
		if (flag&os.O_RDWR) != 0 && !d.Allow(ctx, permission, AllowWrite) {
			return nil, webdav.ErrNotAllowed
		}
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
	permission := d.PermissionHandler(ctx, Action{Name: name, Action: AllowDelete})
	if !d.Allow(ctx, permission, AllowStat) {
		return os.ErrNotExist
	}
	if !d.Allow(ctx, permission, AllowDelete) {
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
	permission := d.PermissionHandler(ctx, Action{Name: oldName, Action: AllowRead})
	if !d.Allow(ctx, permission, AllowStat) {
		return os.ErrNotExist
	}
	if !d.Allow(ctx, permission, AllowRead) {
		return webdav.ErrNotAllowed
	}

	// if the name DOES exist, then rename is not allowed
	if newName = d.resolve(newName); newName != "" {
		return webdav.ErrNotAllowed
	}

	permission = d.PermissionHandler(ctx, Action{Name: newName, Action: AllowCreate})
	if !d.Allow(ctx, permission, AllowWrite) {
		return webdav.ErrNotAllowed
	}

	if root := filepath.Clean(d.Root); root == oldName || root == newName {
		// Prohibit renaming from or to the virtual root directory.
		return os.ErrInvalid
	}
	return os.Rename(oldName, newName)
}

// Note that if we can't stat a file, we should tell the user that it does not exist.
func (d FS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	if name = d.resolve(name); name == "" {
		return nil, os.ErrNotExist
	}
	permission := d.PermissionHandler(ctx, Action{Name: name, Action: AllowStat})
	if !d.Allow(ctx, permission, AllowStat) {
		return nil, os.ErrNotExist
	}
	return os.Stat(name)
}
