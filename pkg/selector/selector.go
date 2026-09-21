package selector

import (
	"math"
	"math/rand"
	"sort"

	"immich-album-generator/pkg/immich"
)

// AResItem is used for internal A-Res algorithm
type AResItem struct {
	Asset immich.Asset
	Key   float64
}

// SelectWeightedRandom uses A-Res algorithm (Weighted Random Sampling without Replacement)
func SelectWeightedRandom(assets []immich.Asset, weights []float64, limit int) []immich.Asset {
	if limit >= len(assets) {
		return assets
	}

	var items []AResItem
	for i, asset := range assets {
		weight := weights[i]
		if weight <= 0 {
			weight = 1e-10 // avoid log(0) or log(negative)
		}
		// A-Res Key = u ^ (1/w) where u is rand(0, 1]
		u := rand.Float64()
		if u == 0 {
			u = 1e-10
		}
		key := math.Pow(u, 1.0/weight)
		items = append(items, AResItem{
			Asset: asset,
			Key:   key,
		})
	}

	// Sort items by key descending
	sort.Slice(items, func(i, j int) bool {
		return items[i].Key > items[j].Key
	})

	var result []immich.Asset
	for i := 0; i < limit; i++ {
		result = append(result, items[i].Asset)
	}

	return result
}

// CalculateDecayWeight calculates weight = 0.5 ^ (distance / halfLife)
func CalculateDecayWeight(distance float64, halfLife float64) float64 {
	if halfLife <= 0 {
		return 1.0 // no decay
	}
	return math.Pow(0.5, distance/halfLife)
}
