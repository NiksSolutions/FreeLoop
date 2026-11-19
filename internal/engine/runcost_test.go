package engine

import (
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// WFL-015, PRV-008: a run's calls are priced as the Usage page prices them, each at
// the price that held on its day, so a price change does not reprice what was spent
// before it.
func TestRunCostPricesEachDay(t *testing.T) {
	prices := []config.Price{
		{Model: "m", From: "2026-12-01", Input: 4},
		{Model: "m", Input: 2},
	}
	million := event.UsageData{InputTokens: 1_000_000}
	part := priceCalls(prices, []usedOn{
		{model: "m", day: "2026-11-30", d: million},
		{model: "m", day: "2026-12-01", d: million},
		{model: "m", day: "2026-12-02", d: million},
	})
	if part.Cost != 10 || part.Unpriced || len(part.Models) != 1 || part.Models[0].Cost != 10 || !part.Models[0].Priced {
		t.Errorf("part = %+v", part)
	}
	part = priceCalls(prices, []usedOn{
		{model: "m", day: "2026-11-30", d: million},
		{model: "other", day: "2026-11-30", d: million},
	})
	if part.Cost != 2 || !part.Unpriced || len(part.Models) != 2 || part.Models[1].Priced {
		t.Errorf("with an unpriced model: %+v", part)
	}
}
