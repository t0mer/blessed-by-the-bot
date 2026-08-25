// Package webui embeds the built frontend and serves it as a single-page app.
package webui

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var embedded embed.FS

// placeholder is served when dist holds no built index.html. Only .gitkeep is
// tracked in that directory — committing a built index.html would reference
// hashed asset files that are themselves ignored, so a fresh clone would embed
// a page pointing at scripts that do not exist.
const placeholder = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>blessed-by-the-bot</title>
  </head>
  <body>
    <p>UI not built. Run <code>make web</code> to build the frontend.</p>
  </body>
</html>
`

// Handler serves the embedded frontend. Requests that do not match a real file
// fall back to index.html so client-side routing works on deep links.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, fmt.Errorf("opening embedded dist: %w", err)
	}
	return HandlerFor(sub)
}

// HandlerFor serves an arbitrary filesystem as the SPA. Handler uses it with the
// embedded build; tests use it to exercise the not-yet-built case.
func HandlerFor(sub fs.FS) (http.Handler, error) {
	files := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		if _, statErr := fs.Stat(sub, name); statErr != nil {
			serveIndex(w, r, sub)
			return
		}
		files.ServeHTTP(w, r)
	}), nil
}

func serveIndex(w http.ResponseWriter, r *http.Request, sub fs.FS) {
	body, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		body = []byte(placeholder)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if _, err := w.Write(body); err != nil {
		return
	}
}
