package fleet

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"sync"
	"time"

	"tomsoir-service-chess-bots/internal/botsreg"
	"tomsoir-service-chess-bots/internal/chessapi"
	"tomsoir-service-chess-bots/internal/config"
	"tomsoir-service-chess-bots/internal/play"
	"tomsoir-service-chess-bots/internal/roster"
	"tomsoir-service-chess-bots/internal/wsclient"
)

type activeBot struct {
	identity roster.Identity
	lobbyID  string
	minutes  int
	tc       string
	variant  string
}

type Manager struct {
	chess    *chessapi.Client
	reg      *botsreg.Registry
	play     *play.Driver
	presence *wsclient.PresenceHub
	rootCtx  context.Context
	loc      *time.Location
	minVis   int
	maxVis   int
	hbEvery  time.Duration
	tick     time.Duration

	mu     sync.Mutex
	active map[string]*activeBot // bot player id -> state
	inGame map[string]bool
}

func New(chess *chessapi.Client, reg *botsreg.Registry, driver *play.Driver, presence *wsclient.PresenceHub) *Manager {
	return &Manager{
		chess:    chess,
		reg:      reg,
		play:     driver,
		presence: presence,
		loc:      config.Timezone(),
		minVis:   config.MinVisible(),
		maxVis:   config.MaxVisible(),
		hbEvery:  config.HeartbeatEvery(),
		tick:     config.FleetTick(),
		active:   map[string]*activeBot{},
		inGame:   map[string]bool{},
	}
}

func (m *Manager) Start(ctx context.Context) {
	m.rootCtx = ctx
	ticker := time.NewTicker(m.tick)
	hb := time.NewTicker(m.hbEvery)
	defer ticker.Stop()
	defer hb.Stop()
	defer m.presence.CloseAll()
	for {
		select {
		case <-ctx.Done():
			m.leaveAll(context.Background())
			return
		case <-ticker.C:
			m.tickOnce(ctx)
		case <-hb.C:
			m.heartbeatAll(ctx)
		}
	}
}

func (m *Manager) targetVisible(now time.Time) int {
	hour := now.In(m.loc).Hour()
	base := 4
	switch {
	case hour >= 0 && hour < 6:
		base = 3
	case hour >= 6 && hour < 12:
		base = 5
	case hour >= 12 && hour < 17:
		base = 7
	case hour >= 17 && hour < 22:
		base = 10
	default:
		base = 6
	}
	jitter := rand.IntN(3) - 1
	n := base + jitter
	if n < m.minVis {
		n = m.minVis
	}
	if n > m.maxVis {
		n = m.maxVis
	}
	return n
}

var commonTCs = []struct {
	minutes int
	tc      string
}{
	{1, "1+0"},
	{3, "3+0"},
	{3, "3+2"},
	{5, "5+0"},
	{5, "5+3"},
	{10, "10+0"},
	{15, "15+10"},
}

func (m *Manager) tickOnce(ctx context.Context) {
	entries, err := m.chess.ListWaiting(ctx)
	if err != nil {
		log.Printf("fleet list waiting: %v", err)
		return
	}

	m.reconcileMatched(ctx, entries)

	m.serveLonelySeekers(ctx, entries)

	entries, err = m.chess.ListWaiting(ctx)
	if err != nil {
		log.Printf("fleet list waiting: %v", err)
		return
	}

	target := m.targetVisible(time.Now())
	botWaiting := m.countBotWaiting(entries)
	protected := m.protectedBotIDs(entries)

	if botWaiting < target {
		need := target - botWaiting
		for i := 0; i < need; i++ {
			if err := m.spawnRandom(ctx); err != nil {
				log.Printf("fleet spawn: %v", err)
				break
			}
		}
	} else if botWaiting > target {
		extra := botWaiting - target
		m.churnLeave(ctx, extra, protected)
	} else if rand.IntN(8) == 0 && botWaiting >= 3 {
		m.churnLeave(ctx, 1, protected)
	}

	m.pollStatuses(ctx)
}

func (m *Manager) serveLonelySeekers(ctx context.Context, entries []chessapi.LobbyEntry) {
	now := time.Now().UTC()
	grace := config.SeekerGrace()
	botIDs := m.knownBotIDs()

	for _, seeker := range m.realSeekers(entries, botIDs) {
		if m.hasCompatibleReal(entries, seeker, botIDs) {
			continue
		}
		waited := now.Sub(seeker.JoinedAt)
		if waited < grace {
			continue
		}
		if err := m.ensureBotMatch(ctx, seeker); err != nil {
			log.Printf("fleet ensure bot match for %s (%s): %v", seeker.PlayerName, seeker.PlayerID, err)
		}
	}
}

