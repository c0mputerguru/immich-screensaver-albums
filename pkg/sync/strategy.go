package sync

import (
	"fmt"
	"log"
	"time"

	"immich-album-generator/pkg/immich"
	"immich-album-generator/pkg/selector"
)

func SyncRecent(client *immich.Client, albumName string, maxDays int, limit int, halfLife float64, dryRun bool) error {
	log.Printf("Processing 'Recent' album: %s", albumName)

	today := time.Now()
	// Fetch ONLY assets from the last N days directly via the API.
	start := today.AddDate(0, 0, -maxDays)
	log.Printf("Recent: Querying server for assets between %s and %s", start.Format("2006-01-02"), today.Format("2006-01-02"))

	queriedAssets, err := client.SearchAssetsByDateRange(start, today)
	if err != nil {
		return fmt.Errorf("failed to search recent assets: %w", err)
	}
	log.Printf("Recent: Fetched %d items from API search", len(queriedAssets))

	candidates, weights := selector.ProcessRecent(queriedAssets, halfLife)
	log.Printf("Recent: computed weights for %d items (server query already limited to %d days)", len(candidates), maxDays)

	selected := selector.SelectWeightedRandom(candidates, weights, limit)
	log.Printf("Recent: weighted random selection chose %d items (limit: %d)", len(selected), limit)

	album, err := EnsureAlbum(client, albumName, dryRun)
	if err != nil {
		return err
	}

	return SyncAlbumAssets(client, album.ID, selected, dryRun)
}

func SyncMemories(client *immich.Client, albumName string, maxDays int, limit int, halfLife float64, dryRun bool) error {
	log.Printf("Processing 'Memories' album: %s", albumName)

	today := time.Now()
	var allQueriedAssets []immich.Asset

	// Memories span previous years. Let's query up to 50 years back.
	log.Printf("Memories: Querying server for assets within %d days of today's date across the past 50 years", maxDays)
	for i := 1; i <= 50; i++ {
		targetYear := today.Year() - i
		targetDate := time.Date(targetYear, today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

		start := targetDate.AddDate(0, 0, -maxDays)
		end := targetDate.AddDate(0, 0, maxDays+1) // +1 to cover the end date fully

		yearAssets, err := client.SearchAssetsByDateRange(start, end)
		if err == nil && len(yearAssets) > 0 {
			allQueriedAssets = append(allQueriedAssets, yearAssets...)
		}
	}
	log.Printf("Memories: Fetched %d relevant items from API search across past years", len(allQueriedAssets))

	filtered, weights := selector.ProcessMemories(allQueriedAssets, halfLife)
	log.Printf("Memories: kept %d items from past years (out of %d queried)", len(filtered), len(allQueriedAssets))

	selected := selector.SelectWeightedRandom(filtered, weights, limit)
	log.Printf("Memories: weighted random selection chose %d items (limit: %d)", len(selected), limit)

	album, err := EnsureAlbum(client, albumName, dryRun)
	if err != nil {
		return err
	}

	return SyncAlbumAssets(client, album.ID, selected, dryRun)
}

func SyncPeople(client *immich.Client, peopleList []string, albumName string, limit int, halfLife float64, dryRun bool) error {
	log.Printf("Processing 'People' album: %s", albumName)

	log.Println("Resolving people names/ids...")

	var personIds []string
	personLookup := make(map[string]bool)

	for _, input := range peopleList {
		// First try as a person ID via GET /api/people/:id
		if p, err := client.GetPersonByID(input); err == nil && p != nil && p.ID != "" {
			if !personLookup[p.ID] {
				personIds = append(personIds, p.ID)
				personLookup[p.ID] = true
			}
			continue
		}

		// Otherwise search by name via POST /api/search/person
		peopleResult, err := client.SearchPeopleByName(input)
		if err == nil && len(peopleResult) > 0 {
			for _, p := range peopleResult {
				if p.Name == input { // Ensure exact match
					if !personLookup[p.ID] {
						personIds = append(personIds, p.ID)
						personLookup[p.ID] = true
					}
				}
			}
		} else {
			log.Printf("Warning: Could not find person for '%s'", input)
		}
	}

	if len(personIds) == 0 {
		log.Println("No matching people found, aborting people sync.")
		return nil
	}

	assets, err := client.GetAssetsByPeople(personIds)
	if err != nil {
		return err
	}
	log.Printf("People: matching people query returned %d items", len(assets))

	filtered, weights := selector.ProcessPeople(assets, halfLife)
	selected := selector.SelectWeightedRandom(filtered, weights, limit)
	log.Printf("People: weighted random selection chose %d items (limit: %d)", len(selected), limit)

	album, err := EnsureAlbum(client, albumName, dryRun)
	if err != nil {
		return err
	}

	return SyncAlbumAssets(client, album.ID, selected, dryRun)
}
