package engineclient

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	enginev1 "tomsoir-service-chess-bots/api/engine/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type Client struct {
	addr string
	mu   sync.Mutex
	conn *grpc.ClientConn
	stub enginev1.ChessEngineServiceClient
	sem  chan struct{}
}

func New(addr string, maxConcurrent int) (*Client, error) {
	if addr == "" {
		return nil, fmt.Errorf("ENGINE_GRPC_ADDR not set")
	}
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &Client{addr: addr, sem: make(chan struct{}, maxConcurrent)}, nil
}

func (c *Client) connect(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	grpcConn, err := grpc.DialContext(ctx, c.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("engine grpc dial: %w", err)
	}
	c.conn = grpcConn
	c.stub = enginev1.NewChessEngineServiceClient(grpcConn)
	return nil
}

func reconnectable(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled, codes.Aborted:
			return true
		}
	}
	return strings.Contains(err.Error(), "connection") || strings.Contains(err.Error(), "transport")
}

func (c *Client) withStub(ctx context.Context, fn func(enginev1.ChessEngineServiceClient) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stub == nil {
		if err := c.connect(ctx); err != nil {
			return err
		}
	}
	err := fn(c.stub)
	if !reconnectable(err) {
		return err
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = nil
	c.stub = nil
	if dialErr := c.connect(ctx); dialErr != nil {
		return err
	}
	return fn(c.stub)
}

func (c *Client) GetBestMove(ctx context.Context, fen, variant string, engineLevel, movetimeMS int) (string, error) {
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	var uci string
	err := c.withStub(ctx, func(stub enginev1.ChessEngineServiceClient) error {
		res, callErr := stub.GetBestMove(ctx, &enginev1.GetBestMoveRequest{
			Fen:         fen,
			Variant:     variant,
			EngineLevel: int32(engineLevel),
			MovetimeMs:  int32(movetimeMS),
		})
		if callErr != nil {
			return callErr
		}
		uci = res.GetUci()
		return nil
	})
	return uci, err
}
