package selector

import (
	"math"
	"time"

	"immich-album-generator/pkg/immich"
)

// CalculateDaysDistance calculates absolute days between today and target date
func CalculateDaysDistance(today, target time.Time) float64 {
	// Normalize to start of day
	y1, m1, d1 := today.Date()
	y2, m2, d2 := target.Date()

	t1 := time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC)

	diff := t1.Sub(t2).Hours() / 24.0
	return math.Abs(diff)
}

// CalculateMemoriesDistance calculates distance disregarding the year
func CalculateMemoriesDistance(today, target time.Time) float64 {
	// target in current year
	yToday, mToday, dToday := today.Date()
	_, mTarget, dTarget := target.Date()

	tToday := time.Date(yToday, mToday, dToday, 0, 0, 0, 0, time.UTC)
	// Try creating target in today's year
	tTargetThisYear := time.Date(yToday, mTarget, dTarget, 0, 0, 0, 0, time.UTC)

	// Since we might be around the New Year boundary (e.g. today is Jan 1, target is Dec 31)
	// The difference shouldn't be 364 days, it should be 1 day.
	tTargetLastYear := time.Date(yToday-1, mTarget, dTarget, 0, 0, 0, 0, time.UTC)
	tTargetNextYear := time.Date(yToday+1, mTarget, dTarget, 0, 0, 0, 0, time.UTC)

	diffThis := math.Abs(tToday.Sub(tTargetThisYear).Hours() / 24.0)
	diffLast := math.Abs(tToday.Sub(tTargetLastYear).Hours() / 24.0)
	diffNext := math.Abs(tToday.Sub(tTargetNextYear).Hours() / 24.0)

	minDiff := math.Min(diffThis, math.Min(diffLast, diffNext))
	return minDiff
}

// ProcessMemories drops assets from the current year, since a memory is by definition
// from a past year, and returns the weights mapped 1:1 with the kept assets.
// The +/- maxDays window is already applied by the search query in SyncMemories, so the
// only date logic here is the year check, which the query cannot express: when today is
// close to the New Year the window for the previous year spills into the current year.
func ProcessMemories(assets []immich.Asset, halfLife float64) ([]immich.Asset, []float64) {
	today := time.Now()
	var filtered []immich.Asset
	var weights []float64

	for _, a := range assets {
		// memory usually means past years
		if a.FileCreatedAt.Year() >= today.Year() {
			continue
		}

		dist := CalculateMemoriesDistance(today, a.FileCreatedAt)
		filtered = append(filtered, a)
		weights = append(weights, CalculateDecayWeight(dist, halfLife))
	}

	return filtered, weights
}

// ProcessRecent returns the assets and their decay weights. SyncRecent already queries
// only the last maxDays, so there is nothing left to filter here.
func ProcessRecent(assets []immich.Asset, halfLife float64) ([]immich.Asset, []float64) {
	today := time.Now()
	weights := make([]float64, 0, len(assets))

	for _, a := range assets {
		dist := CalculateDaysDistance(today, a.FileCreatedAt)
		weights = append(weights, CalculateDecayWeight(dist, halfLife))
	}

	return assets, weights
}

// ProcessPeople just applies the decay function based on the asset age
func ProcessPeople(assets []immich.Asset, halfLife float64) ([]immich.Asset, []float64) {
	today := time.Now()
	var weights []float64

	for _, a := range assets {
		dist := CalculateDaysDistance(today, a.FileCreatedAt)
		weight := CalculateDecayWeight(dist, halfLife)
		weights = append(weights, weight)
	}

	return assets, weights
}
