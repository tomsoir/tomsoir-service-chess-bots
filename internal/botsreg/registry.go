package botsreg

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyBotIDs = "chess:bots:ids"

type Registry struct {
	client *redis.Client
}

func New(addr, password string) (*Registry, error) {
	opts := &redis.Options{Addr: addr}
	if password != "" {
		opts.Password = password
	}
	client := redis.NewClient(opts)
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("botsreg redis: %w", err)
	}
	return &Registry{client: client}, nil
}

func (r *Registry) Close() error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Close()
}

func (r *Registry) Clear(ctx context.Context) error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Del(ctx, keyBotIDs).Err()
}

func (r *Registry) Register(ctx context.Context, id string) error {
	if r == nil || r.client == nil || id == "" {
		return nil
	}
	return r.client.SAdd(ctx, keyBotIDs, id).Err()
}

func (r *Registry) Unregister(ctx context.Context, id string) error {
	if r == nil || r.client == nil || id == "" {
		return nil
	}
	return r.client.SRem(ctx, keyBotIDs, id).Err()
}

func (r *Registry) RegisterAll(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return r.client.SAdd(ctx, keyBotIDs, args...).Err()
}

func (r *Registry) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return r.client.Ping(ctx).Err()
}
