package play

import (
	"math/rand/v2"
	"time"
)

type thinkInput struct {
	Level            int
	MoveCount        int
	OpeningComplete  bool
	Minutes          int
	IncrementSec     int
	RemainingClockMS int
	SearchElapsed    time.Duration
	SearchBudgetMS   int
}

// thinkDelay is how long a human would take from seeing the opponent's move.
// Search time already spent is part of that budget — callers should sleep only
// the remainder.
func thinkDelay(in thinkInput) time.Duration {
	return thinkDelayRNG(in, rand.IntN)
}

func thinkDelayRNG(in thinkInput, intn func(int) int) time.Duration {
	if intn == nil {
		intn = rand.IntN
	}

	remaining := in.RemainingClockMS
	if remaining <= 0 {
		remaining = in.Minutes * 60 * 1000
		if remaining <= 0 {
			remaining = 60_000
		}
	}

	movesLeft := 30 - in.MoveCount/3
	if movesLeft < 10 {
		movesLeft = 10
	}
	slice := remaining / movesLeft
	if capSlice := clockSliceCap(in.Minutes); slice > capSlice {
		slice = capSlice
	}
	base := slice + in.IncrementSec*400

	switch {
	case in.Minutes <= 1:
		base = base * 55 / 100
	case in.Minutes <= 2:
		base = base * 70 / 100
	case in.Minutes <= 3:
		base = base * 85 / 100
	case in.Minutes >= 15:
		base = base * 120 / 100
	}

	switch {
	case in.Level <= 1:
		base = base * 70 / 100
	case in.Level == 2:
		base = base * 80 / 100
	case in.Level == 3:
		base = base * 90 / 100
	}

	ply := in.MoveCount
	switch {
	case !in.OpeningComplete || ply < 2:
		base = base * 22 / 100
		if base > 900 {
			base = 900
		}
	case ply < 6:
		base = base * 38 / 100
		if base > 1600 {
			base = 1600
		}
	case ply < 10:
		base = base * 55 / 100
	}

	base = applySearchHardness(base, in)

	span := base/2 + 150
	if span < 1 {
		span = 1
	}
	base = base*3/4 + intn(span)

	// Occasional long think in the middlegame, rare snap replies.
	if ply >= 8 && intn(100) < 10 {
		base = base * (180 + intn(80)) / 100
	} else if ply >= 6 && intn(100) < 8 {
		base = base * (30 + intn(25)) / 100
	}

	minMS, maxMS := thinkBounds(in)
	if base < minMS {
		base = minMS
	}
	if base > maxMS {
		base = maxMS
	}
	return time.Duration(base) * time.Millisecond
}

func applySearchHardness(base int, in thinkInput) int {
	if in.SearchBudgetMS > 0 {
		ms := in.SearchElapsed.Milliseconds()
		if ms < 0 {
			ms = 0
		}
		ratio := float64(ms) / float64(in.SearchBudgetMS)
		switch {
		case ratio < 0.2:
			return base * 40 / 100
		case ratio < 0.5:
			return base * 70 / 100
		case ratio > 0.85:
			return base * 135 / 100
		}
		return base
	}
	if in.SearchElapsed > 0 && in.SearchElapsed < 50*time.Millisecond {
		return base * 45 / 100
	}
	return base
}

func clockSliceCap(minutes int) int {
	switch {
	case minutes <= 1:
		return 1800
	case minutes <= 3:
		return 3500
	case minutes <= 5:
		return 5000
	case minutes >= 15:
		return 10000
	default:
		return 7000
	}
}

func thinkBounds(in thinkInput) (minMS, maxMS int) {
	minMS = 160
	maxMS = 12_000
	switch {
	case in.Minutes <= 1:
		minMS, maxMS = 120, 2500
	case in.Minutes <= 3:
		minMS, maxMS = 150, 6000
	case in.Minutes <= 5:
		minMS, maxMS = 160, 9000
	case in.Minutes >= 15:
		minMS, maxMS = 200, 20_000
	}

	ply := in.MoveCount
	if !in.OpeningComplete || ply < 2 {
		if maxMS > 8000 {
			maxMS = 8000
		}
		if minMS > 220 {
			minMS = 180
		}
	}

	remaining := in.RemainingClockMS
	if remaining > 0 {
		headroom := remaining / 3
		if remaining < 8000 && headroom < maxMS {
			maxMS = headroom
		}
		if remaining < 2500 {
			minMS = 40
			if maxMS > 200 {
				maxMS = 200
			}
		}
	}
	if minMS < 40 {
		minMS = 40
	}
	if maxMS < minMS {
		maxMS = minMS
	}
	return minMS, maxMS
}
