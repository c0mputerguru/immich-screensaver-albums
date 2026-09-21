package selector

import (
	"math"
	"testing"
	"time"

	"immich-album-generator/pkg/immich"
)

func TestCalculateDaysDistance(t *testing.T) {
	today := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	target := time.Date(2026, 9, 18, 5, 0, 0, 0, time.UTC)

	dist := CalculateDaysDistance(today, target)
	if math.Abs(dist-2.0) > 1e-9 {
		t.Errorf("Expected distance 2, got %f", dist)
	}
}

func TestCalculateMemoriesDistance(t *testing.T) {
	today := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	// Past year, same day
	target1 := time.Date(2025, 9, 20, 12, 0, 0, 0, time.UTC)
	dist1 := CalculateMemoriesDistance(today, target1)
	if math.Abs(dist1-0.0) > 1e-9 {
		t.Errorf("Expected distance 0, got %f", dist1)
	}

	// Past year, 3 days apart
	target2 := time.Date(2022, 9, 23, 12, 0, 0, 0, time.UTC)
	dist2 := CalculateMemoriesDistance(today, target2)
	if math.Abs(dist2-3.0) > 1e-9 {
		t.Errorf("Expected distance 3, got %f", dist2)
	}

	// Wrap around new year boundary tests
	todayNY := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	target3 := time.Date(2020, 12, 31, 12, 0, 0, 0, time.UTC)
	dist3 := CalculateMemoriesDistance(todayNY, target3)
	if math.Abs(dist3-1.0) > 1e-9 {
		t.Errorf("Expected distance 1, got %f", dist3)
	}
}

func TestProcessRecentWeightsEveryAsset(t *testing.T) {
	now := time.Now()
	assets := []immich.Asset{
		{ID: "today", FileCreatedAt: now},
		{ID: "yesterday", FileCreatedAt: now.AddDate(0, 0, -1)},
		{ID: "old", FileCreatedAt: now.AddDate(0, 0, -29)},
	}

	kept, weights := ProcessRecent(assets, 7)

	if len(kept) != len(assets) {
		t.Fatalf("Expected all %d assets to be kept, got %d", len(assets), len(kept))
	}
	if len(weights) != len(kept) {
		t.Fatalf("Expected %d weights to match %d assets, got %d", len(kept), len(kept), len(weights))
	}

	want := []float64{
		CalculateDecayWeight(0, 7),
		CalculateDecayWeight(1, 7),
		CalculateDecayWeight(29, 7),
	}
	for i, w := range want {
		if math.Abs(weights[i]-w) > 1e-9 {
			t.Errorf("Expected weight %f for %s, got %f", w, kept[i].ID, weights[i])
		}
	}
}

func TestProcessMemoriesDropsCurrentYear(t *testing.T) {
	now := time.Now()
	lastYear := time.Date(now.Year()-1, now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)

	assets := []immich.Asset{
		{ID: "current", FileCreatedAt: now},
		{ID: "future", FileCreatedAt: now.AddDate(1, 0, 0)},
		{ID: "memory", FileCreatedAt: lastYear},
	}

	kept, weights := ProcessMemories(assets, 7)

	if len(kept) != 1 || kept[0].ID != "memory" {
		t.Fatalf("Expected only the past-year asset to be kept, got %d assets", len(kept))
	}
	if len(weights) != len(kept) {
		t.Fatalf("Expected %d weights to match %d assets, got %d", len(kept), len(kept), len(weights))
	}
	if want := CalculateDecayWeight(CalculateMemoriesDistance(now, lastYear), 7); math.Abs(weights[0]-want) > 1e-9 {
		t.Errorf("Expected weight %f, got %f", want, weights[0])
	}
}
