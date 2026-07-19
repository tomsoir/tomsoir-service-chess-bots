package roster_test

import (
	"testing"

	"tomsoir-service-chess-bots/internal/roster"
)

func TestDefaultRosterStableIDs(t *testing.T) {
	a := roster.DefaultRoster()
	b := roster.DefaultRoster()
	if len(a) != 40 {
		t.Fatalf("expected 40 bots, got %d", len(a))
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Fatalf("unstable id at %d", i)
		}
		if a[i].EngineLevel < 1 || a[i].EngineLevel > 5 {
			t.Fatalf("bad level %d", a[i].EngineLevel)
		}
	}
}

func TestWithinBand(t *testing.T) {
	if !roster.WithinBand(1200, 1400) {
		t.Fatal("expected in band")
	}
	if roster.WithinBand(1200, 1401) {
		t.Fatal("expected out of band")
	}
}

func TestScoreNear(t *testing.T) {
	for i := 0; i < 20; i++ {
		s := roster.ScoreNear(900, 3)
		if !roster.WithinBand(s, 900) {
			t.Fatalf("score %d not near 900", s)
		}
	}
}
