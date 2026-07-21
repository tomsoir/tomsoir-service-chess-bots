package roster_test

import (
	"testing"

	"tomsoir-service-chess-bots/internal/roster"
)

func TestNewEphemeralUsuallyGuest(t *testing.T) {
	guests := 0
	n := 200
	for i := 0; i < n; i++ {
		id := roster.NewEphemeral(1200)
		if id.ID == "" {
			t.Fatal("empty id")
		}
		if id.Name == "Guest" {
			guests++
		}
		if id.EngineLevel < 1 || id.EngineLevel > 6 {
			t.Fatalf("bad level %d", id.EngineLevel)
		}
		if !roster.WithinBand(id.Score, 1200) {
			t.Fatalf("score %d not near 1200", id.Score)
		}
	}
	// RareNameChance=0.12 → expect most Guests; allow wide variance.
	if guests < n/2 {
		t.Fatalf("expected mostly Guest, got %d/%d", guests, n)
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
