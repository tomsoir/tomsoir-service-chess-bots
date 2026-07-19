package play

import (
	"context"
	"log"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"tomsoir-service-chess-bots/internal/chessapi"
	"tomsoir-service-chess-bots/internal/engineclient"
	"tomsoir-service-chess-bots/internal/roster"
	"tomsoir-service-chess-bots/internal/wsclient"
)

type Driver struct {
	chess  *chessapi.Client
	engine *engineclient.Client
	wsBase string
	onDone func(playerID string)

	mu    sync.Mutex
	games map[string]bool
}

func New(chess *chessapi.Client, engine *engineclient.Client, wsBase string) *Driver {
	return &Driver{
		chess:  chess,
		engine: engine,
		wsBase: wsBase,
		games:  map[string]bool{},
	}
}

func (d *Driver) SetOnDone(fn func(playerID string)) {
	d.onDone = fn
}

func (d *Driver) HandleGame(parent context.Context, bot roster.Identity, gameID string, seed *chessapi.Game) {
	d.mu.Lock()
	if d.games[gameID] {
		d.mu.Unlock()
		return
	}
	d.games[gameID] = true
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		delete(d.games, gameID)
		d.mu.Unlock()
		if d.onDone != nil {
			d.onDone(bot.ID)
		}
	}()

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	chatDisabled := false
	var chatMu sync.Mutex
	ws := wsclient.DialGame(ctx, d.wsBase, gameID, bot.ID, func(msg wsclient.Message) {
		if msg.Type == "chat_disabled" {
			chatMu.Lock()
			chatDisabled = true
			chatMu.Unlock()
		}
	})
	defer ws.Close()

	maybeEmoji := func(force bool) {
		chatMu.Lock()
		disabled := chatDisabled
		chatMu.Unlock()
		if disabled {
			return
		}
		if !force && rand.Float64() > bot.EmojiRate {
			return
		}
		emojis := []string{"🔥", "😎", "👀", "💪"}
		_ = ws.SendEmoji(emojis[rand.IntN(len(emojis))])
	}

	if rand.Float64() < bot.EmojiRate*0.5 {
		time.AfterFunc(2*time.Second+time.Duration(rand.IntN(3000))*time.Millisecond, func() {
			maybeEmoji(true)
		})
	}

	g := seed
	var err error
	if g == nil {
		g, err = d.chess.GetGame(ctx, gameID)
		if err != nil {
			log.Printf("play get game %s: %v", gameID, err)
			return
		}
	}

	color := botColor(g, bot.ID)
	if color == "" {
		log.Printf("play: bot %s not seated in game %s", bot.ID, gameID)
		return
	}

	ticker := time.NewTicker(800 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g, err = d.chess.GetGame(ctx, gameID)
			if err != nil {
				continue
			}
			if g.Status != "active" {
				return
			}
			if g.Turn != color {
				continue
			}
			if err := d.playMove(ctx, bot, g, color, maybeEmoji); err != nil {
				log.Printf("play move game %s bot %s: %v", gameID, bot.Name, err)
			}
		}
	}
}

func botColor(g *chessapi.Game, botID string) string {
	if g.White != nil && g.White.ID == botID {
		return "white"
	}
	if g.Black != nil && g.Black.ID == botID {
		return "black"
	}
	return ""
}

func (d *Driver) playMove(ctx context.Context, bot roster.Identity, g *chessapi.Game, color string, maybeEmoji func(bool)) error {
	delay := thinkDelay(bot.EngineLevel, len(g.Moves), g.OpeningComplete)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}

	// Re-check turn after delay.
	fresh, err := d.chess.GetGame(ctx, g.ID)
	if err != nil {
		return err
	}
	if fresh.Status != "active" || fresh.Turn != color {
		return nil
	}

	movetime := engineMovetime(fresh, color, bot.EngineLevel)
	uci, err := d.engine.GetBestMove(ctx, fresh.FEN, fresh.Variant, bot.EngineLevel, movetime)
	if err != nil {
		return err
	}
	if uci == "" {
		return nil
	}
	// Kid levels: occasional random legal-ish swap is handled by low skill; keep engine move.
	updated, err := d.chess.MakeMove(ctx, fresh.ID, bot.ID, uci)
	if err != nil {
		return err
	}
	if looksLikeCaptureOrCheck(updated) && rand.Float64() < bot.EmojiRate {
		maybeEmoji(true)
	}
	return nil
}

func thinkDelay(level, moveCount int, openingComplete bool) time.Duration {
	base := 800
	switch level {
	case 1:
		base = 400
	case 2:
		base = 700
	case 3:
		base = 1200
	case 4:
		base = 1800
	default:
		base = 2500
	}
	if !openingComplete || moveCount < 6 {
		base = base * 2 / 3
	}
	if moveCount > 20 {
		base = base + 400
	}
	jitter := rand.IntN(base/2 + 200)
	ms := base + jitter
	if ms < 250 {
		ms = 250
	}
	if ms > 8000 {
		ms = 8000
	}
	// Stay under opening timeout (~25s) with margin.
	if !openingComplete && ms > 12000 {
		ms = 12000
	}
	return time.Duration(ms) * time.Millisecond
}

func engineMovetime(g *chessapi.Game, color string, level int) int {
	clock := g.WhiteClock
	if color == "black" {
		clock = g.BlackClock
	}
	mt := clock / 10
	if mt < 150 {
		mt = 150
	}
	if mt > 2000 {
		mt = 2000
	}
	if level <= 2 && mt > 400 {
		mt = 400
	}
	return mt
}

func looksLikeCaptureOrCheck(g *chessapi.Game) bool {
	if g == nil || len(g.Moves) == 0 {
		return false
	}
	last := g.Moves[len(g.Moves)-1]
	return strings.Contains(last, "x") || strings.Contains(last, "+") || strings.Contains(last, "#")
}
