package serve

import (
	_ "embed"
	"net/http"
)

// The web UI is split into three embedded assets so each stays easy to edit:
// markup, styles and behaviour. They are served from the binary directly.

//go:embed static/index.html
var indexHTML []byte

//go:embed static/style.css
var styleCSS []byte

//go:embed static/app.js
var appJS []byte

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

func handleStyleCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write(styleCSS)
}

func handleAppJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write(appJS)
}
