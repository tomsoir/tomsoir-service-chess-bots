# tomsoir-service-chess-bots

Lobby bot fleet for the chess stack. Joins the normal lobby as fake players, plays matched games via Stockfish, and keeps an adaptive waiting count (~2–12 by time of day).

## Env

| Variable | Default | Notes |
|----------|---------|-------|
| `BOTS_ENABLED` | false | Set `true` to run the fleet |
| `APP_PORT` | 9600 | Health endpoint |
| `CHESS_HTTP_ADDR` | `http://localhost:9200` | Chess REST API |
| `REALTIME_WS_ADDR` | `ws://localhost:9300` | Game WS for emoji chat |
| `ENGINE_GRPC_ADDR` | — | Required when enabled |
| `REDIS_ADDR` | `localhost:6379` | Registers bot player IDs |
| `BOTS_MIN_VISIBLE` / `BOTS_MAX_VISIBLE` | 2 / 12 | Fleet size bounds |
| `BOTS_TIMEZONE` | `America/Los_Angeles` | Adaptive curve |
| `BOTS_ENGINE_MAX_CONCURRENCY` | 2 | Cap parallel Stockfish calls |

## Local stack

Wired in `tomsoir-cluster/local/chess-stack` as service `bots`. Kill switch: `BOTS_ENABLED=false`.

## Helm / K8s

Chart: `tomsoir-cluster/helm/tomsoir-service-chess-bots-chart`

```bash
helm install tomsoir-service-chess-bots-release ./tomsoir-service-chess-bots-chart
```

See the chart README for upgrade, kill switch (`bots.enabled=false`), and debug commands.
