package engine

import "testing"

func TestDecidePrice_FollowsCompetitorUp(t *testing.T) {
	// 用户报告的场景：我方 R90，第二名竞品涨到 R95，应跟涨到 R94（保持比竞品低一个步长）。
	got := decidePrice(90, 95, 90, 0, 0, 1, 1, false)
	if got.NewPrice != 94 {
		t.Fatalf("NewPrice = %d, want 94", got.NewPrice)
	}
	if got.Action != "跟涨" {
		t.Fatalf("Action = %q, want 跟涨", got.Action)
	}
}

func TestDecidePrice_FollowsCompetitorDown(t *testing.T) {
	got := decidePrice(90, 85, 85, 0, 0, 1, 1, false)
	if got.NewPrice != 84 {
		t.Fatalf("NewPrice = %d, want 84", got.NewPrice)
	}
	if got.Action != "跟降" {
		t.Fatalf("Action = %q, want 跟降", got.Action)
	}
}

func TestDecidePrice_FloorProtection(t *testing.T) {
	got := decidePrice(90, 85, 85, 86, 0, 1, 1, false)
	if got.NewPrice != 86 {
		t.Fatalf("NewPrice = %d, want 86", got.NewPrice)
	}
	if got.Action != "底价保护" {
		t.Fatalf("Action = %q, want 底价保护", got.Action)
	}
}

func TestDecidePrice_FloorAlreadyReached(t *testing.T) {
	got := decidePrice(86, 85, 85, 86, 0, 1, 1, false)
	if got.NewPrice != -1 {
		t.Fatalf("NewPrice = %d, want -1 (hold)", got.NewPrice)
	}
}

func TestDecidePrice_GapSmallerThanStep(t *testing.T) {
	got := decidePrice(90, 91, 90, 0, 0, 1, 1, false)
	if got.NewPrice != -1 {
		t.Fatalf("NewPrice = %d, want -1 (hold)", got.NewPrice)
	}
}

func TestDecidePrice_MaxPriceCapsIncrease(t *testing.T) {
	got := decidePrice(90, 100, 90, 0, 94, 1, 1, false)
	if got.NewPrice != 94 {
		t.Fatalf("NewPrice = %d, want 94 (capped by max)", got.NewPrice)
	}
	if got.Action != "跟涨" {
		t.Fatalf("Action = %q, want 跟涨", got.Action)
	}
}

func TestDecidePrice_AlreadyAtMaxPrice(t *testing.T) {
	got := decidePrice(94, 100, 94, 0, 94, 1, 1, false)
	if got.NewPrice != -1 {
		t.Fatalf("NewPrice = %d, want -1 (hold at max)", got.NewPrice)
	}
}

func TestDecidePrice_TieWithCompetitorHolds(t *testing.T) {
	got := decidePrice(90, 90, 90, 0, 0, 1, 1, false)
	if got.NewPrice != -1 {
		t.Fatalf("NewPrice = %d, want -1 (hold)", got.NewPrice)
	}
}

func TestDecidePrice_SoloRaisesUpToMax(t *testing.T) {
	got := decidePrice(90, 0, 90, 0, 95, 1, 1, true)
	if got.NewPrice != 91 {
		t.Fatalf("NewPrice = %d, want 91", got.NewPrice)
	}
	if got.Action != "跟涨" {
		t.Fatalf("Action = %q, want 跟涨", got.Action)
	}
}

func TestDecidePrice_SoloAtMaxHolds(t *testing.T) {
	got := decidePrice(95, 0, 95, 0, 95, 1, 1, true)
	if got.NewPrice != -1 {
		t.Fatalf("NewPrice = %d, want -1 (hold)", got.NewPrice)
	}
}

func TestDecidePrice_SoloWithoutMaxPriceHolds(t *testing.T) {
	got := decidePrice(90, 0, 90, 0, 0, 1, 1, true)
	if got.NewPrice != -1 {
		t.Fatalf("NewPrice = %d, want -1 (no max price => no blind raise)", got.NewPrice)
	}
}

func TestDecidePrice_UnknownCompetitorNeverRaises(t *testing.T) {
	// 存在竞品但拿不到竞品价时，bestPrice == curPrice，绝不能上调。
	got := decidePrice(90, 0, 90, 0, 200, 1, 1, false)
	if got.NewPrice != -1 {
		t.Fatalf("NewPrice = %d, want -1 (must not raise on unknown competitor)", got.NewPrice)
	}
}

func TestDecidePrice_UnknownCompetitorStillLowers(t *testing.T) {
	got := decidePrice(90, 0, 85, 0, 200, 1, 1, false)
	if got.NewPrice != 84 {
		t.Fatalf("NewPrice = %d, want 84", got.NewPrice)
	}
	if got.Action != "跟降" {
		t.Fatalf("Action = %q, want 跟降", got.Action)
	}
}

func TestDecidePrice_DefaultStepsWhenNonPositive(t *testing.T) {
	got := decidePrice(90, 95, 90, 0, 0, 0, 0, false)
	if got.NewPrice != 94 {
		t.Fatalf("NewPrice = %d, want 94 with defaulted steps", got.NewPrice)
	}
}

func TestDecidePrice_RecoversPriceBelowFloor(t *testing.T) {
	for _, tc := range []struct {
		name       string
		competitor int
		best       int
		max        int
		solo       bool
	}{
		{"reported price 120 competitor 119 floor 210", 119, 119, 0, false},
		{"competitor higher but still below floor", 150, 120, 0, false},
		{"tie below floor", 120, 120, 0, false},
		{"unknown competitor", 0, 0, 0, false},
		{"solo raises directly to floor", 0, 120, 300, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := decidePrice(120, tc.competitor, tc.best, 210, tc.max, 1, 1, tc.solo)
			if got.NewPrice != 210 || got.Action != "底价保护" {
				t.Fatalf("decision = %+v, want floor recovery to 210", got)
			}
		})
	}
}

func TestDecidePrice_InvalidLimitsOrCurrentPriceHold(t *testing.T) {
	for _, tc := range []struct{ current, min, max int }{
		{120, 210, 200},
		{300, 210, 200},
		{0, 210, 0},
		{-1, 210, 0},
	} {
		if got := decidePrice(tc.current, 119, 119, tc.min, tc.max, 1, 1, false); got.NewPrice != -1 {
			t.Errorf("limits %+v: decision = %+v, want hold", tc, got)
		}
	}
}
