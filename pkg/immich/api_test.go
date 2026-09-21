package immich

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("could not parse time %q: %v", value, err)
	}
	return parsed
}

func assetIDs(assets []Asset) []string {
	ids := make([]string, 0, len(assets))
	for _, a := range assets {
		ids = append(ids, a.ID)
	}
	return ids
}

// newStubServer replies with the queued responses in order and records the
// request bodies it received.
func newStubServer(t *testing.T, responses []string) (*httptest.Server, *[]SearchMetadataRequest) {
	t.Helper()

	var requests []SearchMetadataRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req SearchMetadataRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("could not decode request body: %v", err)
		}
		requests = append(requests, req)

		if len(requests) > len(responses) {
			t.Errorf("server received %d requests, only %d responses queued", len(requests), len(responses))
			http.Error(w, "no more responses", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responses[len(requests)-1]))
	}))

	t.Cleanup(server.Close)
	return server, &requests
}

func TestSearchMetadataFollowsNextCursor(t *testing.T) {
	server, requests := newStubServer(t, []string{
		`{"assets":{"items":[{"id":"1"},{"id":"2"}],"nextCursor":"cursor-2"}}`,
		`{"assets":{"items":[{"id":"3"}],"nextCursor":"cursor-3"}}`,
		`{"assets":{"items":[{"id":"4"}],"nextCursor":null}}`,
	})

	client := NewClient(server.URL, "test-key")
	assets, err := client.GetAssetsByPeople([]string{"person-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := assetIDs(assets)
	want := []string{"1", "2", "3", "4"}
	if len(got) != len(want) {
		t.Fatalf("expected %d assets, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("asset %d: expected %q, got %q", i, want[i], got[i])
		}
	}

	if len(*requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(*requests))
	}
	if (*requests)[0].Cursor != "" || (*requests)[0].Page != 1 {
		t.Errorf("first request should start on page 1 with no cursor, got %+v", (*requests)[0])
	}
	if (*requests)[1].Cursor != "cursor-2" || (*requests)[1].Page != 0 {
		t.Errorf("second request should use the cursor from the first response, got %+v", (*requests)[1])
	}
	if (*requests)[2].Cursor != "cursor-3" {
		t.Errorf("third request should use the cursor from the second response, got %+v", (*requests)[2])
	}
}

func TestSearchMetadataFollowsNextPage(t *testing.T) {
	server, requests := newStubServer(t, []string{
		`{"assets":{"items":[{"id":"1"}],"nextPage":"2"}}`,
		`{"assets":{"items":[{"id":"2"}],"nextPage":null}}`,
	})

	client := NewClient(server.URL, "test-key")
	assets, err := client.SearchAssetsByDateRange(
		mustParseTime(t, "2026-09-01T00:00:00Z"),
		mustParseTime(t, "2026-09-20T00:00:00Z"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := assetIDs(assets)
	if len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Fatalf("expected [1 2], got %v", got)
	}

	if len(*requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(*requests))
	}
	if (*requests)[1].Page != 2 || (*requests)[1].Cursor != "" {
		t.Errorf("second request should ask for page 2, got %+v", (*requests)[1])
	}
	for _, req := range *requests {
		if req.Size != searchMetadataPageSize {
			t.Errorf("expected size %d, got %d", searchMetadataPageSize, req.Size)
		}
	}
}

// Immich filters takenAfter/takenBefore on the capture time, while
// createdAfter/createdBefore filter on upload time, so the date range search must
// send the former.
func TestSearchAssetsByDateRangeUsesTakenFields(t *testing.T) {
	var rawBody map[string]any
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
			t.Errorf("could not decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"assets":{"items":[],"nextCursor":null}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	after := mustParseTime(t, "2026-09-01T00:00:00Z")
	before := mustParseTime(t, "2026-09-20T00:00:00Z")
	if _, err := client.SearchAssetsByDateRange(after, before); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if path != "/api/search/metadata" {
		t.Errorf("expected /api/search/metadata, got %q", path)
	}
	if got := rawBody["takenAfter"]; got != after.Format(time.RFC3339) {
		t.Errorf("expected takenAfter %q, got %v", after.Format(time.RFC3339), got)
	}
	if got := rawBody["takenBefore"]; got != before.Format(time.RFC3339) {
		t.Errorf("expected takenBefore %q, got %v", before.Format(time.RFC3339), got)
	}
	for _, unwanted := range []string{"createdAfter", "createdBefore"} {
		if _, ok := rawBody[unwanted]; ok {
			t.Errorf("request must not send %q (that filters on upload time): %v", unwanted, rawBody)
		}
	}
}

// Immich treats personIds as "must contain every person", so searching for all
// people at once would return only the intersection. GetAssetsByPeople must query
// each person separately and merge the results to get the union.
func TestGetAssetsByPeopleUnionsPerPersonSearches(t *testing.T) {
	server, requests := newStubServer(t, []string{
		`{"assets":{"items":[{"id":"1"},{"id":"2"}],"nextCursor":null}}`,
		`{"assets":{"items":[{"id":"2"},{"id":"3"}],"nextCursor":null}}`,
	})

	client := NewClient(server.URL, "test-key")
	assets, err := client.GetAssetsByPeople([]string{"person-1", "person-2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := assetIDs(assets)
	want := []string{"1", "2", "3"}
	if len(got) != len(want) {
		t.Fatalf("expected %d assets, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("asset %d: expected %q, got %q", i, want[i], got[i])
		}
	}

	if len(*requests) != 2 {
		t.Fatalf("expected one request per person, got %d", len(*requests))
	}
	for i, personID := range []string{"person-1", "person-2"} {
		if len((*requests)[i].PersonIds) != 1 || (*requests)[i].PersonIds[0] != personID {
			t.Errorf("request %d should search only %q, got %v", i, personID, (*requests)[i].PersonIds)
		}
	}
}

func TestSearchMetadataEmptyResultIsNotAnError(t *testing.T) {
	server, requests := newStubServer(t, []string{
		`{"assets":{"items":[],"total":0,"count":0,"nextCursor":null}}`,
	})

	client := NewClient(server.URL, "test-key")
	assets, err := client.GetAssetsByPeople([]string{"person-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assets) != 0 {
		t.Fatalf("expected no assets, got %v", assetIDs(assets))
	}
	if len(*requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(*requests))
	}
}

func TestSearchMetadataStopsOnRepeatedCursor(t *testing.T) {
	server, requests := newStubServer(t, []string{
		`{"assets":{"items":[{"id":"1"}],"nextCursor":"same"}}`,
		`{"assets":{"items":[{"id":"2"}],"nextCursor":"same"}}`,
	})

	client := NewClient(server.URL, "test-key")
	assets, err := client.GetAssetsByPeople([]string{"person-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 assets, got %v", assetIDs(assets))
	}
	if len(*requests) != 2 {
		t.Fatalf("expected the loop to stop after 2 requests, got %d", len(*requests))
	}
}

func TestParseSearchResultShapes(t *testing.T) {
	t.Run("nested envelope with cursor", func(t *testing.T) {
		result, err := parseSearchResult([]byte(`{"assets":{"items":[{"id":"1"}],"nextCursor":"abc"}}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Items) != 1 || result.NextCursor != "abc" {
			t.Fatalf("unexpected result: %+v", result)
		}
	})

	t.Run("empty envelope", func(t *testing.T) {
		result, err := parseSearchResult([]byte(`{"assets":{"items":[],"nextPage":null}}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Items) != 0 || result.NextPage != "" {
			t.Fatalf("unexpected result: %+v", result)
		}
	})

	t.Run("flat array", func(t *testing.T) {
		result, err := parseSearchResult([]byte(`[{"id":"1"},{"id":"2"}]`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Items) != 2 {
			t.Fatalf("unexpected result: %+v", result)
		}
	})

	t.Run("unparseable", func(t *testing.T) {
		if _, err := parseSearchResult([]byte(`not json`)); err == nil {
			t.Fatal("expected an error for an unparseable response")
		}
	})
}

// Immich searches people via GET /api/search/person with the name as a query
// parameter; the POST form the client used previously is rejected.
func TestSearchPeopleByNameUsesGetWithQuery(t *testing.T) {
	var method, path, name string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		name = r.URL.Query().Get("name")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"person-1","name":"Alice Smith"}]`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	people, err := client.SearchPeopleByName("Alice Smith")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if method != http.MethodGet {
		t.Errorf("expected GET, got %s", method)
	}
	if path != "/api/search/person" {
		t.Errorf("expected /api/search/person, got %q", path)
	}
	if name != "Alice Smith" {
		t.Errorf("expected name query param %q, got %q", "Alice Smith", name)
	}
	if len(people) != 1 || people[0].ID != "person-1" || people[0].Name != "Alice Smith" {
		t.Fatalf("unexpected people: %+v", people)
	}
}
