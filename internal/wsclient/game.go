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

type Message struct {
	Type     string `json:"type"`
	PlayerID string `json:"player_id,omitempty"`
	Text     string `json:"text,omitempty"`
}

type Conn struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func DialGame(ctx context.Context, wsBase, gameID, playerID string, onMsg func(Message)) *Conn {
	u, err := url.Parse(wsBase + "/v1/ws/game")
	if err != nil {
		return &Conn{}
	}
	q := u.Query()
	q.Set("game_id", gameID)
	q.Set("player_id", playerID)
	u.RawQuery = q.Encode()

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		log.Printf("ws dial game %s: %v", gameID, err)
		return &Conn{}
	}
	c := &Conn{conn: conn}
	_ = c.SendPresence(true)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.SendPresence(true); err != nil {
					return
				}
			}
		}
	}()
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg Message
			if json.Unmarshal(data, &msg) == nil && onMsg != nil {
				onMsg(msg)
			}
		}
	}()
	return c
}

func (c *Conn) SendPresence(focused bool) error {
	if c == nil || c.conn == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	payload, _ := json.Marshal(map[string]any{"type": "presence", "focused": focused})
	_ = c.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	return c.conn.WriteMessage(websocket.TextMessage, payload)
}

func (c *Conn) SendEmoji(emoji string) error {
	if c == nil || c.conn == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	payload, _ := json.Marshal(map[string]string{"type": "chat_bubble", "text": emoji})
	_ = c.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	return c.conn.WriteMessage(websocket.TextMessage, payload)
}

func (c *Conn) Close() {
	if c == nil || c.conn == nil {
		return
	}
	_ = c.conn.Close()
}
