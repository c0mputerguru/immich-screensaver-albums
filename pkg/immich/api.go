package immich

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// searchMetadataPageSize is the number of results requested per page. Immich
// rejects a "size" larger than 1000.
const searchMetadataPageSize = 1000

// searchResult is a single page of a /api/search/metadata response.
type searchResult struct {
	Items      []Asset
	NextCursor string
	NextPage   string
}

// searchMetadata issues POST /api/search/metadata requests until every matching
// asset has been retrieved. The endpoint is paginated, so a single request only
// returns the first page.
func (c *Client) searchMetadata(req SearchMetadataRequest) ([]Asset, error) {
	if req.Size < 1 {
		req.Size = searchMetadataPageSize
	}

	var all []Asset
	page := 1
	cursor := ""
	pageToken := ""

	for {
		pageReq := req
		if cursor != "" {
			pageReq.Cursor = cursor
		} else {
			pageReq.Page = page
		}

		res, err := c.doRequest("POST", "/api/search/metadata", pageReq)
		if err != nil {
			return nil, err
		}

		result, err := parseSearchResult(res)
		if err != nil {
			return nil, err
		}
		all = append(all, result.Items...)

		if len(result.Items) == 0 {
			return all, nil
		}

		switch {
		// Immich >= v3.2.0 returns an opaque cursor pointing at the next page.
		case result.NextCursor != "":
			if result.NextCursor == cursor {
				return all, nil // server ignored the cursor; stop instead of looping
			}
			cursor = result.NextCursor
		// Older versions return the number of the next page as a string.
		case result.NextPage != "":
			if result.NextPage == pageToken {
				return all, nil
			}
			pageToken = result.NextPage
			page++
		default:
			return all, nil
		}
	}
}

// parseSearchResult decodes a /api/search/metadata response. Some Immich versions
// wrap the results in an {"assets":{"items":[...]}} envelope, others return a flat
// asset array.
func parseSearchResult(res []byte) (searchResult, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(res, &envelope); err == nil {
		if assetsRaw, ok := envelope["assets"]; ok {
			var assets struct {
				Items      []Asset `json:"items"`
				NextCursor string  `json:"nextCursor"`
				NextPage   string  `json:"nextPage"`
			}
			if err := json.Unmarshal(assetsRaw, &assets); err == nil {
				return searchResult{
					Items:      assets.Items,
					NextCursor: assets.NextCursor,
					NextPage:   assets.NextPage,
				}, nil
			}
		}
	}

	var flat []Asset
	if err := json.Unmarshal(res, &flat); err == nil {
		return searchResult{Items: flat}, nil
	}

	return searchResult{}, fmt.Errorf("could not parse search response: %s", string(res))
}

// GetAssetsByPeople fetches all assets matching any of the specified person IDs.
// Immich applies personIds conjunctively (an asset must contain every listed
// person), so passing them all in one request returns only the intersection. To
// get the union instead, each person is searched separately and the results are
// merged, dropping any asset seen more than once.
func (c *Client) GetAssetsByPeople(personIds []string) ([]Asset, error) {
	seen := make(map[string]struct{})
	var all []Asset

	for _, personID := range personIds {
		assets, err := c.searchMetadata(SearchMetadataRequest{
			PersonIds: []string{personID},
		})
		if err != nil {
			return nil, err
		}

		for _, asset := range assets {
			if _, ok := seen[asset.ID]; ok {
				continue
			}
			seen[asset.ID] = struct{}{}
			all = append(all, asset)
		}
	}

	return all, nil
}

// SearchPeopleByName uses GET /api/search/person?name=... to find people by name
func (c *Client) SearchPeopleByName(name string) ([]Person, error) {
	query := url.Values{}
	query.Set("name", name)
	endpoint := "/api/search/person?" + query.Encode()
	res, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var people []Person
	if err := json.Unmarshal(res, &people); err != nil {
		return nil, fmt.Errorf("could not parse person search response: %w", err)
	}
	return people, nil
}

// GetPersonByID fetches a person by their ID using GET /api/people/:id
func (c *Client) GetPersonByID(id string) (*Person, error) {
	res, err := c.doRequest("GET", "/api/people/"+id, nil)
	if err != nil {
		return nil, err
	}

	var person Person
	if err := json.Unmarshal(res, &person); err != nil {
		return nil, err
	}
	return &person, nil
}

// SearchAssetsByDateRange fetches assets taken within a specific date range.
// Immich filters takenAfter/takenBefore against the capture time (fileCreatedAt),
// whereas createdAfter/createdBefore would filter on when the asset was uploaded.
func (c *Client) SearchAssetsByDateRange(after, before time.Time) ([]Asset, error) {
	req := SearchMetadataRequest{
		TakenAfter:  after.Format(time.RFC3339),
		TakenBefore: before.Format(time.RFC3339),
	}
	return c.searchMetadata(req)
}
