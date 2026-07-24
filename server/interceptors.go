package server

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func loggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		start := time.Now()

		resp, err = handler(ctx, req)

		method := info.FullMethod

		if err == nil {
			logger.LogAttrs(ctx, slog.LevelInfo, "gRPC REQUEST",
				slog.String("method", method),
				slog.Duration("duration", time.Since(start)),
			)
		} else {
			logger.LogAttrs(ctx, slog.LevelError, "gRPC REQUEST_ERROR",
				slog.String("method", method),
				slog.Duration("duration", time.Since(start)),
				slog.String("error", err.Error()),
			)
		}
		return resp, err
	}
}

func metricsInterceptor(metrics *MetricsRegistry) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		startTime := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(startTime).Seconds()
		method := info.FullMethod

		metrics.RequestDuration.WithLabelValues(method).Observe(duration)

		if err != nil {
			errorCode := "unknown"
			if grpcErr, ok := status.FromError(err); ok {
				errorCode = grpcErr.Code().String()
			}
			metrics.ErrorCounter.WithLabelValues(method, errorCode).Inc()
		} else {
			metrics.SuccessCounter.WithLabelValues(method).Inc()
		}

		return resp, err
	}
}

func tracingInterceptor(cfg *Config) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		var requestID string

		if r, ok := req.(interface{ GetRquid() string }); ok {
			requestID = r.GetRquid()
		}

		resp, err := handler(ctx, req)

		span := trace.SpanFromContext(ctx)
		if span != nil {
			attrs := []attribute.KeyValue{
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.service", info.FullMethod),
				attribute.String("rpc.method", extractMethod(info.FullMethod)),
				attribute.String("net.protocol", "grpc"),
				attribute.String("grpc.server", cfg.Service),
				attribute.String("grpc.host", cfg.GrpcAddress),
			}

			if requestID != "" {
				attrs = append(attrs, attribute.String("x-request-id", requestID))
			}

			span.SetAttributes(attrs...)
		}

		return resp, err
	}
}

func extractMethod(fullMethod string) string {
	parts := strings.Split(fullMethod, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return fullMethod
}
