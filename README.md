<p align="center">
  <h1 align="center">gRPC-Gateway Server - Enterprise-Grade Server Library for Go</h1>
</p>

<p align="center">
  <a href="https://pkg.go.dev/github.com/nikon11211/grpc-gateway">
    <img src="https://pkg.go.dev/badge/github.com/nikon11211/grpc-gateway.svg" alt="Go Reference"/>
  </a>
  <a href="https://goreportcard.com/report/github.com/nikon11211/grpc-gateway">
    <img src="https://goreportcard.com/badge/github.com/nikon11211/grpc-gateway" alt="Go Report Card"/>
  </a>
  <a href="https://github.com/nikon11211/grpc-gateway/actions/workflows/test.yaml">
    <img src="https://github.com/nikon11211/grpc-gateway/actions/workflows/test.yaml/badge.svg" alt="Tests"/>
  </a>
  <a href="https://codecov.io/gh/nikon11211/grpc-gateway">
    <img src="https://codecov.io/gh/nikon11211/grpc-gateway/branch/main/graph/badge.svg" alt="Coverage"/>
  </a>
  <a href="https://opensource.org/licenses/MIT">
    <img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"/>
  </a>
  <a href="https://golang.org/">
    <img src="https://img.shields.io/badge/Go-%3E%3D%201.21-blue" alt="Go Version"/>
  </a>
</p>

<p align="center">
  <b>A high-performance, production-ready gRPC-Gateway server library for Go microservices</b><br/>
  <i>Echo framework • gRPC • OpenTelemetry tracing • Prometheus metrics • Rate limiting</i>
</p>

---

## ✨ Why gRPC-Gateway Server?

This library provides a complete, production-ready gRPC-Gateway server with built-in support for HTTP REST APIs, gRPC services, distributed tracing, Prometheus metrics, rate limiting, and structured logging. Perfect for building modern microservices that need both gRPC and REST interfaces.

```go
// Create server with single function call
srv := server.New(config,
server.WithTracerProvider(tp),
)

// Register your gRPC services
srv.RegisterGRPCServices(func(gs *grpc.Server) {
pb.RegisterYourServiceServer(gs, &YourService{})
})

// Register HTTP gateway
srv.RegisterHTTPGateway(ctx, pb.RegisterYourServiceHandlerFromEndpoint)

// Start all servers (gRPC + HTTP + Metrics)
srv.Start()
```

---

## 🎯 Features

<table>
<tr>
<td width="50%">

### 🚀 Core Features
- gRPC Server with keepalive configuration and interceptors
- HTTP REST API via gRPC-Gateway with automatic routing
- Prometheus Metrics with built-in endpoint and middleware
- Rate Limiting with configurable limits and burst
- Request ID generation via UUID
- CORS support out of the box
- Graceful Shutdown with configurable timeout

### 📊 Observability
- OpenTelemetry Tracing for both gRPC and HTTP
- Structured Logging with slog
- Request Duration Metrics via Prometheus histograms
- Success/Error Counters for gRPC methods
- Automatic Trace Context Propagation

</td>
<td width="50%">

### 🔒 Enterprise Ready
- TLS Support via gRPC options
- Configurable Timeouts for all connections
- Keepalive with MaxConnectionIdle, Timeout, MaxConnectionAge
- Context Timeout for request processing
- Max Message Size configuration

</td>
</tr>
</table>

## 📦 Installation

```bash
go get github.com/nikon11211/grpc-gateway
```

## 🏗️ Architecture

```
┌───────────────────────────────────────────────────────────┐
│                    Your Application                       │
├───────────────────────────────────────────────────────────┤
│                 gRPC-Gateway Server Library               │
│                                                           │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────────┐    │
│  │  gRPC    │ │  HTTP    │ │ Metrics  │ │  Tracing   │    │
│  │  Server  │ │  Server  │ │  Server  │ │  (OTEL)    │    │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └─────┬──────┘    │
│       └────────────┴────────────┴─────────────┘           │
│                          │                                │
│                   Echo Framework                          │
│                          │                                │
│                 gRPC-Gateway Runtime                      │
└───────────────────────────────────────────────────────────┘
```

