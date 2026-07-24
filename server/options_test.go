package server

import (
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestWithTracerProvider(t *testing.T) {
	opts := &options{}
	tp := noop.NewTracerProvider()

	opt := WithTracerProvider(tp)
	opt(opts)

	assert.NotNil(t, opts.tracerProvider)
	assert.Equal(t, tp, opts.tracerProvider)
}

func TestWithGRPCOption(t *testing.T) {
	opts := &options{}

	grpcOpt := grpc.Creds(insecure.NewCredentials())
	opt := WithGRPCOption(grpcOpt)
	opt(opts)

	assert.Len(t, opts.grpcOptions, 1)
}

func TestWithEchoMiddleware(t *testing.T) {
	opts := &options{}

	mw := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return next(c)
		}
	}

	opt := WithEchoMiddleware(mw)
	opt(opts)

	assert.Len(t, opts.echoMiddleware, 1)
}

func TestMultipleOptions(t *testing.T) {
	opts := &options{}
	tp := noop.NewTracerProvider()

	WithTracerProvider(tp)(opts)
	WithGRPCOption(grpc.Creds(insecure.NewCredentials()))(opts)
	WithEchoMiddleware(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return next(c)
		}
	})(opts)

	assert.NotNil(t, opts.tracerProvider)
	assert.Len(t, opts.grpcOptions, 1)
	assert.Len(t, opts.echoMiddleware, 1)
}
