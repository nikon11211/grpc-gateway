package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/labstack/echo/v4"
	"github.com/nikon11211/grpc-gateway/server"
	"google.golang.org/grpc"
)

func main() {
	cfg := &server.Config{
		Service:             "example-service",
		HttpAddress:         ":8080",
		GrpcAddress:         ":9090",
		MetricsAddress:      ":9091",
		IdleTimeout:         120 * time.Second,
		ReadTimeout:         30 * time.Second,
		WriteTimeout:        30 * time.Second,
		WriteContextTimeout: 5 * time.Second,
		ShutdownTimeout:     10 * time.Second,
		KeepAlive:           true,
		RateLimit: server.RateLimitConfig{
			Limit:    100,
			Burst:    20,
			ExpireIn: 60 * time.Second,
		},
		MaxRecvMsgSize:    4 * 1024 * 1024,
		MaxConnectionIdle: 60 * time.Second,
		Timeout:           60 * time.Second,
		MaxConnectionAge:  60 * time.Second,
		Time:              60 * time.Second,
	}

	srv := server.New(cfg,
		server.WithEchoMiddleware(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				c.Response().Header().Set("X-Custom-Header", "example-value")
				return next(c)
			}
		}),
	)

	srv.RegisterGRPCServices(func(gs *grpc.Server) {
		log.Println("Registering gRPC services...")
	})

	if err := srv.RegisterHTTPGateway(context.Background(),
		func(ctx context.Context, mux *runtime.ServeMux, addr string, opts []grpc.DialOption) error {
			log.Printf("Registering HTTP gateway for gRPC server at %s...", addr)
			return nil
		},
	); err != nil {
		log.Fatalf("Failed to register HTTP gateway: %v", err)
	}

	apiRouter := srv.AddRouter("/api")
	apiRouter.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]any{
			"status":    "healthy",
			"service":   cfg.Service,
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	apiRouter.GET("/info", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]any{
			"http_address":    cfg.HttpAddress,
			"grpc_address":    cfg.GrpcAddress,
			"metrics_address": cfg.MetricsAddress,
			"rate_limit":      cfg.RateLimit.Limit,
		})
	})

	go func() {
		fmt.Printf("Starting %s server...\n", cfg.Service)
		fmt.Printf("HTTP server: http://localhost%s\n", cfg.HttpAddress)
		fmt.Printf("gRPC server: localhost%s\n", cfg.GrpcAddress)
		fmt.Printf("Metrics server: http://localhost%s/metrics\n", cfg.MetricsAddress)
		fmt.Printf("Health check: http://localhost%s/api/health\n", cfg.HttpAddress)

		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down servers...")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Stop(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped successfully")
}
