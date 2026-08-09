package webroot

import (
	"embed"
	"net/http"
)

//go:embed index.html
var indexFS embed.FS

// Index returns the embedded SPA entry point.
func Index() ([]byte, error) {
	return indexFS.ReadFile("index.html")
}

// Serve replies with the embedded SPA for any non-API route. It always
// responds 200 so the SPA entry point loads correctly (gin's NoRoute would
// otherwise default to 404 even after we write the body).
func Serve(w http.ResponseWriter, _ *http.Request) {
	data, err := Index()
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
