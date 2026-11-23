package modelcatalog

import (
	"math"
	"testing"

	"ruoyi-go/internal/model"
)

func TestPriceIntegerRounding(t *testing.T) {
	for _, tt := range []struct {
		name                  string
		r                     model.PODModelRate
		in, out, images, want int64
		bad                   bool
	}{
		{"round combined tokens once", model.PODModelRate{InputPerMillion: 1, OutputPerMillion: 1}, 1, 1, 0, 1, false},
		{"exact million", model.PODModelRate{InputPerMillion: 3}, 1_000_000, 0, 0, 3, false},
		{"round remainder", model.PODModelRate{OutputPerMillion: 3}, 0, 1_000_001, 0, 4, false},
		{"zero price", model.PODModelRate{}, math.MaxInt64, 0, 0, 0, false},
		{"images", model.PODModelRate{PerImage: 7}, 0, 0, 3, 21, false},
		{"large intermediate", model.PODModelRate{InputPerMillion: 1_000_000}, 1_000_000_000_000, 0, 0, 1_000_000_000_000, false},
		{"overflow", model.PODModelRate{PerImage: math.MaxInt64}, 0, 0, 2, 0, true},
		{"negative usage", model.PODModelRate{}, -1, 0, 0, 0, true},
		{"negative rate", model.PODModelRate{InputPerMillion: -1}, 0, 0, 0, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Price(tt.r, tt.in, tt.out, tt.images)
			if (err != nil) != tt.bad || !tt.bad && got != tt.want {
				t.Fatalf("price=%d error=%v", got, err)
			}
		})
	}
}

func TestUsagePriceDetectsOverboundsEvenWhenFree(t *testing.T) {
	b := model.PODBoundModel{Kind: "text", InputTokenBound: 1256, Capabilities: model.PODModelCapabilities{MaxOutputTokens: 100}}
	if _, _, exceeded, err := UsagePrice(b, model.PODProviderUsage{InputTokens: 1257}); err != nil || !exceeded {
		t.Fatal("free model escaped input limit")
	}
	b = model.PODBoundModel{Kind: "image", ImageCount: 2, Points: 20, CostMicros: 200, PointRule: model.PODModelRate{PerImage: 10}, CostRule: model.PODModelRate{PerImage: 100}}
	if c, p, exceeded, err := UsagePrice(b, model.PODProviderUsage{ImageCount: 1}); err != nil || exceeded || c != 100 || p != 10 {
		t.Fatal("partial result was not priced by confirmed usage")
	}
	if c, p, exceeded, err := UsagePrice(b, model.PODProviderUsage{ImageCount: 3}); err != nil || !exceeded || c != 300 || p != 30 {
		t.Fatal("overbound actual usage was lost")
	}
}
