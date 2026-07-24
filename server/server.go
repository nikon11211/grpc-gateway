package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/propagation"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

type Router struct {
	name string
	*echo.Group
}

type Server struct {
	grpcPort        string
	httpPort        string
	metricsAddress  string
	GRPCServer      *grpc.Server
	EchoServer      *echo.Echo
	config          *Config
	groups          map[string]*Router
	shutdownTimeout time.Duration
	contextTimeout  time.Duration
	rateLimitConfig middleware.RateLimiterConfig
	metrics         *MetricsRegistry
	*slog.Logger
}

func New(cfg *Config, opts ...Option) *Server {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	options := &options{}
	for _, opt := range opts {
		opt(options)
	}

	metrics := NewMetricsRegistry(cfg.Service)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	e := echo.New()
	e.HideBanner = true
	e.Server.Addr = cfg.HttpAddress
	e.Server.ReadHeaderTimeout = cfg.ReadTimeout
	e.Server.ReadTimeout = cfg.ReadTimeout
	e.Server.WriteTimeout = cfg.WriteTimeout
	e.Server.IdleTimeout = cfg.IdleTimeout

	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowCredentials: true,
		MaxAge:           86400,
	}))
	e.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{
		Generator: func() string {
			return uuid.NewString()
		},
	}))

	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:    true,
		LogURI:       true,
		LogRequestID: true,
		LogMethod:    true,
		LogError:     true,
		HandleError:  true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			if v.Error == nil {
				logger.LogAttrs(context.Background(), slog.LevelInfo, "REQUEST",
					slog.String("request_id", v.RequestID),
					slog.String("method", v.Method),
					slog.String("uri", v.URI),
					slog.Int("status", v.Status),
				)
			} else {
				logger.LogAttrs(context.Background(), slog.LevelError, "REQUEST_ERROR",
					slog.String("request_id", v.RequestID),
					slog.String("method", v.Method),
					slog.String("uri", v.URI),
					slog.Int("status", v.Status),
					slog.String("error", v.Error.Error()),
				)
			}
			return nil
		},
	}))

	rateLimitConfig := middleware.RateLimiterConfig{
		Skipper: middleware.DefaultSkipper,
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(
			middleware.RateLimiterMemoryStoreConfig{
				Rate:      rate.Limit(cfg.RateLimit.Limit),
				Burst:     cfg.RateLimit.Burst,
				ExpiresIn: cfg.RateLimit.ExpireIn,
			},
		),
		IdentifierExtractor: func(ctx echo.Context) (string, error) {
			return ctx.RealIP(), nil
		},
		ErrorHandler: func(ctx echo.Context, err error) error {
			return ctx.JSON(http.StatusForbidden, nil)
		},
		DenyHandler: func(ctx echo.Context, identifier string, err error) error {
			return ctx.JSON(http.StatusTooManyRequests, map[string]string{
				"error": "rate limit exceeded",
			})
		},
	}
	e.Use(middleware.RateLimiterWithConfig(rateLimitConfig))

	e.Use(echoprometheus.NewMiddlewareWithConfig(echoprometheus.MiddlewareConfig{
		Namespace:  cfg.Service,
		Registerer: metrics.GetRegistry(),
	}))

	e.Use(middleware.ContextTimeoutWithConfig(middleware.ContextTimeoutConfig{
		Timeout: cfg.WriteContextTimeout,
	}))

	e.Server.SetKeepAlivesEnabled(cfg.KeepAlive)

	if options.tracerProvider != nil {
		e.Use(otelecho.Middleware(cfg.Service,
			otelecho.WithTracerProvider(options.tracerProvider),
			otelecho.WithPropagators(propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{})),
		))
		logger.LogAttrs(context.Background(), slog.LevelInfo, "OpenTelemetry tracing enabled for HTTP server")
	}

	for _, mw := range options.echoMiddleware {
		e.Use(mw)
	}

	grpcOpts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: cfg.MaxConnectionIdle,
			Timeout:           cfg.Timeout,
			MaxConnectionAge:  cfg.MaxConnectionAge,
			Time:              cfg.Time,
		}),
		grpc.MaxRecvMsgSize(cfg.MaxRecvMsgSize),
	}

	interceptors := []grpc.UnaryServerInterceptor{
		loggingInterceptor(logger),
		metricsInterceptor(metrics),
	}

	if options.tracerProvider != nil {
		grpcOpts = append(grpcOpts,
			grpc.StatsHandler(otelgrpc.NewServerHandler(
				otelgrpc.WithTracerProvider(options.tracerProvider),
				otelgrpc.WithPropagators(propagation.TraceContext{}),
			)),
		)
		interceptors = append(interceptors, tracingInterceptor(cfg))
		logger.LogAttrs(context.Background(), slog.LevelInfo, "OpenTelemetry tracing enabled for gRPC server")
	}

	grpcOpts = append(grpcOpts, grpc.ChainUnaryInterceptor(interceptors...))
	grpcOpts = append(grpcOpts, options.grpcOptions...)

	grpcServer := grpc.NewServer(grpcOpts...)

	return &Server{
		grpcPort:        cfg.GrpcAddress,
		httpPort:        cfg.HttpAddress,
		metricsAddress:  cfg.MetricsAddress,
		GRPCServer:      grpcServer,
		EchoServer:      e,
		config:          cfg,
		groups:          make(map[string]*Router),
		shutdownTimeout: cfg.ShutdownTimeout,
		contextTimeout:  cfg.WriteContextTimeout,
		rateLimitConfig: rateLimitConfig,
		metrics:         metrics,
		Logger:          logger,
	}
}

