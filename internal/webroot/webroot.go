package webroot

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// The embedded assets are the OFFICIAL Immich web UI, built from the pinned
// immich release (see scripts/build-web.sh) and copied into webui/ at build
// time. This is the product UI — not a hand-written replacement (per
// AGENTS.md hard rule 6).
//
//go:embed all:webui
var rootFS embed.FS

// Serve replies with an embedded static file when it exists, otherwise falls
// back to the SPA entry point (webui/index.html) so client-side routing works.
// It is wired to gin's NoRoute in main.go for every non-/api request.
func Serve(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	// defend against path traversal
	if strings.Contains(p, "..") {
		p = "index.html"
	}
	full := "webui/" + p
	if data, err := rootFS.ReadFile(full); err == nil {
		w.Header().Set("Content-Type", contentType(p))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}
	// SPA fallback — any unknown path loads the app shell. The official web
	// handles client-side routes (incl. /share/<key>) via this fallback.
	data, err := rootFS.ReadFile("webui/index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// Index returns the raw SPA entry point (kept for backwards compatibility).
func Index() ([]byte, error) {
	return rootFS.ReadFile("webui/index.html")
}

func contentType(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js", ".mjs":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".wasm":
		return "application/wasm"
	case ".woff", ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".ico":
		return "image/x-icon"
	case ".map":
		return "application/json; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// ensure embed.FS is referenced even if only sub-paths are used.
var _ fs.FS = rootFS
