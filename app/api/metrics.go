package main

import (
	"fmt"
	"io"
	"net/http"
)

// metrics exposes product numbers in the Prometheus text format for the in-cluster scraper.
// The ingress does not route /metrics, so it is not reachable from the internet.
func (s *server) metrics(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Stats(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	gauge(w, "maiday_users", "Registered accounts.", st.Users)
	gaugeHeader(w, "maiday_active_users", "Accounts that saved something within the window.")
	fmt.Fprintf(w, "maiday_active_users{window=\"1d\"} %d\n", st.ActiveUsers1d)
	fmt.Fprintf(w, "maiday_active_users{window=\"7d\"} %d\n", st.ActiveUsers7d)
	fmt.Fprintf(w, "maiday_active_users{window=\"30d\"} %d\n", st.ActiveUsers30d)
	gauge(w, "maiday_active_sessions", "Signed-in sessions that have not expired.", st.ActiveSessions)
	gauge(w, "maiday_assets", "Uploaded files.", st.Assets)
	gauge(w, "maiday_asset_bytes", "Total size of uploaded files in bytes.", st.AssetBytes)

	gaugeHeader(w, "maiday_items", "Stored entries per collection.")
	for _, c := range collections {
		fmt.Fprintf(w, "maiday_items{collection=%q} %d\n", c, st.ItemsByCollection[c])
	}
}

func gaugeHeader(w io.Writer, name, help string) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
}

func gauge(w io.Writer, name, help string, value int64) {
	gaugeHeader(w, name, help)
	fmt.Fprintf(w, "%s %d\n", name, value)
}
