package immich

import (
	"time"
)

type Asset struct {
	ID            string    `json:"id"`
	FileCreatedAt time.Time `json:"fileCreatedAt"`
}

type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Album struct {
	ID        string `json:"id"`
	AlbumName string `json:"albumName"`
}

// AlbumInfo is the album detail response. Immich no longer includes the asset
// list in this payload (only assetCount), so callers should fetch assets with
// GetAlbumInfo, which falls back to the search API.
type AlbumInfo struct {
	ID         string  `json:"id"`
	AlbumName  string  `json:"albumName"`
	AssetCount int     `json:"assetCount"`
	Assets     []Asset `json:"assets"`
}

type SearchMetadataRequest struct {
	PersonIds   []string `json:"personIds,omitempty"`
	TakenAfter  string   `json:"takenAfter,omitempty"`  // capture time, i.e. asset.fileCreatedAt
	TakenBefore string   `json:"takenBefore,omitempty"` // capture time, i.e. asset.fileCreatedAt
	AlbumIds    []string `json:"albumIds,omitempty"`    // filter assets by album; Immich expects an array
	Page        int      `json:"page,omitempty"`        // 1-based page number, used by Immich < v3.2.0
	Size        int      `json:"size,omitempty"`        // page size, Immich caps this at 1000
	Cursor      string   `json:"cursor,omitempty"`      // opaque next-page cursor, Immich >= v3.2.0
}

// BulkIdsRequest is the body Immich expects for bulk album asset operations
// (PUT/DELETE /api/albums/{id}/assets). The field is "ids", not "assetIds".
type BulkIdsRequest struct {
	Ids []string `json:"ids"`
}

type CreateAlbumRequest struct {
	AlbumName string   `json:"albumName"`
	AssetIds  []string `json:"assetIds,omitempty"`
}
