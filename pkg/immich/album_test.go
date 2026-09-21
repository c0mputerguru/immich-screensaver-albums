package immich

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Body   map[string]any
}

// newAlbumStubServer records every request it receives and replies with a fixed
// body. Each call to the returned server returns the response for that request.
func newAlbumStubServer(t *testing.T, responses map[string]string) (*httptest.Server, *[]recordedRequest) {
	t.Helper()

	var requests []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record := recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
		}
		if r.Body != nil {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				record.Body = body
			}
		}
		requests = append(requests, record)

		w.Header().Set("Content-Type", "application/json")
		body, ok := responses[r.URL.Path]
		if !ok {
			t.Errorf("unexpected request to %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))

	t.Cleanup(server.Close)
	return server, &requests
}

// Immich expects the album asset add/remove body to use the "ids" key (the
// BulkIdsDto schema), not "assetIds". Sending the wrong key is silently rejected
// by the API and leaves the album unchanged.
func TestAddAssetsToAlbumSendsIdsField(t *testing.T) {
	server, requests := newAlbumStubServer(t, map[string]string{
		"/api/albums/album-1/assets": `[]`,
	})

	client := NewClient(server.URL, "test-key")
	if err := client.AddAssetsToAlbum("album-1", []string{"a", "b"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(*requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(*requests))
	}
	req := (*requests)[0]
	if req.Method != "PUT" {
		t.Errorf("expected PUT, got %s", req.Method)
	}
	if req.Path != "/api/albums/album-1/assets" {
		t.Errorf("unexpected path %q", req.Path)
	}
	ids, ok := req.Body["ids"].([]any)
	if !ok {
		t.Fatalf(`expected body key "ids", got %v`, req.Body)
	}
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Errorf("expected ids [a b], got %v", ids)
	}
	if _, ok := req.Body["assetIds"]; ok {
		t.Errorf(`body must not use the "assetIds" key: %v`, req.Body)
	}
}

func TestRemoveAssetsFromAlbumSendsIdsField(t *testing.T) {
	server, requests := newAlbumStubServer(t, map[string]string{
		"/api/albums/album-1/assets": `[]`,
	})

	client := NewClient(server.URL, "test-key")
	if err := client.RemoveAssetsFromAlbum("album-1", []string{"c", "d"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(*requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(*requests))
	}
	req := (*requests)[0]
	if req.Method != "DELETE" {
		t.Errorf("expected DELETE, got %s", req.Method)
	}
	ids, ok := req.Body["ids"].([]any)
	if !ok {
		t.Fatalf(`expected body key "ids", got %v`, req.Body)
	}
	if len(ids) != 2 || ids[0] != "c" || ids[1] != "d" {
		t.Errorf("expected ids [c d], got %v", ids)
	}
}

// The album detail endpoint only returns assetCount, so GetAlbumInfo must fall
// back to the search API. Immich filters by album with the "albumIds" array, not
// a scalar "albumId"; sending the wrong field returns every asset instead of the
// album's assets.
func TestGetAlbumInfoFallsBackToAlbumIdsSearch(t *testing.T) {
	server, requests := newAlbumStubServer(t, map[string]string{
		"/api/albums/album-1":  `{"id":"album-1","albumName":"Recent","assetCount":1}`,
		"/api/search/metadata": `{"assets":{"items":[{"id":"asset-1"}],"nextCursor":null}}`,
	})

	client := NewClient(server.URL, "test-key")
	info, err := client.GetAlbumInfo("album-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(info.Assets) != 1 || info.Assets[0].ID != "asset-1" {
		t.Fatalf("expected asset-1, got %v", assetIDs(info.Assets))
	}

	if len(*requests) != 2 {
		t.Fatalf("expected the detail request plus a search request, got %d", len(*requests))
	}
	search := (*requests)[1]
	if search.Path != "/api/search/metadata" {
		t.Fatalf("expected search fallback, got %s %s", search.Method, search.Path)
	}
	albumIds, ok := search.Body["albumIds"].([]any)
	if !ok {
		t.Fatalf(`expected body key "albumIds", got %v`, search.Body)
	}
	if len(albumIds) != 1 || albumIds[0] != "album-1" {
		t.Errorf("expected albumIds [album-1], got %v", albumIds)
	}
	if _, ok := search.Body["albumId"]; ok {
		t.Errorf(`body must not use the scalar "albumId" key: %v`, search.Body)
	}
}

// Immich's album list filter parameter is "name", not "albumName". An unknown
// parameter is ignored, so the server would return every album.
func TestGetAlbumByNameFiltersByNameParam(t *testing.T) {
	server, requests := newAlbumStubServer(t, map[string]string{
		"/api/albums": `[{"id":"album-1","albumName":"Recent"},{"id":"album-2","albumName":"Other"}]`,
	})

	client := NewClient(server.URL, "test-key")
	album, err := client.GetAlbumByName("Recent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if album == nil || album.ID != "album-1" {
		t.Fatalf("expected album-1, got %+v", album)
	}
	if len(*requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(*requests))
	}
	if got := (*requests)[0].Query; got != "name=Recent" {
		t.Errorf("expected query name=Recent, got %q", got)
	}
}
