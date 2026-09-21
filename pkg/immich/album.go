package immich

import (
	"encoding/json"
	"fmt"
	"net/url"
)

func (c *Client) GetAlbumByName(name string) (*Album, error) {
	// Try /api/albums first (newer Immich versions)
	res, err := c.doRequest("GET", "/api/albums?name="+url.QueryEscape(name), nil)
	if err != nil {
		// fallback to /api/album
		res, err = c.doRequest("GET", "/api/album?name="+url.QueryEscape(name), nil)
		if err != nil {
			return nil, err
		}
	}

	var albums []Album
	if err := json.Unmarshal(res, &albums); err != nil {
		return nil, err
	}

	// API might still return multiple or partial matches, return exact match
	for _, a := range albums {
		if a.AlbumName == name {
			return &a, nil
		}
	}

	return nil, nil // not found
}

func (c *Client) GetAlbumInfo(id string) (*AlbumInfo, error) {
	res, err := c.doRequest("GET", fmt.Sprintf("/api/albums/%s", id), nil)
	if err != nil {
		res, err = c.doRequest("GET", fmt.Sprintf("/api/album/%s", id), nil)
		if err != nil {
			return nil, err
		}
	}

	var info AlbumInfo
	if err := json.Unmarshal(res, &info); err != nil {
		return nil, err
	}

	// Immich's album detail response only carries assetCount, not the assets
	// themselves, so fall back to the search API to enumerate album contents.
	if len(info.Assets) == 0 {
		if assets, err := c.searchMetadata(SearchMetadataRequest{AlbumIds: []string{id}}); err == nil {
			info.Assets = assets
		}
	}

	return &info, nil
}

func (c *Client) CreateAlbum(name string) (*Album, error) {
	req := CreateAlbumRequest{
		AlbumName: name,
	}
	res, err := c.doRequest("POST", "/api/albums", req)
	if err != nil {
		res, err = c.doRequest("POST", "/api/album", req)
		if err != nil {
			return nil, err
		}
	}

	var album Album
	if err := json.Unmarshal(res, &album); err != nil {
		return nil, err
	}
	return &album, nil
}

func (c *Client) AddAssetsToAlbum(albumId string, assetIds []string) error {
	if len(assetIds) == 0 {
		return nil
	}
	req := BulkIdsRequest{
		Ids: assetIds,
	}
	// For Immich PUT /api/albums/{id}/assets adds items to album
	_, err := c.doRequest("PUT", fmt.Sprintf("/api/albums/%s/assets", albumId), req)
	if err != nil {
		// Fallback
		_, err = c.doRequest("PUT", fmt.Sprintf("/api/album/%s/assets", albumId), req)
	}
	return err
}

func (c *Client) RemoveAssetsFromAlbum(albumId string, assetIds []string) error {
	if len(assetIds) == 0 {
		return nil
	}
	req := BulkIdsRequest{
		Ids: assetIds,
	}

	_, err := c.doRequest("DELETE", fmt.Sprintf("/api/albums/%s/assets", albumId), req)
	if err != nil {
		// try fallback
		_, err = c.doRequest("DELETE", fmt.Sprintf("/api/album/%s/assets", albumId), req)
	}

	return err
}
