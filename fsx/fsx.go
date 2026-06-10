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
