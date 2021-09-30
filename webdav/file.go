// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webdav

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
)

// A FileSystem implements access to a collection of named files. The elements
// in a file path are separated by slash ('/', U+002F) characters, regardless
// of host operating system convention.
//
// Each method has the same semantics as the os package's function of the same
// name.
//
// Note that the os.Rename documentation says that "OS-specific restrictions
// might apply". In particular, whether or not renaming a file or directory
// overwriting another existing file or directory is an error is OS-dependent.
type FileSystem interface {
	Mkdir(ctx context.Context, name string, perm os.FileMode) error
	OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (File, error)
	RemoveAll(ctx context.Context, name string) error
	Rename(ctx context.Context, oldName, newName string) error
	Stat(ctx context.Context, name string) (os.FileInfo, error)
	Allow(ctx context.Context, name string, allow Allow) bool
}

type Allow string

const AllowMkdir = Allow("Mkdir")
const AllowOpenFileRead = Allow("OpenFileRead")
const AllowOpenFileWrite = Allow("OpenFileWrite")
const AllowRemoveAll = Allow("RemoveAll")
const AllowRename = Allow("Rename")
const AllowStat = Allow("Stat")

// A File is returned by a FileSystem's OpenFile method and can be served by a
// Handler.
type File interface {
	http.File
	io.Writer
	DeadPropsHolder
}

var (
	// The errors need to be public so that implementations can
	// return them, as there are equality checks done against them!
	ErrDestinationEqualsSource = errors.New("webdav: destination equals source")
	ErrDirectoryNotEmpty       = errors.New("webdav: directory not empty")
	ErrInvalidDepth            = errors.New("webdav: invalid depth")
	ErrInvalidDestination      = errors.New("webdav: invalid destination")
	ErrInvalidIfHeader         = errors.New("webdav: invalid If header")
	ErrInvalidLockInfo         = errors.New("webdav: invalid lock info")
	ErrInvalidLockToken        = errors.New("webdav: invalid lock token")
	ErrInvalidPropfind         = errors.New("webdav: invalid propfind")
	ErrInvalidProppatch        = errors.New("webdav: invalid proppatch")
	ErrInvalidResponse         = errors.New("webdav: invalid response")
	ErrInvalidTimeout          = errors.New("webdav: invalid timeout")
	ErrNoFileSystem            = errors.New("webdav: no file system")
	ErrNoLockSystem            = errors.New("webdav: no lock system")
	ErrNotADirectory           = errors.New("webdav: not a directory")
	ErrPrefixMismatch          = errors.New("webdav: prefix mismatch")
	ErrRecursionTooDeep        = errors.New("webdav: recursion too deep")
	ErrUnsupportedLockInfo     = errors.New("webdav: unsupported lock info")
	ErrUnsupportedMethod       = errors.New("webdav: unsupported method")
	ErrNotAllowed              = errors.New("webdav: not allowed")
)
