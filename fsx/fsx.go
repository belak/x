package fsx

import (
	"io/fs"
	"os"
	"path"
)

// NoListFS wraps an fs.FS and disables directory listings by returning
// os.ErrNotExist for any directory that lacks an index.html.
type NoListFS struct {
	FS fs.FS
}

func (n NoListFS) Open(name string) (fs.File, error) {
	f, err := n.FS.Open(name)
	if err != nil {
		return nil, err
	}

	s, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	if s.IsDir() {
		if _, err := n.FS.Open(path.Join(name, "index.html")); err != nil {
			f.Close()
			return nil, os.ErrNotExist
		}
	}

	return f, nil
}

// MergedFS combines multiple fs.FSs into one, trying each in order and
// returning the first successful match. This is useful for serving assets from
// several sources, such as combining a shared "common" package with an
// app-specific one.
type MergedFS []fs.FS

func (m MergedFS) Open(name string) (fs.File, error) {
	var err error

	for _, fsys := range m {
		var f fs.File

		f, err = fsys.Open(name)
		if err == nil {
			return f, nil
		}
	}

	if err == nil {
		err = os.ErrNotExist
	}

	return nil, err
}
