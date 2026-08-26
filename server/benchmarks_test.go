package server

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"google.golang.org/grpc"
)

var (
	gwBenchAny any
	gwBenchErr error
	gwBenchStr string
	gwBenchCfg *Config
)

func BenchmarkExtractMethod(b *testing.B) {
	for i := 0; i < b.N; i++ {
		gwBenchStr = extractMethod("/app.orders.v1.OrderService/GetByID")
	}
}

func BenchmarkConfigValidate(b *testing.B) {
	cfg := DefaultConfig()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		gwBenchErr = cfg.Validate()
	}
}

func BenchmarkServerNew(b *testing.B) {
	cfg := DefaultConfig()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		gwBenchCfg = cfg
		_ = New(gwBenchCfg)
	}
}

func BenchmarkServerNewWithGRPCOption(b *testing.B) {
	cfg := DefaultConfig()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		gwBenchCfg = cfg
		_ = New(gwBenchCfg, WithGRPCOption(grpc.MaxConcurrentStreams(100)))
	}
}

func BenchmarkLoggingInterceptor(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	interceptor := loggingInterceptor(logger)
	info := &grpc.UnaryServerInfo{FullMethod: "/app.v1.OrderService/GetByID"}
	handler := func(ctx context.Context, req any) (any, error) { return req, nil }
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		gwBenchAny, gwBenchErr = interceptor(ctx, "request", info, handler)
	}
}

func BenchmarkMetricsInterceptor(b *testing.B) {
	metrics := NewMetricsRegistry("bench")
	interceptor := metricsInterceptor(metrics)
	info := &grpc.UnaryServerInfo{FullMethod: "/app.v1.OrderService/Create"}
	handler := func(ctx context.Context, req any) (any, error) { return req, nil }
	ctx := context.Background()
	req := &gwRequest{name: "bench", rquid: "request-id"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		gwBenchAny, gwBenchErr = interceptor(ctx, req, info, handler)
	}
}

type gwRequest struct {
	name  string
	rquid string
}

func (r *gwRequest) GetName() string { return r.name }

func (r *gwRequest) GetRquid() string { return r.rquid }