func (m *Manager) hasCompatibleReal(entries []chessapi.LobbyEntry, seeker chessapi.LobbyEntry, botIDs map[string]bool) bool {
	for _, e := range entries {
		if e.PlayerID == seeker.PlayerID || botIDs[e.PlayerID] {
			continue
		}
		if e.Minutes != seeker.Minutes {
			continue
		}
		if normalizeVariant(e.Variant) != normalizeVariant(seeker.Variant) {
			continue
		}
		return true
	}
	return false
}

func (m *Manager) findBestInBandBot(entries []chessapi.LobbyEntry, seeker chessapi.LobbyEntry, exclude map[string]bool) *chessapi.LobbyEntry {
	botIDs := m.knownBotIDs()
	var best *chessapi.LobbyEntry
	bestDiff := 1 << 30
	for i := range entries {
		e := &entries[i]
		if !botIDs[e.PlayerID] || exclude[e.PlayerID] {
			continue
		}
		if e.Minutes != seeker.Minutes {
			continue
		}
		if normalizeVariant(e.Variant) != normalizeVariant(seeker.Variant) {
			continue
		}
		if !roster.WithinBand(e.Score, seeker.Score) {
			continue
		}
		diff := e.Score - seeker.Score
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			bestDiff = diff
			best = e
		}
	}
	return best
}

func (m *Manager) ensureBotMatch(ctx context.Context, seeker chessapi.LobbyEntry) error {
	entries, err := m.chess.ListWaiting(ctx)
	if err != nil {
		return err
	}
	if !m.seekerStillWaiting(entries, seeker.PlayerID) {
		return nil
	}
	if m.hasCompatibleReal(entries, seeker, m.knownBotIDs()) {
		return nil
	}

	for _, e := range entries {
		if e.PlayerID == seeker.PlayerID {
			seeker = e
			break
		}
	}

	exclude := map[string]bool{}
	bot := m.findBestInBandBot(entries, seeker, exclude)
	if bot == nil {
		spawned, spawnErr := m.spawnNear(ctx, seeker)
		if spawnErr != nil {
			return fmt.Errorf("spawn near: %w", spawnErr)
		}
		entries, err = m.chess.ListWaiting(ctx)
		if err != nil {
			return err
		}
		if !m.seekerStillWaiting(entries, seeker.PlayerID) {
			return nil
		}
		for _, e := range entries {
			if e.PlayerID == seeker.PlayerID {
				seeker = e
				break
			}
		}
		if spawned != nil {
			bot = m.entryForPlayer(entries, spawned.ID)
		}
		if bot == nil {
			bot = m.findBestInBandBot(entries, seeker, exclude)
		}
	}
	if bot == nil {
		return fmt.Errorf("no in-band bot after spawn")
	}

	if err := m.challengeSeeker(ctx, bot, seeker); err != nil {
		failedID := bot.PlayerID
		exclude[failedID] = true
		m.leaveOne(ctx, failedID)

		spawned, spawnErr := m.spawnNear(ctx, seeker)
		if spawnErr != nil {
			return fmt.Errorf("match challenge: %w (retry spawn: %v)", err, spawnErr)
		}
		entries, listErr := m.chess.ListWaiting(ctx)
		if listErr != nil {
			return fmt.Errorf("match challenge: %w (relist: %v)", err, listErr)
		}
		if !m.seekerStillWaiting(entries, seeker.PlayerID) {
			return nil
		}
		for _, e := range entries {
			if e.PlayerID == seeker.PlayerID {
				seeker = e
				break
			}
		}
		bot = nil
		if spawned != nil {
			bot = m.entryForPlayer(entries, spawned.ID)
		}
		if bot == nil {
			bot = m.findBestInBandBot(entries, seeker, exclude)
		}
		if bot == nil {
			return fmt.Errorf("match challenge: %w (no bot after retry)", err)
		}
		if err := m.challengeSeeker(ctx, bot, seeker); err != nil {
			return fmt.Errorf("match challenge retry: %w", err)
		}
	}
	return nil
}

func (m *Manager) entryForPlayer(entries []chessapi.LobbyEntry, playerID string) *chessapi.LobbyEntry {
	for i := range entries {
		if entries[i].PlayerID == playerID {
			return &entries[i]
		}
	}
	return nil
}

