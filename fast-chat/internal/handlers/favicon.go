package handlers

import (
	_ "embed"
	"net/http"
)

//go:embed favicon.svg
var faviconSVG string

// FaviconHandler serves a lightweight SVG favicon
func FaviconHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=31536000") // Cache for 1 year
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(faviconSVG))
	}
}
