package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/labstack/echo/v4"
	"github.com/nikon11211/grpc-gateway/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestServerHTTPRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping HTTP integration test in short mode")
	}

	cfg := server.DefaultConfig()
	cfg.Service = "test-http-requests"
	cfg.HttpAddress = ":0"
	cfg.GrpcAddress = ":0"
	cfg.MetricsAddress = ":0"
	cfg.KeepAlive = false

	srv := server.New(cfg)
	require.NotNil(t, srv)

	router := srv.AddRouter("/api")
	require.NotNil(t, router)

	router.GET("/hello", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"message": "Hello, World!",
		})
	})

	router.GET("/echo/:name", func(c echo.Context) error {
		name := c.Param("name")
		return c.String(http.StatusOK, fmt.Sprintf("Echo: %s", name))
	})

	errChan := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
	}()

	time.Sleep(500 * time.Millisecond)

	select {
	case err := <-errChan:
		t.Fatalf("Server failed to start: %v", err)
	default:
	}

	assert.NotNil(t, srv.EchoServer)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	assert.NoError(t, srv.Stop(ctx))
}

func TestServerGracefulShutdown(t *testing.T) {
	cfg := server.DefaultConfig()
	cfg.Service = "test-graceful-shutdown"
	cfg.HttpAddress = ":0"
	cfg.GrpcAddress = ":0"
	cfg.MetricsAddress = ":0"
	cfg.ShutdownTimeout = 2 * time.Second
	cfg.KeepAlive = false

	srv := server.New(cfg)
	require.NotNil(t, srv)

	router := srv.AddRouter("/api")
	router.GET("/slow", func(c echo.Context) error {
		time.Sleep(100 * time.Millisecond)
		return c.String(http.StatusOK, "slow response")
	})

	go func() {
		_ = srv.Start()
	}()

	time.Sleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	start := time.Now()
	err := srv.Stop(ctx)
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.True(t, elapsed < cfg.ShutdownTimeout+time.Second,
		"Shutdown took too long: %v", elapsed)
}

func TestServerNilSafety(t *testing.T) {
	var srv *server.Server

	assert.NotPanics(t, func() {
		srv.RegisterGRPCServices(func(gs *grpc.Server) {})
	})

	err := srv.RegisterHTTPGateway(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server is nil")

	err = srv.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server is nil")

	err = srv.Stop(context.Background())
	assert.NoError(t, err)

	router := srv.AddRouter("/test")
	assert.Nil(t, router)

	cfg := srv.GetConfig()
	assert.Nil(t, cfg)
}

func TestServerWithCustomMetrics(t *testing.T) {
	cfg1 := server.DefaultConfig()
	cfg1.Service = "custom-metrics-1"
	cfg1.HttpAddress = ":0"
	cfg1.GrpcAddress = ":0"
	cfg1.MetricsAddress = ":0"
	cfg1.KeepAlive = false

	cfg2 := server.DefaultConfig()
	cfg2.Service = "custom-metrics-2"
	cfg2.HttpAddress = ":0"
	cfg2.GrpcAddress = ":0"
	cfg2.MetricsAddress = ":0"
	cfg2.KeepAlive = false

	srv1 := server.New(cfg1)
	srv2 := server.New(cfg2)

	metrics1 := srv1.GetMetricsRegistry()
	metrics2 := srv2.GetMetricsRegistry()

	assert.NotNil(t, metrics1)
	assert.NotNil(t, metrics2)
	assert.NotEqual(t, metrics1.GetRegistry(), metrics2.GetRegistry())

	metrics1.SuccessCounter.WithLabelValues("/Service1/Method").Inc()
	metrics1.SuccessCounter.WithLabelValues("/Service1/Method").Inc()

	metrics2.SuccessCounter.WithLabelValues("/Service2/Method").Inc()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_ = srv1.Stop(ctx)
	_ = srv2.Stop(ctx)
}

func TestExampleServerCreation(t *testing.T) {
	cfg := &server.Config{
		Service:             "test-service-creation",
		HttpAddress:         ":0",
		GrpcAddress:         ":0",
		MetricsAddress:      ":0",
		IdleTimeout:         5 * time.Second,
		ReadTimeout:         5 * time.Second,
		WriteTimeout:        5 * time.Second,
		WriteContextTimeout: 3 * time.Second,
		ShutdownTimeout:     3 * time.Second,
		KeepAlive:           false,
		RateLimit: server.RateLimitConfig{
			Limit:    10,
			Burst:    5,
			ExpireIn: 30 * time.Second,
		},
		MaxRecvMsgSize:    1024 * 1024,
		MaxConnectionIdle: 10 * time.Second,
		Timeout:           10 * time.Second,
		MaxConnectionAge:  10 * time.Second,
		Time:              10 * time.Second,
	}

	srv := server.New(cfg)
	require.NotNil(t, srv)
	assert.NotNil(t, srv.GRPCServer)
	assert.NotNil(t, srv.EchoServer)
	assert.NotNil(t, srv.GetMetricsRegistry())

	srv.RegisterGRPCServices(func(gs *grpc.Server) {
	})

	err := srv.RegisterHTTPGateway(context.Background(),
		func(ctx context.Context, mux *runtime.ServeMux, addr string, opts []grpc.DialOption) error {
			return nil
		},
	)
	require.NoError(t, err)

	router := srv.AddRouter("/test")
	require.NotNil(t, router)

	router.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	errChan := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
	}()

	time.Sleep(500 * time.Millisecond)

	select {
	case err := <-errChan:
		t.Fatalf("Server failed to start: %v", err)
	default:
	}

	retrievedCfg := srv.GetConfig()
	assert.Equal(t, cfg.Service, retrievedCfg.Service)
	assert.Equal(t, cfg.HttpAddress, retrievedCfg.HttpAddress)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = srv.Stop(ctx)
	assert.NoError(t, err)
}