func (m *Manager) challengeSeeker(ctx context.Context, bot *chessapi.LobbyEntry, seeker chessapi.LobbyEntry) error {
	botLobbyID := bot.ID
	m.mu.Lock()
	ab, ok := m.active[bot.PlayerID]
	if ok && ab.lobbyID != "" {
		botLobbyID = ab.lobbyID
	}
	identity := roster.Identity{
		ID:          bot.PlayerID,
		Name:        bot.PlayerName,
		CountryCode: bot.CountryCode,
		Score:       bot.Score,
		EngineLevel: roster.LevelForScore(bot.Score),
	}
	if ok {
		identity = ab.identity
		identity.Score = bot.Score
	} else {
		m.active[bot.PlayerID] = &activeBot{
			identity: identity,
			lobbyID:  botLobbyID,
			minutes:  bot.Minutes,
			tc:       bot.TimeControl,
			variant:  bot.Variant,
		}
		ab = m.active[bot.PlayerID]
	}
	m.mu.Unlock()

	res, err := m.chess.Match(ctx, bot.PlayerID, botLobbyID, seeker.ID)
	if err != nil {
		return err
	}
	if res != nil && res.Status == "matched" && res.Game != nil {
		m.mu.Lock()
		delete(m.active, bot.PlayerID)
		m.inGame[bot.PlayerID] = true
		identity = ab.identity
		m.mu.Unlock()
		go m.play.HandleGame(ctx, identity, res.GameID, res.Game)
		log.Printf("fleet matched bot %s (%d) → %s (%d) after grace",
			identity.Name, identity.Score, seeker.PlayerName, seeker.Score)
	}
	return nil
}

func (m *Manager) seekerStillWaiting(entries []chessapi.LobbyEntry, playerID string) bool {
	for _, e := range entries {
		if e.PlayerID == playerID {
			return true
		}
	}
	return false
}

func (m *Manager) protectedBotIDs(entries []chessapi.LobbyEntry) map[string]bool {
	out := map[string]bool{}
	botIDs := m.knownBotIDs()
	for _, seeker := range m.realSeekers(entries, botIDs) {
		if m.hasCompatibleReal(entries, seeker, botIDs) {
			continue
		}
		if bot := m.findBestInBandBot(entries, seeker, nil); bot != nil {
			out[bot.PlayerID] = true
		}
	}
	return out
}

func (m *Manager) reconcileMatched(ctx context.Context, entries []chessapi.LobbyEntry) {
	present := map[string]bool{}
	for _, e := range entries {
		present[e.PlayerID] = true
	}

	type matched struct {
		identity roster.Identity
		gameID   string
		game     *chessapi.Game
	}
	var toPlay []matched
	var toRetire []string

	m.mu.Lock()
	for id, ab := range m.active {
		if present[id] {
			continue
		}
		status, gameID, game, err := m.chess.LobbyStatus(ctx, ab.lobbyID)
		if err == nil && status == "matched" && game != nil {
			m.inGame[id] = true
			delete(m.active, id)
			toPlay = append(toPlay, matched{identity: ab.identity, gameID: gameID, game: game})
			continue
		}
		delete(m.active, id)
		toRetire = append(toRetire, id)
	}
	m.mu.Unlock()

	for _, id := range toRetire {
		m.retireBot(ctx, id)
	}
	for _, item := range toPlay {
		m.presence.Online(m.rootCtx, item.identity.ID)
		go m.play.HandleGame(ctx, item.identity, item.gameID, item.game)
	}
}

func (m *Manager) pollStatuses(ctx context.Context) {
	m.mu.Lock()
	snap := make([]*activeBot, 0, len(m.active))
	for _, ab := range m.active {
		snap = append(snap, ab)
	}
	m.mu.Unlock()
	for _, ab := range snap {
		status, gameID, game, err := m.chess.LobbyStatus(ctx, ab.lobbyID)
		if err != nil {
			continue
		}
		if status == "matched" && game != nil {
			m.mu.Lock()
			delete(m.active, ab.identity.ID)
			m.inGame[ab.identity.ID] = true
			identity := ab.identity
			m.mu.Unlock()
			m.presence.Online(m.rootCtx, identity.ID)
			go m.play.HandleGame(ctx, identity, gameID, game)
		}
	}
}

func (m *Manager) countBotWaiting(entries []chessapi.LobbyEntry) int {
	ids := m.knownBotIDs()
	n := 0
	for _, e := range entries {
		if ids[e.PlayerID] {
			n++
		}
	}
	return n
}

