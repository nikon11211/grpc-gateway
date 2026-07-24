package server

import (
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

type Option func(*options)

type options struct {
	tracerProvider trace.TracerProvider
	grpcOptions    []grpc.ServerOption
	echoMiddleware []echo.MiddlewareFunc
}

func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(o *options) {
		o.tracerProvider = tp
	}
}

func WithGRPCOption(opt grpc.ServerOption) Option {
	return func(o *options) {
		o.grpcOptions = append(o.grpcOptions, opt)
	}
}

func WithEchoMiddleware(mw echo.MiddlewareFunc) Option {
	return func(o *options) {
		o.echoMiddleware = append(o.echoMiddleware, mw)
	}
}