func TestServerConfig(t *testing.T) {
	cfg := server.DefaultConfig()

	assert.Equal(t, "default-service", cfg.Service)
	assert.Equal(t, ":8080", cfg.HttpAddress)
	assert.Equal(t, ":9090", cfg.GrpcAddress)
	assert.Equal(t, ":9091", cfg.MetricsAddress)
	assert.True(t, cfg.KeepAlive)
	assert.Equal(t, float64(100), cfg.RateLimit.Limit)
	assert.Equal(t, 20, cfg.RateLimit.Burst)
	assert.Equal(t, 60*time.Second, cfg.RateLimit.ExpireIn)
	assert.Equal(t, 4*1024*1024, cfg.MaxRecvMsgSize)
	assert.Equal(t, 120*time.Second, cfg.IdleTimeout)
	assert.Equal(t, 30*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.WriteTimeout)
	assert.Equal(t, 5*time.Second, cfg.WriteContextTimeout)
	assert.Equal(t, 10*time.Second, cfg.ShutdownTimeout)

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestServerWithOptions(t *testing.T) {
	cfg := server.DefaultConfig()
	cfg.Service = "test-with-options"
	cfg.HttpAddress = ":0"
	cfg.GrpcAddress = ":0"
	cfg.MetricsAddress = ":0"
	cfg.KeepAlive = false

	customHeaderValue := "test-value"
	customMiddleware := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set("X-Test", customHeaderValue)
			return next(c)
		}
	}

	srv := server.New(cfg,
		server.WithEchoMiddleware(customMiddleware),
	)
	require.NotNil(t, srv)

	router := srv.AddRouter("/api")
	require.NotNil(t, router)

	router.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "test")
	})

	errChan := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
	}()

	time.Sleep(500 * time.Millisecond)

	select {
	case err := <-errChan:
		t.Fatalf("Server failed to start: %v", err)
	default:
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := srv.Stop(ctx)
	assert.NoError(t, err)
}

func TestMultipleServersWithDifferentMetrics(t *testing.T) {
	cfg1 := server.DefaultConfig()
	cfg1.Service = "server-1"
	cfg1.HttpAddress = ":0"
	cfg1.GrpcAddress = ":0"
	cfg1.MetricsAddress = ":0"
	cfg1.KeepAlive = false

	cfg2 := server.DefaultConfig()
	cfg2.Service = "server-2"
	cfg2.HttpAddress = ":0"
	cfg2.GrpcAddress = ":0"
	cfg2.MetricsAddress = ":0"
	cfg2.KeepAlive = false

	srv1 := server.New(cfg1)
	require.NotNil(t, srv1)
	metrics1 := srv1.GetMetricsRegistry()
	require.NotNil(t, metrics1)

	srv2 := server.New(cfg2)
	require.NotNil(t, srv2)
	metrics2 := srv2.GetMetricsRegistry()
	require.NotNil(t, metrics2)

	assert.NotEqual(t, metrics1.GetRegistry(), metrics2.GetRegistry())

	metrics1.SuccessCounter.WithLabelValues("/test.Method1").Inc()
	metrics2.SuccessCounter.WithLabelValues("/test.Method1").Inc()

	metrics1.ErrorCounter.WithLabelValues("/test.Method1", "NotFound").Inc()
	metrics2.ErrorCounter.WithLabelValues("/test.Method1", "NotFound").Inc()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	assert.NoError(t, srv1.Stop(ctx))
	assert.NoError(t, srv2.Stop(ctx))
}

func TestServerHealthEndpoint(t *testing.T) {
	cfg := server.DefaultConfig()
	cfg.Service = "test-health"
	cfg.HttpAddress = ":0"
	cfg.GrpcAddress = ":0"
	cfg.MetricsAddress = ":0"
	cfg.KeepAlive = false

	srv := server.New(cfg)
	require.NotNil(t, srv)

	router := srv.AddRouter("/health-check")
	require.NotNil(t, router)

	router.GET("/status", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status":    "healthy",
			"service":   cfg.Service,
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	go func() {
		_ = srv.Start()
	}()

	time.Sleep(500 * time.Millisecond)

	assert.NotNil(t, router)

	retrievedCfg := srv.GetConfig()
	assert.Equal(t, "test-health", retrievedCfg.Service)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	assert.NoError(t, srv.Stop(ctx))
}

func BenchmarkServerCreation(b *testing.B) {
	cfg := server.DefaultConfig()
	cfg.HttpAddress = ":0"
	cfg.GrpcAddress = ":0"
	cfg.MetricsAddress = ":0"
	cfg.KeepAlive = false

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg.Service = fmt.Sprintf("bench-service-%d", i)
		srv := server.New(cfg)
		require.NotNil(b, srv)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		_ = srv.Stop(ctx)
		cancel()
	}
}
