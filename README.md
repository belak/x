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

## License

MIT
