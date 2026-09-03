package main

import "net/http"

// uiHandler returns the embedded single-page app handler, or nil to run the
// API without a UI. The embedded frontend is wired in a later milestone; until
// then Cronus serves the REST API and /metrics only.
func uiHandler() http.Handler {
	return nil
}
