package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestNewServer(t *testing.T) {
	t.Run("with default config", func(t *testing.T) {
		srv := New(nil)
		require.NotNil(t, srv)
		assert.NotNil(t, srv.GRPCServer)
		assert.NotNil(t, srv.EchoServer)
		assert.NotNil(t, srv.config)
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &Config{
			Service:             "test-service",
			HttpAddress:         ":8081",
			GrpcAddress:         ":9091",
			MetricsAddress:      ":9200",
			IdleTimeout:         60 * time.Second,
			ReadTimeout:         10 * time.Second,
			WriteTimeout:        10 * time.Second,
			WriteContextTimeout: 3 * time.Second,
			ShutdownTimeout:     5 * time.Second,
			KeepAlive:           false,
			RateLimit: RateLimitConfig{
				Limit:    50,
				Burst:    10,
				ExpireIn: 30 * time.Second,
			},
			MaxRecvMsgSize:    1024 * 1024,
			MaxConnectionIdle: 30 * time.Second,
			Timeout:           30 * time.Second,
			MaxConnectionAge:  30 * time.Second,
			Time:              30 * time.Second,
		}

		srv := New(cfg)
		require.NotNil(t, srv)
		assert.Equal(t, ":8081", srv.httpPort)
		assert.Equal(t, ":9091", srv.grpcPort)
		assert.Equal(t, ":9200", srv.metricsAddress)
	})

	t.Run("with options", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MetricsAddress = ":9201"
		srv := New(cfg, WithGRPCOption(grpc.MaxRecvMsgSize(1024)))
		require.NotNil(t, srv)
	})
}

func TestServerNilSafety(t *testing.T) {
	t.Run("nil server RegisterGRPCServices", func(t *testing.T) {
		var srv *Server
		assert.NotPanics(t, func() {
			srv.RegisterGRPCServices(func(server *grpc.Server) {})
		})
	})

	t.Run("nil server RegisterHTTPGateway", func(t *testing.T) {
		var srv *Server
		err := srv.RegisterHTTPGateway(context.Background(), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "server is nil")
	})

	t.Run("nil server Start", func(t *testing.T) {
		var srv *Server
		err := srv.Start()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "server is nil")
	})

	t.Run("nil server Stop", func(t *testing.T) {
		var srv *Server
		err := srv.Stop(context.Background())
		assert.NoError(t, err)
	})

	t.Run("nil server AddRouter", func(t *testing.T) {
		var srv *Server
		router := srv.AddRouter("test")
		assert.Nil(t, router)
	})

	t.Run("nil server GetConfig", func(t *testing.T) {
		var srv *Server
		cfg := srv.GetConfig()
		assert.Nil(t, cfg)
	})
}

func TestRegisterGRPCServices(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MetricsAddress = ":9202"
	srv := New(cfg)

	called := false
	srv.RegisterGRPCServices(func(server *grpc.Server) {
		called = true
	})

	assert.True(t, called)
}

func TestAddRouter(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MetricsAddress = ":9203"
	srv := New(cfg)

	router := srv.AddRouter("/api")
	require.NotNil(t, router)
	assert.Equal(t, "/api", router.name)
	assert.NotNil(t, router.Group)

	assert.Len(t, srv.groups, 1)
	assert.Contains(t, srv.groups, "/api")
}

func TestGetConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MetricsAddress = ":9204"
	srv := New(cfg)

	retrievedCfg := srv.GetConfig()
	assert.NotNil(t, retrievedCfg)
	assert.Equal(t, cfg.Service, retrievedCfg.Service)
}

func TestExtractMethod(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/package.Service/Method", "Method"},
		{"/package.Service/GetUser", "GetUser"},
		{"MethodOnly", "MethodOnly"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractMethod(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
