package chessapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	base   string
	http   *http.Client
}

func New(base string) *Client {
	return &Client{
		base: base,
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

type Player struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CountryCode string `json:"country_code,omitempty"`
	Score       int    `json:"score"`
}

type LobbyEntry struct {
	ID          string    `json:"id"`
	PlayerID    string    `json:"player_id"`
	PlayerName  string    `json:"player_name"`
	CountryCode string    `json:"country_code,omitempty"`
	Score       int       `json:"score"`
	Minutes     int       `json:"minutes"`
	TimeControl string    `json:"time_control"`
	Variant     string    `json:"variant"`
	ColorPref   string    `json:"color_pref"`
	JoinedAt    time.Time `json:"joined_at"`
	LastSeenAt  time.Time `json:"last_seen_at,omitempty"`
}

type Game struct {
	ID           string   `json:"id"`
	Mode         string   `json:"mode"`
	Status       string   `json:"status"`
	Variant      string   `json:"variant"`
	Minutes      int      `json:"minutes"`
	IncrementSec int      `json:"increment_sec"`
	FEN          string   `json:"fen"`
	Turn         string   `json:"turn"`
	White        *Player  `json:"white,omitempty"`
	Black        *Player  `json:"black,omitempty"`
	Moves        []string `json:"moves"`
	WhiteClock   int      `json:"white_clock_ms"`
	BlackClock   int      `json:"black_clock_ms"`
	OpeningComplete bool  `json:"opening_complete,omitempty"`
}

type JoinResult struct {
	LobbyID string      `json:"lobby_id"`
	PlayerID string     `json:"player_id"`
	Entry   *LobbyEntry `json:"entry"`
	Status  string      `json:"status"`
	GameID  string      `json:"game_id,omitempty"`
	Game    *Game       `json:"game,omitempty"`
	Entries []LobbyEntry `json:"entries,omitempty"`
}

func (c *Client) JoinLobby(ctx context.Context, playerID, name, country string, score, minutes int, timeControl, variant, colorPref string) (*JoinResult, error) {
	body := map[string]any{
		"player_id":    playerID,
		"player_name":  name,
		"country_code": country,
		"score":        score,
		"minutes":      minutes,
		"time_control": timeControl,
		"variant":      variant,
		"color_pref":   colorPref,
	}
	var out JoinResult
	if err := c.post(ctx, "/v1/lobby/join", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Heartbeat(ctx context.Context, lobbyID, playerID string) error {
	return c.post(ctx, "/v1/lobby/heartbeat", map[string]any{
		"lobby_id":  lobbyID,
		"player_id": playerID,
	}, nil)
}

func (c *Client) LeaveLobby(ctx context.Context, lobbyID string) error {
	return c.postNoContent(ctx, "/v1/lobby/leave", map[string]any{"lobby_id": lobbyID})
}

type MatchResult struct {
	Status string `json:"status"`
	GameID string `json:"game_id"`
	Color  string `json:"color"`
	Game   *Game  `json:"game"`
}

func (c *Client) Match(ctx context.Context, playerID, lobbyID, targetLobbyID string) (*MatchResult, error) {
	var out MatchResult
	if err := c.post(ctx, "/v1/lobby/match", map[string]any{
		"player_id":        playerID,
		"lobby_id":         lobbyID,
		"target_lobby_id":  targetLobbyID,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListWaiting(ctx context.Context) ([]LobbyEntry, error) {
	var out struct {
		Entries []LobbyEntry `json:"entries"`
	}
	if err := c.get(ctx, "/v1/lobby/waiting", &out); err != nil {
		return nil, err
	}
	return out.Entries, nil
}

func (c *Client) LobbyStatus(ctx context.Context, lobbyID string) (status, gameID string, game *Game, err error) {
	var out struct {
		Status string `json:"status"`
		GameID string `json:"game_id"`
		Game   *Game  `json:"game"`
	}
	if err = c.get(ctx, "/v1/lobby/"+lobbyID+"/status", &out); err != nil {
		return "", "", nil, err
	}
	return out.Status, out.GameID, out.Game, nil
}

func (c *Client) GetGame(ctx context.Context, gameID string) (*Game, error) {
	var g Game
	if err := c.get(ctx, "/v1/games/"+gameID, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

func (c *Client) MakeMove(ctx context.Context, gameID, playerID, uci string) (*Game, error) {
	var g Game
	if err := c.post(ctx, "/v1/games/"+gameID+"/move", map[string]any{
		"player_id": playerID,
		"uci":       uci,
	}, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s: %s (%s)", path, res.Status, string(raw))
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *Client) postNoContent(ctx context.Context, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(res.Body)
		return fmt.Errorf("%s: %s (%s)", path, res.Status, string(raw))
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s: %s (%s)", path, res.Status, string(raw))
	}
	return json.Unmarshal(raw, out)
}
