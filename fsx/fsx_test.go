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

func TestMergedFS(t *testing.T) {
	auth := fstest.MapFS{
		"login.html": {Data: []byte("<html>login</html>")},
		"shared.css": {Data: []byte("auth")},
	}
	common := fstest.MapFS{
		"style.css":  {Data: []byte("<css>common</css>")},
		"shared.css": {Data: []byte("common")},
	}

	fs := fsx.MergedFS{http.FS(auth), http.FS(common)}

	tests := []struct {
		path    string
		want    string
		wantErr error
	}{
		{path: "/login.html", want: "<html>login</html>"},
		{path: "/style.css", want: "<css>common</css>"},
		{path: "/shared.css", want: "auth"}, // first match wins
		{path: "/missing", wantErr: os.ErrNotExist},
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
				t.Fatalf("Open(%q) unexpected error: %v", tt.path, err)
			}
			defer f.Close()

			buf := make([]byte, 64)
			n, _ := f.Read(buf)
			if got := string(buf[:n]); got != tt.want {
				t.Errorf("Open(%q) content = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestMergedFSEmpty(t *testing.T) {
	fs := fsx.MergedFS{}

	_, err := fs.Open("/anything")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Open() error = %v, want %v", err, os.ErrNotExist)
	}
}
