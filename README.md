# github.com/belak/x

Personal Go utility packages.

## Packages

- **httpx** - HTTP middleware, routing, JSON helpers, flash messages, client IP resolution
- **migrate** - minimal SQL migration runner with multi-layer fs.FS support
- **slogx** - structured logging helpers wrapping `log/slog`
- **versionx** - VCS build info version extraction

### versionx

[godoc](https://pkg.go.dev/github.com/belak/x/versionx)

Extracts the application version from Go build info. Returns the VCS
revision (plus `-dirty` if the tree was modified), falling back to `"dev"`
or `"unknown"`. Accepts an optional override for ldflags stamping.

```go
// main.go
var version string // set via -ldflags "-X main.version=v1.2.3"

func main() {
    fmt.Println(versionx.Get(version)) // ldflags value if set, else VCS revision
    fmt.Println(versionx.GoVersion())  // e.g. "go1.22.0"
}
```

### slogx

[godoc](https://pkg.go.dev/github.com/belak/x/slogx)

Wraps `log/slog` with a few conveniences: a `New` function that creates and
sets the default logger in one call, context-based logger passing, and a
`Level` and `Format` type that both implement `flag.Value` for easy CLI
wiring.

```go
var (
    logLevel  slogx.Level  = slogx.LevelInfo
    logFormat slogx.Format = slogx.FormatPretty
)

flag.Var(&logLevel, "log-level", "debug, info, warn, error")
flag.Var(&logFormat, "log-format", "pretty, json, text")
flag.Parse()

logger := slogx.New(logFormat, logLevel)
logger.Info("started", slogx.String("version", version))

// Attach to context and retrieve downstream.
ctx = slogx.WithLogger(ctx, logger)
slogx.FromContext(ctx).Info("handling request", slogx.Err(err))
```

### httpx

[godoc](https://pkg.go.dev/github.com/belak/x/httpx)

HTTP server utilities built on top of `net/http`: a router with middleware
support and route grouping, JSON request/response helpers, flash messages,
client IP resolution behind trusted proxies, and graceful shutdown.

```go
logger := slogx.New(slogx.FormatPretty, slogx.LevelInfo)

r := httpx.NewRouter(logger)
r.Use(httpx.WithRequestID, httpx.Logging(logger), httpx.Recovery(logger))

r.Handle("GET /api/hello", func(w http.ResponseWriter, r *http.Request) {
    httpx.RespondJSON(w, http.StatusOK, map[string]string{"hello": "world"})
})

r.Group(func(r *httpx.Router) {
    r.Use(requireAuth)
    r.Handle("POST /api/items", createItem)
})

ctx, cancel := httpx.WithSignalShutdown(context.Background(), logger)
defer cancel()

r.ListenAndServe(ctx, ":8080")
```

## License

MIT