func (s *Server) RegisterGRPCServices(registerFunc func(*grpc.Server)) {
	if s == nil {
		return
	}
	registerFunc(s.GRPCServer)
}

func (s *Server) RegisterHTTPGateway(ctx context.Context, registerFunc func(context.Context, *runtime.ServeMux, string, []grpc.DialOption) error) error {
	if s == nil {
		return fmt.Errorf("server is nil")
	}

	mux := runtime.NewServeMux(
		runtime.WithMetadata(func(ctx context.Context, req *http.Request) metadata.MD {
			md := metadata.MD{}
			if requestID := req.Header.Get(echo.HeaderXRequestID); requestID != "" {
				md.Set("x-request-id", requestID)
			}
			return md
		}),
	)

	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if err := registerFunc(ctx, mux, s.grpcPort, opts); err != nil {
		return fmt.Errorf("failed to register HTTP gateway: %w", err)
	}

	s.EchoServer.Any("/*", echo.WrapHandler(mux))
	return nil
}

func (s *Server) Start() error {
	if s == nil {
		return fmt.Errorf("server is nil")
	}

	lis, err := net.Listen("tcp", s.grpcPort)
	if err != nil {
		return fmt.Errorf("failed to listen on gRPC port: %w", err)
	}

	go func() {
		s.LogAttrs(context.Background(), slog.LevelDebug, fmt.Sprintf("gRPC server starting on %s", s.grpcPort))
		if err := s.GRPCServer.Serve(lis); err != nil {
			s.LogAttrs(context.Background(), slog.LevelDebug, fmt.Sprintf("gRPC server error: %v", err))
		}
	}()

	go func() {
		metricsEcho := echo.New()
		metricsEcho.HideBanner = true
		metricsEcho.GET("/metrics", echoprometheus.NewHandlerWithConfig(
			echoprometheus.HandlerConfig{
				Gatherer: s.metrics.GetRegistry(),
			},
		))
		s.LogAttrs(context.Background(), slog.LevelDebug, fmt.Sprintf("Metrics server starting on %s", s.metricsAddress))

		if err := metricsEcho.Start(s.metricsAddress); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.LogAttrs(context.Background(), slog.LevelDebug, fmt.Sprintf("Metrics server error: %v", err))
		}
	}()

	s.LogAttrs(context.Background(), slog.LevelDebug, fmt.Sprintf("HTTP server starting on %s", s.httpPort))
	return s.EchoServer.Start(s.httpPort)
}

func (s *Server) GetMetricsRegistry() *MetricsRegistry {
	return s.metrics
}

func (s *Server) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}

	s.LogAttrs(context.Background(), slog.LevelDebug, "Shutting down servers...")

	s.GRPCServer.GracefulStop()

	if err := s.EchoServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("error shutting down HTTP server: %w", err)
	}

	return nil
}

func (s *Server) AddRouter(name string) *Router {
	if s == nil {
		return nil
	}
	group := &Router{name: name, Group: s.EchoServer.Group(name)}
	s.groups[name] = group
	return group
}

func (s *Server) GetConfig() *Config {
	if s == nil {
		return nil
	}
	return s.config
}
