package play

import (
	"testing"
	"time"
)

func midRand(n int) int {
	if n <= 1 {
		return 0
	}
	return n / 2
}

func TestThinkDelayOpeningFasterThanMiddlegame(t *testing.T) {
	open := thinkDelayRNG(thinkInput{
		Level:            3,
		MoveCount:        0,
		Minutes:          5,
		RemainingClockMS: 300_000,
		SearchBudgetMS:   300,
		SearchElapsed:    40 * time.Millisecond,
	}, midRand)
	mid := thinkDelayRNG(thinkInput{
		Level:            3,
		MoveCount:        24,
		OpeningComplete:  true,
		Minutes:          5,
		RemainingClockMS: 180_000,
		SearchBudgetMS:   300,
		SearchElapsed:    280 * time.Millisecond,
	}, midRand)
	if open >= mid {
		t.Fatalf("opening %v should be faster than middlegame %v", open, mid)
	}
}

func TestThinkDelayScalesWithTimeControl(t *testing.T) {
	bullet := thinkDelayRNG(thinkInput{
		Level:            4,
		MoveCount:        20,
		OpeningComplete:  true,
		Minutes:          1,
		RemainingClockMS: 40_000,
		SearchBudgetMS:   200,
		SearchElapsed:    180 * time.Millisecond,
	}, midRand)
	rapid := thinkDelayRNG(thinkInput{
		Level:            4,
		MoveCount:        20,
		OpeningComplete:  true,
		Minutes:          15,
		RemainingClockMS: 600_000,
		SearchBudgetMS:   2000,
		SearchElapsed:    1800 * time.Millisecond,
	}, midRand)
	if bullet >= rapid {
		t.Fatalf("bullet %v should be faster than rapid %v", bullet, rapid)
	}
}

func TestThinkDelayInstantSearchFasterThanFullBudget(t *testing.T) {
	easy := thinkDelayRNG(thinkInput{
		Level:            4,
		MoveCount:        16,
		OpeningComplete:  true,
		Minutes:          5,
		RemainingClockMS: 200_000,
		SearchBudgetMS:   1000,
		SearchElapsed:    40 * time.Millisecond,
	}, midRand)
	hard := thinkDelayRNG(thinkInput{
		Level:            4,
		MoveCount:        16,
		OpeningComplete:  true,
		Minutes:          5,
		RemainingClockMS: 200_000,
		SearchBudgetMS:   1000,
		SearchElapsed:    950 * time.Millisecond,
	}, midRand)
	if easy >= hard {
		t.Fatalf("instant engine %v should be faster than full-budget %v", easy, hard)
	}
}

func TestThinkDelayStaysUnderOpeningWindow(t *testing.T) {
	d := thinkDelayRNG(thinkInput{
		Level:            6,
		MoveCount:        0,
		Minutes:          15,
		RemainingClockMS: 900_000,
		SearchBudgetMS:   2000,
		SearchElapsed:    1800 * time.Millisecond,
	}, func(n int) int {
		if n <= 1 {
			return 0
		}
		return n - 1
	})
	if d > 8*time.Second {
		t.Fatalf("opening delay %v exceeds first-move cap", d)
	}
}

func TestThinkDelayLowClockIsSnappy(t *testing.T) {
	d := thinkDelayRNG(thinkInput{
		Level:            5,
		MoveCount:        40,
		OpeningComplete:  true,
		Minutes:          3,
		RemainingClockMS: 1800,
		SearchBudgetMS:   150,
		SearchElapsed:    140 * time.Millisecond,
	}, midRand)
	if d > 250*time.Millisecond {
		t.Fatalf("flag-pressure delay %v too slow", d)
	}
}
