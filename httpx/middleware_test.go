package httpx_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/belak/x/httpx"
)

func tagMiddleware(tag string) httpx.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Tag", tag)
			next.ServeHTTP(w, r)
		})
	}
}

func TestByMethodReadVsWrite(t *testing.T) {
	t.Parallel()

	mw := httpx.ByMethod(tagMiddleware("read"), tagMiddleware("write"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		method string
		want   string
	}{
		{http.MethodGet, "read"},
		{http.MethodHead, "read"},
		{http.MethodOptions, "read"},
		{http.MethodPost, "write"},
		{http.MethodPut, "write"},
		{http.MethodPatch, "write"},
		{http.MethodDelete, "write"},
		{"BREW", "write"},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(tc.method, "/", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			assert.Equal(t, tc.want, w.Header().Get("X-Tag"))
		})
	}
}

func TestByMethodExtraReadMethods(t *testing.T) {
	t.Parallel()
	mw := httpx.ByMethod(tagMiddleware("read"), tagMiddleware("write"), "PROPFIND")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("PROPFIND", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, "read", w.Header().Get("X-Tag"))
}

func TestByMethodNilReadOrWrite(t *testing.T) {
	t.Parallel()

	mw := httpx.ByMethod(nil, tagMiddleware("write"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, "", w.Header().Get("X-Tag"))

	req = httptest.NewRequest(http.MethodPost, "/", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, "write", w.Header().Get("X-Tag"))
}

func TestCSPSetsHeader(t *testing.T) {
	t.Parallel()
	policy := "default-src 'self'"
	handler := httpx.CSP(policy)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, policy, w.Header().Get("Content-Security-Policy"))
}

func TestWrap(t *testing.T) {
	t.Parallel()

	var order []string
	mw := func(tag string) httpx.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, tag)
				next.ServeHTTP(w, r)
			})
		}
	}

	endpoint := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "endpoint")
	})

	assert.Equal(t, http.Handler(endpoint), httpx.Wrap(endpoint))

	h := httpx.Wrap(endpoint, nil, mw("m1"), nil, mw("m2"))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, []string{"m1", "m2", "endpoint"}, order)
}

func TestLogging_Route(t *testing.T) {
	t.Parallel()

	cases := []struct {
		pattern string
		path    string
		want    string
	}{
		{"GET /items/{id}", "/items/42", "/items/{id}"},
		{"GET\t/items/{id}", "/items/42", "/items/{id}"},
		{"", "/not-found", ""},
	}

	for _, tc := range cases {
		t.Run(tc.pattern, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))

			router := httpx.NewRouter()
			if tc.pattern != "" {
				router.Handle(tc.pattern, func(w http.ResponseWriter, r *http.Request) {})
			}
			httpx.Wrap(router, httpx.Logging(logger)).ServeHTTP(
				httptest.NewRecorder(),
				httptest.NewRequest(http.MethodGet, tc.path, nil),
			)

			var entry map[string]any
			assert.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
			httpGroup := entry["http"].(map[string]any)
			assert.Equal(t, tc.path, httpGroup["path"].(string))
			if tc.want != "" {
				assert.Equal(t, tc.want, httpGroup["route"].(string))
			} else {
				assert.Equal(t, nil, httpGroup["route"])
			}
		})
	}
}
