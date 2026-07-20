package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func Enabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("BOTS_ENABLED")))
	return v == "1" || v == "true" || v == "yes"
}

func HTTPPort() string {
	if p := os.Getenv("APP_PORT"); p != "" {
		return p
	}
	return "9600"
}

func ChessHTTPBase() string {
	if a := strings.TrimSpace(os.Getenv("CHESS_HTTP_ADDR")); a != "" {
		return strings.TrimRight(a, "/")
	}
	return "http://localhost:9200"
}

func RealtimeWSBase() string {
	if a := strings.TrimSpace(os.Getenv("REALTIME_WS_ADDR")); a != "" {
		return strings.TrimRight(a, "/")
	}
	return "ws://localhost:9300"
}

func EngineGRPCAddr() string {
	return strings.TrimSpace(os.Getenv("ENGINE_GRPC_ADDR"))
}

func RedisAddr() string {
	if a := strings.TrimSpace(os.Getenv("REDIS_ADDR")); a != "" {
		return a
	}
	return "localhost:6379"
}

func RedisPassword() string {
	return os.Getenv("PASSWORD")
}

func MinVisible() int {
	return envInt("BOTS_MIN_VISIBLE", 2)
}

func MaxVisible() int {
	return envInt("BOTS_MAX_VISIBLE", 12)
}

func Timezone() *time.Location {
	name := strings.TrimSpace(os.Getenv("BOTS_TIMEZONE"))
	if name == "" {
		name = "America/Los_Angeles"
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.Local
	}
	return loc
}

func EngineMaxConcurrency() int {
	return envInt("BOTS_ENGINE_MAX_CONCURRENCY", 2)
}

func FleetTick() time.Duration {
	sec := envInt("BOTS_FLEET_TICK_SEC", 2)
	return time.Duration(sec) * time.Second
}

func HeartbeatEvery() time.Duration {
	sec := envInt("BOTS_HEARTBEAT_SEC", 30)
	return time.Duration(sec) * time.Second
}

// SeekerGrace is how long a real player may wait alone before we force a bot match.
func SeekerGrace() time.Duration {
	sec := envInt("BOTS_SEEKER_GRACE_SEC", 7)
	return time.Duration(sec) * time.Second
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
