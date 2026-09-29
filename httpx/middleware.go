package httpx

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"sync"

	"github.com/belak/x/slogx"
	"github.com/felixge/httpsnoop"
)

// Middleware is the standard middleware signature.
type Middleware func(http.Handler) http.Handler

// Wrap chains middlewares around any http.Handler. The first middleware in
// the slice is outermost (runs first).
//
// Recommended transport order:
//
//	httpx.Wrap(router,
//	    httpx.WithRequestID,
//	    httpx.Logging(logger),
//	    httpx.Recovery(logger, nil),
//	    httpx.SecurityHeaders,
//	)
func Wrap(h http.Handler, mws ...Middleware) http.Handler {
	for _, mw := range slices.Backward(mws) {
		if mw != nil {
			h = mw(h)
		}
	}
	return h
}

const panicAttrKey = "error.panic"

type contextKey string

const requestIDKey contextKey = "request_id"

var requestIDHeader = "X-Request-ID"

// GetRequestID retrieves the request ID from the context, or empty string.
func GetRequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// WithRequestID adds a unique request ID to each request. If the
// request already has an X-Request-ID header (e.g. from a load
// balancer), it is reused. The ID is also stored on the context and
// retrievable via GetRequestID.
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = generateRequestID()
		}

		ctx := context.WithValue(r.Context(), requestIDKey, id)
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "req_error"
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

type logAccumulatorKey struct{}

type logAccumulator struct {
	mu    sync.Mutex
	attrs []slog.Attr
}

func (a *logAccumulator) add(attrs ...slog.Attr) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.attrs = append(a.attrs, attrs...)
}

func (a *logAccumulator) getAttrs() []slog.Attr {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]slog.Attr(nil), a.attrs...)
}

func (a *logAccumulator) hasAttr(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, attr := range a.attrs {
		if attr.Key == key {
			return true
		}
	}
	return false
}

// AddLogAttrs attaches attributes to the request's canonical log line.
// Attributes are buffered and emitted once when Logging completes. If
// Logging is not in the middleware chain, AddLogAttrs is a safe no-op.
func AddLogAttrs(ctx context.Context, attrs ...slog.Attr) {
	if acc, ok := ctx.Value(logAccumulatorKey{}).(*logAccumulator); ok && acc != nil {
		acc.add(attrs...)
	}
}

// Logging creates middleware that emits a single canonical log line (or "wide
// event") per request, capturing method, path, route pattern, status code,
// duration, and bytes written.
//
// Handlers and inner middleware can attach contextual attributes to this line
// using AddLogAttrs rather than emitting separate log records (see
// https://stripe.com/blog/canonical-log-lines or https://loggingsucks.com).
func Logging(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			acc := &logAccumulator{}
			ctx := context.WithValue(r.Context(), logAccumulatorKey{}, acc)

			requestID := GetRequestID(ctx)

			// Child logger with request ID for downstream handlers.
			reqLogger := logger.With(slogx.String("request_id", requestID))
			ctx = slogx.WithLogger(ctx, reqLogger)
			r = r.WithContext(ctx)

			m := httpsnoop.CaptureMetrics(next, w, r)

			level := slog.LevelInfo
			if m.Code >= 500 {
				level = slog.LevelError
			} else if m.Code >= 400 {
				level = slog.LevelWarn
			}
			if acc.hasAttr(panicAttrKey) {
				level = slog.LevelError
			}

			route := r.Pattern
			if i := strings.IndexAny(r.Pattern, " \t"); i >= 0 {
				route = strings.TrimSpace(r.Pattern[i:])
			}

			httpAttrs := []any{
				slogx.String("method", r.Method),
				slogx.String("path", r.URL.Path),
			}
			if route != "" {
				httpAttrs = append(httpAttrs, slogx.String("route", route))
			}
			httpAttrs = append(httpAttrs,
				slogx.Int("status", m.Code),
				slogx.Duration("duration", m.Duration),
				slogx.Int64("bytes", m.Written),
			)

			attrs := []slog.Attr{
				slogx.Group("http", httpAttrs...),
			}
			attrs = append(attrs, acc.getAttrs()...)

			reqLogger.LogAttrs(ctx, level, "http request", attrs...)
		})
	}
}

// PanicHandlerFunc handles a recovered panic. The third argument is the value
// passed to panic — inspect its type to decide what to show the user.
type PanicHandlerFunc func(http.ResponseWriter, *http.Request, any)

// Recovery creates middleware that recovers from panics, logs the error and
// stack trace, then calls errHandler. If errHandler is nil, a plain-text 500
// response is written.
func Recovery(logger *slog.Logger, errHandler PanicHandlerFunc) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					AddLogAttrs(r.Context(), slogx.Any(panicAttrKey, err))
					logger.Error("panic recovered",
						slogx.Any("error", err),
						slogx.String("method", r.Method),
						slogx.String("path", r.URL.Path),
						slogx.String("stack", string(debug.Stack())),
					)
					if errHandler != nil {
						errHandler(w, r, err)
					} else {
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					}
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders adds standard security headers to every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}

// ByMethod returns middleware that dispatches to read for safe methods
// (GET, HEAD, OPTIONS, plus any extraReadMethods such as "PROPFIND")
// and write for everything else. Either middleware may be nil to skip
// wrapping for that side.
func ByMethod(read, write Middleware, extraReadMethods ...string) Middleware {
	safe := map[string]struct{}{
		http.MethodGet:     {},
		http.MethodHead:    {},
		http.MethodOptions: {},
	}
	for _, m := range extraReadMethods {
		safe[m] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		readChain := next
		if read != nil {
			readChain = read(next)
		}
		writeChain := next
		if write != nil {
			writeChain = write(next)
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := safe[r.Method]; ok {
				readChain.ServeHTTP(w, r)
				return
			}
			writeChain.ServeHTTP(w, r)
		})
	}
}

// CSP returns middleware that sets a Content-Security-Policy header on
// every response.
func CSP(policy string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy", policy)
			next.ServeHTTP(w, r)
		})
	}
}
