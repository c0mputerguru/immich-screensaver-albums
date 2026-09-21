package sync

import (
	"fmt"
	"log"

	"immich-album-generator/pkg/immich"
)

// EnsureAlbum checks if an album with the given name exists, or creates it
func EnsureAlbum(client *immich.Client, albumName string, dryRun bool) (*immich.Album, error) {
	album, err := client.GetAlbumByName(albumName)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch album: %w", err)
	}

	if album != nil {
		return album, nil
	}

	if dryRun {
		log.Printf("[DRY RUN] Would create album '%s'", albumName)
		return &immich.Album{ID: "dry-run-album-id", AlbumName: albumName}, nil
	}

	// create album
	log.Printf("Album '%s' not found, creating...", albumName)
	return client.CreateAlbum(albumName)
}

// SyncAlbumAssets computes diff and syncs the album with target state
func SyncAlbumAssets(client *immich.Client, albumId string, targetAssets []immich.Asset, dryRun bool) error {
	var currentAssets []immich.Asset

	if albumId != "dry-run-album-id" {
		albumInfo, err := client.GetAlbumInfo(albumId)
		if err != nil {
			return fmt.Errorf("failed to fetch album info: %w", err)
		}
		currentAssets = albumInfo.Assets
	}

	currentMap := make(map[string]bool)
	for _, a := range currentAssets {
		currentMap[a.ID] = true
	}

	targetMap := make(map[string]bool)
	for _, a := range targetAssets {
		targetMap[a.ID] = true
	}

	var toAdd []string
	for _, a := range targetAssets {
		if !currentMap[a.ID] {
			toAdd = append(toAdd, a.ID)
		}
	}

	var toRemove []string
	for _, a := range currentAssets {
		if !targetMap[a.ID] {
			toRemove = append(toRemove, a.ID)
		}
	}

	log.Printf("Syncing Album ID: %s. Desired state count: %d. Scheduled Adds: %d, Scheduled Removes: %d.", albumId, len(targetAssets), len(toAdd), len(toRemove))

	if dryRun {
		log.Println("[DRY RUN] Would execute Adds:", len(toAdd), "items")
		log.Println("[DRY RUN] Would execute Removes:", len(toRemove), "items")
		return nil
	}

	if len(toAdd) > 0 {
		log.Printf("Executing Add of %d items", len(toAdd))
		if err := client.AddAssetsToAlbum(albumId, toAdd); err != nil {
			return fmt.Errorf("failed to add assets: %w", err)
		}
	}

	if len(toRemove) > 0 {
		log.Printf("Executing Remove of %d items", len(toRemove))
		if err := client.RemoveAssetsFromAlbum(albumId, toRemove); err != nil {
			return fmt.Errorf("failed to remove assets: %w", err)
		}
	}

	return nil
}
