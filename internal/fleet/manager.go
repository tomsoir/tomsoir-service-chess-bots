package fleet

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"sync"
	"time"

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
	roster   []roster.Identity
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

func New(chess *chessapi.Client, identities []roster.Identity, driver *play.Driver, presence *wsclient.PresenceHub) *Manager {
	return &Manager{
		chess:    chess,
		roster:   identities,
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
	// Quiet nights ~2-4, daytime ~4-8, evenings ~8-12
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

	target := m.targetVisible(time.Now())
	botWaiting := m.countBotWaiting(entries)
	realSeekers := m.realSeekers(entries)

	// Ensure in-band bots for lone real seekers.
	for _, seeker := range realSeekers {
		if m.hasInBandBot(entries, seeker) {
			continue
		}
		if err := m.spawnNear(ctx, seeker); err != nil {
			log.Printf("fleet spawn near %s: %v", seeker.PlayerID, err)
		}
	}

	botWaiting = len(m.snapshotActive())
	if botWaiting < target {
		need := target - botWaiting
		for i := 0; i < need; i++ {
			if err := m.spawnRandom(ctx); err != nil {
				log.Printf("fleet spawn: %v", err)
				break
			}
		}
	} else if botWaiting > target {
		// Leave extras, optionally as fake bot-vs-bot churn.
		extra := botWaiting - target
		m.churnLeave(ctx, extra)
	} else if rand.IntN(5) == 0 && botWaiting >= 2 {
		// Occasional fake "started a game" churn.
		m.churnLeave(ctx, 1+rand.IntN(2))
	}

	m.pollStatuses(ctx)
}

func (m *Manager) reconcileMatched(ctx context.Context, entries []chessapi.LobbyEntry) {
	present := map[string]bool{}
	for _, e := range entries {
		present[e.PlayerID] = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, ab := range m.active {
		if present[id] {
			continue
		}
		// Missing from lobby — check if matched into a game.
		status, gameID, game, err := m.chess.LobbyStatus(ctx, ab.lobbyID)
		if err == nil && status == "matched" && game != nil {
			m.inGame[id] = true
			delete(m.active, id)
			m.presence.Online(m.rootCtx, ab.identity.ID)
			go m.play.HandleGame(ctx, ab.identity, gameID, game)
			continue
		}
		m.presence.Offline(id)
		delete(m.active, id)
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
			m.mu.Unlock()
			m.presence.Online(m.rootCtx, ab.identity.ID)
			go m.play.HandleGame(ctx, ab.identity, gameID, game)
		}
	}
}

func (m *Manager) countBotWaiting(entries []chessapi.LobbyEntry) int {
	ids := m.rosterIDs()
	n := 0
	for _, e := range entries {
		if ids[e.PlayerID] {
			n++
		}
	}
	return n
}

func (m *Manager) rosterIDs() map[string]bool {
	out := make(map[string]bool, len(m.roster))
	for _, id := range m.roster {
		out[id.ID] = true
	}
	return out
}

func (m *Manager) realSeekers(entries []chessapi.LobbyEntry) []chessapi.LobbyEntry {
	ids := m.rosterIDs()
	var out []chessapi.LobbyEntry
	for _, e := range entries {
		if !ids[e.PlayerID] {
			out = append(out, e)
		}
	}
	return out
}

func (m *Manager) hasInBandBot(entries []chessapi.LobbyEntry, seeker chessapi.LobbyEntry) bool {
	ids := m.rosterIDs()
	for _, e := range entries {
		if !ids[e.PlayerID] {
			continue
		}
		if e.Minutes != seeker.Minutes {
			continue
		}
		if normalizeVariant(e.Variant) != normalizeVariant(seeker.Variant) {
			continue
		}
		if roster.WithinBand(e.Score, seeker.Score) {
			return true
		}
	}
	return false
}

func normalizeVariant(v string) string {
	if v == "" {
		return "standard"
	}
	return v
}

func (m *Manager) spawnNear(ctx context.Context, seeker chessapi.LobbyEntry) error {
	level := roster.LevelForScore(seeker.Score)
	id := m.pickIdleIdentity(level, seeker.Score)
	if id == nil {
		return nil
	}
	score := roster.ScoreNear(seeker.Score, level)
	id.Score = score
	id.EngineLevel = level
	tc := seeker.TimeControl
	if tc == "" {
		tc = fmt.Sprintf("%d+0", seeker.Minutes)
	}
	return m.join(ctx, *id, seeker.Minutes, tc, normalizeVariant(seeker.Variant))
}

func (m *Manager) spawnRandom(ctx context.Context) error {
	tc := commonTCs[rand.IntN(len(commonTCs))]
	id := m.pickIdleIdentity(0, 0)
	if id == nil {
		return nil
	}
	return m.join(ctx, *id, tc.minutes, tc.tc, "standard")
}

func (m *Manager) pickIdleIdentity(preferLevel, nearScore int) *roster.Identity {
	m.mu.Lock()
	defer m.mu.Unlock()
	var candidates []roster.Identity
	for _, id := range m.roster {
		if _, ok := m.active[id.ID]; ok {
			continue
		}
		if m.inGame[id.ID] {
			continue
		}
		if preferLevel > 0 && id.EngineLevel != preferLevel {
			continue
		}
		if nearScore > 0 && !roster.WithinBand(id.Score, nearScore) {
			continue
		}
		candidates = append(candidates, id)
	}
	if len(candidates) == 0 && preferLevel > 0 {
		// Relax level filter; keep score band if possible.
		for _, id := range m.roster {
			if _, ok := m.active[id.ID]; ok || m.inGame[id.ID] {
				continue
			}
			if nearScore > 0 && !roster.WithinBand(id.Score, nearScore) {
				continue
			}
			candidates = append(candidates, id)
		}
	}
	if len(candidates) == 0 {
		for _, id := range m.roster {
			if _, ok := m.active[id.ID]; ok || m.inGame[id.ID] {
				continue
			}
			candidates = append(candidates, id)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	picked := candidates[rand.IntN(len(candidates))]
	return &picked
}

func (m *Manager) join(ctx context.Context, id roster.Identity, minutes int, tc, variant string) error {
	res, err := m.chess.JoinLobby(ctx, id.ID, id.Name, id.CountryCode, id.Score, minutes, tc, variant, "random")
	if err != nil {
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

func (m *Manager) churnLeave(ctx context.Context, n int) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.active))
	for id := range m.active {
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
	m.presence.Offline(playerID)
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

func (m *Manager) snapshotActive() map[string]*activeBot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]*activeBot, len(m.active))
	for k, v := range m.active {
		out[k] = v
	}
	return out
}

func (m *Manager) MarkGameDone(playerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.inGame, playerID)
	// Bot finished a game and is no longer waiting — drop site presence until respawned.
	if _, waiting := m.active[playerID]; !waiting {
		m.presence.Offline(playerID)
	}
}