## 🚀 Quick Start

### Basic Server

```go
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/nikon11211/grpc-gateway/server"
	"google.golang.org/grpc"
)

func main() {
	cfg := &server.Config{
		Service:             "my-service",
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

	srv := server.New(cfg)

	srv.RegisterGRPCServices(func(gs *grpc.Server) {
		// Register your gRPC services here
	})

	srv.RegisterHTTPGateway(context.Background(),
		func(ctx context.Context, mux *runtime.ServeMux, addr string, opts []grpc.DialOption) error {
			// Register your HTTP handlers here
			return nil
		},
	)

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	srv.Stop(ctx)
}
```

### With OpenTelemetry Tracing

```go
import "go.opentelemetry.io/otel/trace/noop"

tp := noop.NewTracerProvider()

srv := server.New(cfg,
server.WithTracerProvider(tp),
)
```

### With Custom Middleware

```go
srv := server.New(cfg,
server.WithEchoMiddleware(func(next echo.HandlerFunc) echo.HandlerFunc {
return func(c echo.Context) error {
// Custom middleware logic
return next(c)
}
}),
)
```

### Distributed Tracing with OpenTelemetry

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/trace"
)

func handleOrder(ctx context.Context, orderID string) {
    tracer := otel.Tracer("order-service")
    ctx, span := tracer.Start(ctx, "handleOrder")
    defer span.End()
    
    // TraceID and SpanID automatically injected into logs
    log.InfoCtx(ctx, "Processing order")
    
    // All downstream logs will include trace context
    processPayment(ctx, orderID)
}
```

## 🔧 Configuration Reference

```go
type Config struct {
Service             string          // Service name (required)
HttpAddress         string          // HTTP server address (required)
GrpcAddress         string          // gRPC server address (required)
MetricsAddress      string          // Metrics server address (required)
IdleTimeout         time.Duration   // Max idle time for connections
ReadTimeout         time.Duration   // Max read time for requests
WriteTimeout        time.Duration   // Max write time for responses
WriteContextTimeout time.Duration   // Context timeout for requests
ShutdownTimeout     time.Duration   // Graceful shutdown timeout
KeepAlive           bool            // Enable TCP keep-alive
RateLimit           RateLimitConfig // Rate limiting configuration
MaxRecvMsgSize      int             // Max gRPC message size
MaxConnectionIdle   time.Duration   // Max idle time for gRPC connections
Timeout             time.Duration   // gRPC keepalive timeout
MaxConnectionAge    time.Duration   // Max gRPC connection age
Time                time.Duration   // gRPC keepalive ping interval
}
```

## 🧪 Testing

```go
// Run all tests
go test ./...

// Run with race detection
go test -race ./...
 
// Run with coverage
go test -coverprofile=coverage.txt ./...
go tool cover -html=coverage.txt
```

## 🤝 Contributing

We welcome contributions! Here's how you can help:

1. **Fork** the repository
2. **Create** a feature branch (`git checkout -b feature/amazing-feature`)
3. **Commit** your changes (`git commit -m 'Add amazing feature'`)
4. **Push** to the branch (`git push origin feature/amazing-feature`)
5. **Open** a Pull Request

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.

## 🌟 Show Your Support

Give a ⭐️ if this project helped you! Share it with your team to improve logging across your microservices.

## 🙏 Acknowledgments

- [Echo](https://github.com/labstack/echo/v4/) - High performance HTTP framework
- [gRPC-Go](https://google.golang.org/grpc/) - The official Go gRPC implementation
- [gRPC-Gateway](https://github.com/grpc-ecosystem/grpc-gateway/v2/) - gRPC to JSON proxy generator
- [OpenTelemetry](https://opentelemetry.io/) - Distributed tracing standard
---

<p align="center">
  <b>Made with ❤️ for the Go community</b><br/>
  <sub>Built for performance, designed for reliability</sub>
</p>