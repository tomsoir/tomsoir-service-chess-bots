package wsclient

import (
	"context"
	"encoding/json"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const sitePingEvery = 10 * time.Second

// SiteSession keeps a bot counted in the footer ONLINE indicator, matching real clients.
type SiteSession struct {
	cancel context.CancelFunc
}

func DialSite(parent context.Context, wsBase, playerID string) *SiteSession {
	ctx, cancel := context.WithCancel(parent)
	s := &SiteSession{cancel: cancel}
	go s.loop(ctx, wsBase, playerID)
	return s
}

func (s *SiteSession) Close() {
	if s != nil && s.cancel != nil {
		s.cancel()
	}
}

func (s *SiteSession) loop(ctx context.Context, wsBase, playerID string) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.runOnce(ctx, wsBase, playerID); err != nil && ctx.Err() == nil {
			log.Printf("site ws %s: %v", playerID[:8], err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (s *SiteSession) runOnce(ctx context.Context, wsBase, playerID string) error {
	u, err := url.Parse(wsBase + "/v1/ws/site")
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("player_id", playerID)
	u.RawQuery = q.Encode()

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(sitePingEvery)
	defer ticker.Stop()
	ping, _ := json.Marshal(map[string]string{"type": "ping"})
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-done:
			return nil
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, ping); err != nil {
				return err
			}
		}
	}
}

// PresenceHub tracks site sockets for bots that are currently "on the site".
type PresenceHub struct {
	wsBase string
	mu     sync.Mutex
	live   map[string]*SiteSession
}

func NewPresenceHub(wsBase string) *PresenceHub {
	return &PresenceHub{wsBase: wsBase, live: map[string]*SiteSession{}}
}

func (h *PresenceHub) Online(ctx context.Context, playerID string) {
	if h == nil || playerID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.live[playerID]; ok {
		return
	}
	h.live[playerID] = DialSite(ctx, h.wsBase, playerID)
}

func (h *PresenceHub) Offline(playerID string) {
	if h == nil || playerID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.live[playerID]; ok {
		s.Close()
		delete(h.live, playerID)
	}
}

func (h *PresenceHub) CloseAll() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, s := range h.live {
		s.Close()
		delete(h.live, id)
	}
}
