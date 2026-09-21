package selector

import (
	"immich-album-generator/pkg/immich"
	"math"
	"testing"
)

func TestCalculateDecayWeight(t *testing.T) {
	weight := CalculateDecayWeight(10, 10)
	if math.Abs(weight-0.5) > 1e-9 {
		t.Errorf("Expected 0.5, got %f", weight)
	}

	weightZero := CalculateDecayWeight(0, 10)
	if math.Abs(weightZero-1.0) > 1e-9 {
		t.Errorf("Expected 1.0, got %f", weightZero)
	}

	weightNoHalfLife := CalculateDecayWeight(10, 0)
	if math.Abs(weightNoHalfLife-1.0) > 1e-9 {
		t.Errorf("Expected 1.0, got %f", weightNoHalfLife)
	}
}

func TestSelectWeightedRandomLimits(t *testing.T) {
	assets := []immich.Asset{
		{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}, {ID: "5"},
	}
	weights := []float64{1.0, 1.0, 1.0, 1.0, 1.0}

	res := SelectWeightedRandom(assets, weights, 3)
	if len(res) != 3 {
		t.Errorf("Expected 3 items, got %d", len(res))
	}

	resOver := SelectWeightedRandom(assets, weights, 10)
	if len(resOver) != 5 {
		t.Errorf("Expected 5 items, got %d", len(resOver))
	}
}
