package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// SearchResult is one resource match for the command palette. Only data the
// caller can already read through the regular list endpoints is searched.
type SearchResult struct {
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Link     string `json:"link"`
}

const (
	searchPerKind     = 5
	searchMaxRecords  = 50000
	searchMaxQueryLen = 100
)

type searchSources struct {
	zones      ZoneStore
	clients    ClientStore
	rewrites   RewriteStore
	forwarding ForwardingStore
	blocklists BlocklistStore
}

func registerSearch(mux *http.ServeMux, sources searchSources) {
	mux.HandleFunc("GET /api/v1/search", func(w http.ResponseWriter, r *http.Request) {
		query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		if utf8.RuneCountInString(query) < 2 || len(query) > searchMaxQueryLen {
			failure(w, 400, "invalid_query", "Search for 2 to 100 characters")
			return
		}
		respond(w, 200, search(r.Context(), sources, query))
	})
}

func search(ctx context.Context, sources searchSources, query string) []SearchResult {
	out := []SearchResult{}
	matches := func(values ...string) bool {
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), query) {
				return true
			}
		}
		return false
	}
	add := func(kind string, results *int, item SearchResult) bool {
		if *results >= searchPerKind {
			return false
		}
		item.Kind = kind
		out = append(out, item)
		*results++
		return true
	}
	if sources.zones != nil {
		zoneCount, recordCount, scanned := 0, 0, 0
		for _, zone := range sources.zones.List() {
			if matches(zone.Name) {
				add("zone", &zoneCount, SearchResult{Title: zone.Name, Subtitle: zone.PrimaryNS, Link: "/zones?zone=" + strconv.FormatInt(zone.ID, 10)})
			}
			for _, record := range zone.Records {
				// Bound the work for very large installations.
				if scanned++; scanned > searchMaxRecords || recordCount >= searchPerKind {
					break
				}
				if matches(record.Name, record.Value) {
					add("record", &recordCount, SearchResult{Title: record.Name + " " + record.Type, Subtitle: record.Value, Link: "/zones?zone=" + strconv.FormatInt(zone.ID, 10)})
				}
			}
		}
	}
	if sources.clients != nil {
		if list, err := sources.clients.List(ctx); err == nil {
			count := 0
			for _, client := range list {
				if matches(append([]string{client.Name, client.Group}, client.Addresses...)...) {
					add("client", &count, SearchResult{Title: client.Name, Subtitle: strings.Join(client.Addresses, ", "), Link: "/clients?name=" + url.QueryEscape(client.Name)})
				}
			}
		}
	}
	if sources.rewrites != nil {
		if list, err := sources.rewrites.List(ctx); err == nil {
			count := 0
			for _, rule := range list {
				if matches(rule.Name, rule.Value, rule.Description) {
					add("rewrite", &count, SearchResult{Title: rule.Name + " " + rule.Type, Subtitle: rule.Value, Link: "/rewrites?q=" + url.QueryEscape(rule.Name)})
				}
			}
		}
	}
	if sources.forwarding != nil {
		if list, err := sources.forwarding.List(ctx); err == nil {
			count := 0
			for _, rule := range list {
				if matches(rule.Domain, rule.Description) {
					add("forwarding", &count, SearchResult{Title: rule.Domain, Subtitle: strings.Join(rule.Upstreams, ", "), Link: "/forwarding"})
				}
			}
		}
	}
	if sources.blocklists != nil {
		if list, err := sources.blocklists.List(ctx); err == nil {
			count := 0
			for _, source := range list {
				if matches(source.Name) {
					add("blocklist", &count, SearchResult{Title: source.Name, Subtitle: source.URL, Link: "/blocklists"})
				}
			}
		}
	}
	return out
}
