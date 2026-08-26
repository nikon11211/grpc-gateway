package server

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestLoggingInterceptor(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	interceptor := loggingInterceptor(logger)

	handler := func(ctx context.Context, req any) (any, error) {
		return "response", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	resp, err := interceptor(context.Background(), "request", info, handler)
	assert.NoError(t, err)
	assert.Equal(t, "response", resp)
}

func TestLoggingInterceptorError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	interceptor := loggingInterceptor(logger)

	handler := func(ctx context.Context, req any) (any, error) {
		return nil, status.Error(codes.Internal, "test error")
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	resp, err := interceptor(context.Background(), "request", info, handler)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestMetricsInterceptorSuccess(t *testing.T) {
	metrics := NewMetricsRegistry("test_service1")

	interceptor := metricsInterceptor(metrics)

	handler := func(ctx context.Context, req any) (any, error) {
		return "response", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Success"}

	resp, err := interceptor(context.Background(), "request", info, handler)
	assert.NoError(t, err)
	assert.Equal(t, "response", resp)
}

func TestMetricsInterceptorError(t *testing.T) {
	metrics := NewMetricsRegistry("test_service2")

	interceptor := metricsInterceptor(metrics)

	handler := func(ctx context.Context, req any) (any, error) {
		return nil, status.Error(codes.NotFound, "not found")
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Error"}

	resp, err := interceptor(context.Background(), "request", info, handler)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestMetricsInterceptorUnknownError(t *testing.T) {
	metrics := NewMetricsRegistry("test_service3")

	interceptor := metricsInterceptor(metrics)

	handler := func(ctx context.Context, req any) (any, error) {
		return nil, assert.AnError
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/UnknownError"}

	resp, err := interceptor(context.Background(), "request", info, handler)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestTracingInterceptor(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MetricsAddress = ":9115"

	interceptor := tracingInterceptor(cfg)

	handler := func(ctx context.Context, req any) (any, error) {
		return "response", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Traced"}

	resp, err := interceptor(context.Background(), "request", info, handler)
	assert.NoError(t, err)
	assert.Equal(t, "response", resp)
}

type testRequestWithID struct {
	requestID string
}

func (r *testRequestWithID) GetRquid() string {
	return r.requestID
}

func TestTracingInterceptorWithSpan(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MetricsAddress = ":9118"

	interceptor := tracingInterceptor(cfg)
	handler := func(ctx context.Context, req any) (any, error) {
		return "response", nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Traced"}

	tp := noop.NewTracerProvider()
	tracer := tp.Tracer("test")

	t.Run("span without request id", func(t *testing.T) {
		_, span := tracer.Start(context.Background(), "op")
		defer span.End()

		resp, err := interceptor(trace.ContextWithSpan(context.Background(), span), "request", info, handler)
		assert.NoError(t, err)
		assert.Equal(t, "response", resp)
	})

	t.Run("span with request id", func(t *testing.T) {
		_, span := tracer.Start(context.Background(), "op")
		defer span.End()

		req := &testRequestWithID{requestID: "req-42"}
		resp, err := interceptor(trace.ContextWithSpan(context.Background(), span), req, info, handler)
		assert.NoError(t, err)
		assert.Equal(t, "response", resp)
	})
}