func (m *Manager) knownBotIDs() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]bool, len(m.active)+len(m.inGame))
	for id := range m.active {
		out[id] = true
	}
	for id := range m.inGame {
		out[id] = true
	}
	return out
}

func (m *Manager) realSeekers(entries []chessapi.LobbyEntry, botIDs map[string]bool) []chessapi.LobbyEntry {
	var out []chessapi.LobbyEntry
	for _, e := range entries {
		if !botIDs[e.PlayerID] {
			out = append(out, e)
		}
	}
	return out
}

func normalizeVariant(v string) string {
	if v == "" {
		return "standard"
	}
	return v
}

func (m *Manager) spawnNear(ctx context.Context, seeker chessapi.LobbyEntry) (*roster.Identity, error) {
	id := roster.NewEphemeral(seeker.Score)
	tc := seeker.TimeControl
	if tc == "" {
		tc = fmt.Sprintf("%d+0", seeker.Minutes)
	}
	if err := m.join(ctx, id, seeker.Minutes, tc, normalizeVariant(seeker.Variant)); err != nil {
		return nil, err
	}
	return &id, nil
}

func (m *Manager) spawnRandom(ctx context.Context) error {
	tc := commonTCs[rand.IntN(len(commonTCs))]
	id := roster.NewEphemeral(0)
	return m.join(ctx, id, tc.minutes, tc.tc, "standard")
}

func (m *Manager) join(ctx context.Context, id roster.Identity, minutes int, tc, variant string) error {
	if err := m.reg.Register(ctx, id.ID); err != nil {
		return fmt.Errorf("register bot: %w", err)
	}
	res, err := m.chess.JoinLobby(ctx, id.ID, id.Name, id.CountryCode, id.Score, minutes, tc, variant, "random")
	if err != nil {
		_ = m.reg.Unregister(ctx, id.ID)
		return err
	}
	m.presence.Online(m.rootCtx, id.ID)
	if res.Status == "matched" && res.Game != nil {
		go m.play.HandleGame(ctx, id, res.GameID, res.Game)
		m.mu.Lock()
		m.inGame[id.ID] = true
		m.mu.Unlock()
		return nil
	}
	lobbyID := res.LobbyID
	if lobbyID == "" && res.Entry != nil {
		lobbyID = res.Entry.ID
	}
	m.mu.Lock()
	m.active[id.ID] = &activeBot{
		identity: id,
		lobbyID:  lobbyID,
		minutes:  minutes,
		tc:       tc,
		variant:  variant,
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) churnLeave(ctx context.Context, n int, protected map[string]bool) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.active))
	for id := range m.active {
		if protected[id] {
			continue
		}
		ids = append(ids, id)
	}
	m.mu.Unlock()
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	if n > len(ids) {
		n = len(ids)
	}
	for i := 0; i < n; i++ {
		m.leaveOne(ctx, ids[i])
	}
}

func (m *Manager) leaveOne(ctx context.Context, playerID string) {
	m.mu.Lock()
	ab, ok := m.active[playerID]
	if ok {
		delete(m.active, playerID)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	_ = m.chess.LeaveLobby(ctx, ab.lobbyID)
	m.retireBot(ctx, playerID)
}

func (m *Manager) retireBot(ctx context.Context, playerID string) {
	m.presence.Offline(playerID)
	_ = m.reg.Unregister(ctx, playerID)
}

func (m *Manager) leaveAll(ctx context.Context) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.active))
	for id := range m.active {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.leaveOne(ctx, id)
	}
	m.presence.CloseAll()
}

func (m *Manager) heartbeatAll(ctx context.Context) {
	m.mu.Lock()
	snap := make([]*activeBot, 0, len(m.active))
	for _, ab := range m.active {
		snap = append(snap, ab)
	}
	m.mu.Unlock()
	for _, ab := range snap {
		if err := m.chess.Heartbeat(ctx, ab.lobbyID, ab.identity.ID); err != nil {
			log.Printf("fleet heartbeat %s: %v", ab.identity.Name, err)
		}
	}
}

func (m *Manager) MarkGameDone(playerID string) {
	m.mu.Lock()
	delete(m.inGame, playerID)
	waiting := false
	if _, ok := m.active[playerID]; ok {
		waiting = true
	}
	m.mu.Unlock()
	// One-shot bots: unregister after the game so the ID is never reused.
	if !waiting {
		m.retireBot(context.Background(), playerID)
	}
}
