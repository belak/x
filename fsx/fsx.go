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
