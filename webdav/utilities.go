package webdav

import (
	"context"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"golang.org/x/net/webdav"
)

// slashClean is equivalent to but slightly more efficient than
// path.Clean("/" + name).
func SlashClean(name string) string {
	if name == "" || name[0] != '/' {
		name = "/" + name
	}
	return path.Clean(name)
}

// walkFS traverses filesystem fs starting at name up to depth levels.
//
// Allowed values for depth are 0, 1 or InfiniteDepth. For each visited node,
// walkFS calls walkFn. If a visited file system node is a directory and
// walkFn returns filepath.SkipDir, walkFS will skip traversal of this node.
func WalkFS(ctx context.Context, fs FileSystem, depth int, name string, info os.FileInfo, walkFn filepath.WalkFunc) error {
	// This implementation is based on Walk's code in the standard path/filepath package.
	err := walkFn(name, info, nil)
	if err != nil {
		if info.IsDir() && err == filepath.SkipDir {
			return nil
		}
		return err
	}
	if !info.IsDir() || depth == 0 {
		return nil
	}
	if depth == 1 {
		depth = 0
	}

	// Read directory names.
	f, err := fs.OpenFile(ctx, name, os.O_RDONLY, 0)
	if err != nil {
		return walkFn(name, info, err)
	}
	fileInfos, err := f.Readdir(0)
	f.Close()
	if err != nil {
		return walkFn(name, info, err)
	}

	for _, fileInfo := range fileInfos {
		filename := path.Join(name, fileInfo.Name())
		fileInfo, err := fs.Stat(ctx, filename)
		if err != nil {
			if err := walkFn(filename, fileInfo, err); err != nil && err != filepath.SkipDir {
				return err
			}
		} else {
			err = WalkFS(ctx, fs, depth, filename, fileInfo, walkFn)
			if err != nil {
				if !fileInfo.IsDir() || err != filepath.SkipDir {
					return err
				}
			}
		}
	}
	return nil
}

// copyFiles copies files and/or directories from src to dst.
//
// See section 9.8.5 for when various HTTP status codes apply.
func CopyFiles(ctx context.Context, fs FileSystem, src, dst string, overwrite bool, depth int, recursion int) (status int, err error) {
	if recursion == 1000 {
		return http.StatusInternalServerError, ErrRecursionTooDeep
	}
	recursion++

	// TODO: section 9.8.3 says that "Note that an infinite-depth COPY of /A/
	// into /A/B/ could lead to infinite recursion if not handled correctly."

	srcFile, err := fs.OpenFile(ctx, src, os.O_RDONLY, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return http.StatusNotFound, err
		}
		return http.StatusInternalServerError, err
	}
	defer srcFile.Close()
	srcStat, err := srcFile.Stat()
	if err != nil {
		if os.IsNotExist(err) {
			return http.StatusNotFound, err
		}
		return http.StatusInternalServerError, err
	}
	srcPerm := srcStat.Mode() & os.ModePerm

	created := false
	if _, err := fs.Stat(ctx, dst); err != nil {
		if os.IsNotExist(err) {
			created = true
		} else {
			return http.StatusForbidden, err
		}
	} else {
		if !overwrite {
			return http.StatusPreconditionFailed, os.ErrExist
		}
		if err := fs.RemoveAll(ctx, dst); err != nil && !os.IsNotExist(err) {
			return http.StatusForbidden, err
		}
	}

	if srcStat.IsDir() {
		if err := fs.Mkdir(ctx, dst, srcPerm); err != nil {
			return http.StatusForbidden, err
		}
		if depth == InfiniteDepth {
			children, err := srcFile.Readdir(-1)
			if err != nil {
				return http.StatusForbidden, err
			}
			for _, c := range children {
				name := c.Name()
				s := path.Join(src, name)
				d := path.Join(dst, name)
				cStatus, cErr := CopyFiles(ctx, fs, s, d, overwrite, depth, recursion)
				if cErr != nil {
					// TODO: MultiStatus.
					return cStatus, cErr
				}
			}
		}

	} else {
		dstFile, err := fs.OpenFile(ctx, dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, srcPerm)
		if err != nil {
			if os.IsNotExist(err) {
				return http.StatusConflict, err
			}
			return http.StatusForbidden, err

		}
		_, copyErr := io.Copy(dstFile, srcFile)
		propsErr := CopyProps(dstFile, srcFile)
		closeErr := dstFile.Close()
		if copyErr != nil {
			return http.StatusInternalServerError, copyErr
		}
		if propsErr != nil {
			return http.StatusInternalServerError, propsErr
		}
		if closeErr != nil {
			return http.StatusInternalServerError, closeErr
		}
	}

	if created {
		return http.StatusCreated, nil
	}
	return http.StatusNoContent, nil
}

func CopyProps(dst, src File) error {
	d, ok := dst.(DeadPropsHolder)
	if !ok {
		return nil
	}
	s, ok := src.(DeadPropsHolder)
	if !ok {
		return nil
	}
	m, err := s.DeadProps()
	if err != nil {
		return err
	}
	props := make([]Property, 0, len(m))
	for _, prop := range m {
		props = append(props, prop)
	}
	_, err = d.Patch([]Proppatch{{Props: props}})
	return err
}

// moveFiles moves files and/or directories from src to dst.
//
// See section 9.9.4 for when various HTTP status codes apply.
func MoveFiles(ctx context.Context, fs webdav.FileSystem, src, dst string, overwrite bool) (status int, err error) {
	created := false
	if _, err := fs.Stat(ctx, dst); err != nil {
		if !os.IsNotExist(err) {
			return http.StatusForbidden, err
		}
		created = true
	} else if overwrite {
		// Section 9.9.3 says that "If a resource exists at the destination
		// and the Overwrite header is "T", then prior to performing the move,
		// the server must perform a DELETE with "Depth: infinity" on the
		// destination resource.
		if err := fs.RemoveAll(ctx, dst); err != nil {
			return http.StatusForbidden, err
		}
	} else {
		return http.StatusPreconditionFailed, os.ErrExist
	}
	if err := fs.Rename(ctx, src, dst); err != nil {
		return http.StatusForbidden, err
	}
	if created {
		return http.StatusCreated, nil
	}
	return http.StatusNoContent, nil
}
