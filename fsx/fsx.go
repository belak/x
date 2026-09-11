package fsx

import (
	"net/http"
	"os"
	"path/filepath"
)

// NoListFS wraps an http.FileSystem and disables directory listings by
// returning os.ErrNotExist for any directory that lacks an index.html.
type NoListFS struct {
	FS http.FileSystem
}

func (n NoListFS) Open(path string) (http.File, error) {
	f, err := n.FS.Open(path)
	if err != nil {
		return nil, err
	}

	s, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	if s.IsDir() {
		if _, err := n.FS.Open(filepath.Join(path, "index.html")); err != nil {
			f.Close()
			return nil, os.ErrNotExist
		}
	}

	return f, nil
}

// MergedFS combines multiple http.FileSystems into one, trying each in
// order and returning the first successful match. This is useful for
// serving assets from several sources, such as combining a shared
// "common" package with an app-specific one.
type MergedFS []http.FileSystem

func (m MergedFS) Open(path string) (http.File, error) {
	var err error

	for _, fs := range m {
		var f http.File

		f, err = fs.Open(path)
		if err == nil {
			return f, nil
		}
	}

	if err == nil {
		err = os.ErrNotExist
	}

	return nil, err
}
