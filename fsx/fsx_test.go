package fsx_test

import (
	"errors"
	"net/http"
	"os"
	"testing"
	"testing/fstest"

	"github.com/belak/x/fsx"
)

func TestNoListFS(t *testing.T) {
	mem := fstest.MapFS{
		"index.html":        {Data: []byte("<html>root</html>")},
		"assets/style.css":  {Data: []byte("body{}")},
		"listed/index.html": {Data: []byte("<html>listed</html>")},
	}

	fs := fsx.NoListFS{FS: http.FS(mem)}

	tests := []struct {
		path    string
		wantErr error
	}{
		{"/index.html", nil},
		{"/assets/style.css", nil},
		{"/listed/index.html", nil},
		{"/listed", nil},            // has index.html, allowed
		{"/assets", os.ErrNotExist}, // no index.html, blocked
		{"/missing", os.ErrNotExist},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			f, err := fs.Open(tt.path)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Open(%q) error = %v, want %v", tt.path, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("Open(%q) unexpected error: %v", tt.path, err)
				return
			}
			f.Close()
		})
	}
}
