package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func testConfig(grpcAddr, httpAddr, metricsAddr string) *Config {
	cfg := DefaultConfig()
	cfg.Service = "test-e2e"
	cfg.GrpcAddress = grpcAddr
	cfg.HttpAddress = httpAddr
	cfg.MetricsAddress = metricsAddr
	return cfg
}

func newTestListeners(t *testing.T) (grpcLis, httpLis, metricsLis net.Listener) {
	t.Helper()
	var err error
	grpcLis, err = net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	httpLis, err = net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	metricsLis, err = net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		grpcLis.Close()
		httpLis.Close()
		metricsLis.Close()
	})
	return grpcLis, httpLis, metricsLis
}

func startTestServer(t *testing.T, srv *Server) <-chan error {
	t.Helper()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()
	return errCh
}

func waitReady(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become ready", addr)
}

func TestServerStartStopE2E(t *testing.T) {
	grpcLis, httpLis, metricsLis := newTestListeners(t)

	cfg := testConfig(grpcLis.Addr().String(), httpLis.Addr().String(), metricsLis.Addr().String())
	srv := New(cfg)
	srv.grpcListener = grpcLis
	srv.httpListener = httpLis
	srv.metricsListener = metricsLis

	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("svc-a", healthpb.HealthCheckResponse_SERVING)
	srv.RegisterGRPCServices(func(gs *grpc.Server) {
		healthpb.RegisterHealthServer(gs, healthSrv)
	})

	err := srv.RegisterHTTPGateway(context.Background(), func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) error {
		assert.Equal(t, cfg.GrpcAddress, endpoint)
		assert.NotEmpty(t, opts)
		return mux.HandlePath("GET", "/v1/health", func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
	})
	require.NoError(t, err)

	srv.EchoServer.GET("/boom", func(c echo.Context) error {
		return errors.New("boom")
	})

	startErrCh := startTestServer(t, srv)
	httpAddr := "http://" + httpLis.Addr().String()
	waitReady(t, httpLis.Addr().String())
	waitReady(t, metricsLis.Addr().String())

	conn, err := grpc.NewClient(grpcLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	client := healthpb.NewHealthClient(conn)
	rpcCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Check(rpcCtx, &healthpb.HealthCheckRequest{Service: "svc-a"}, grpc.WaitForReady(true))
	require.NoError(t, err)
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.Status)

	_, err = client.Check(rpcCtx, &healthpb.HealthCheckRequest{Service: "missing"})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))

	getResp, err := http.Get(httpAddr + "/v1/health")
	require.NoError(t, err)
	body, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	getResp.Body.Close()
	assert.Equal(t, "ok", string(body))

	req, err := http.NewRequest(http.MethodGet, httpAddr+"/v1/health", nil)
	require.NoError(t, err)
	req.Header.Set(echo.HeaderXRequestID, "rid-123")
	reqResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	reqResp.Body.Close()
	assert.Equal(t, http.StatusOK, reqResp.StatusCode)

	boomResp, err := http.Get(httpAddr + "/boom")
	require.NoError(t, err)
	boomResp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, boomResp.StatusCode)

	metricsResp, err := http.Get("http://" + metricsLis.Addr().String() + "/metrics")
	require.NoError(t, err)
	metricsBody, err := io.ReadAll(metricsResp.Body)
	require.NoError(t, err)
	metricsResp.Body.Close()
	assert.Contains(t, string(metricsBody), "test_e2e_grpc_requests_success_total")
	assert.Contains(t, string(metricsBody), "test_e2e_grpc_requests_error_total")

	assert.NotNil(t, srv.GetMetricsRegistry())
	assert.Equal(t, srv.metrics, srv.GetMetricsRegistry())

	require.NoError(t, conn.Close())
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	require.NoError(t, srv.Stop(stopCtx))

	metricsLis.Close()
	err = <-startErrCh
	assert.ErrorIs(t, err, http.ErrServerClosed)
}

func TestServerStartBindErrors(t *testing.T) {
	t.Run("grpc port in use", func(t *testing.T) {
		occupied, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer occupied.Close()

		cfg := testConfig(occupied.Addr().String(), ":0", ":0")
		srv := New(cfg)
		err = srv.Start()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to listen on gRPC port")
	})

	t.Run("http port in use", func(t *testing.T) {
		occupied, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer occupied.Close()

		cfg := testConfig("127.0.0.1:0", occupied.Addr().String(), ":0")
		srv := New(cfg)
		err = srv.Start()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to listen on HTTP port")
	})

	t.Run("metrics port in use", func(t *testing.T) {
		occupied, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer occupied.Close()

		cfg := testConfig("127.0.0.1:0", "127.0.0.1:0", occupied.Addr().String())
		srv := New(cfg)
		err = srv.Start()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to listen on metrics port")
	})
}

func TestStopShutdownError(t *testing.T) {
	grpcLis, httpLis, metricsLis := newTestListeners(t)

	cfg := testConfig(grpcLis.Addr().String(), httpLis.Addr().String(), metricsLis.Addr().String())
	srv := New(cfg)
	srv.grpcListener = grpcLis
	srv.httpListener = httpLis
	srv.metricsListener = metricsLis

	startErrCh := startTestServer(t, srv)
	waitReady(t, httpLis.Addr().String())

	conn, err := net.Dial("tcp", httpLis.Addr().String())
	require.NoError(t, err)
	_, err = conn.Write([]byte("GET / HTTP/1.1\r\nHost: localhost\r\n"))
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err = srv.Stop(canceledCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error shutting down HTTP server")

	require.NoError(t, conn.Close())
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupCancel()
	require.NoError(t, srv.EchoServer.Shutdown(cleanupCtx))
	metricsLis.Close()
	<-startErrCh
}

func TestStartMetricsListenerClosed(t *testing.T) {
	grpcLis, httpLis, metricsLis := newTestListeners(t)
	require.NoError(t, metricsLis.Close())

	cfg := testConfig(grpcLis.Addr().String(), httpLis.Addr().String(), metricsLis.Addr().String())
	srv := New(cfg)
	srv.grpcListener = grpcLis
	srv.httpListener = httpLis
	srv.metricsListener = metricsLis

	startErrCh := startTestServer(t, srv)
	waitReady(t, httpLis.Addr().String())

	resp, err := http.Get("http://" + httpLis.Addr().String() + "/boom")
	require.NoError(t, err)
	resp.Body.Close()

	require.NoError(t, grpcLis.Close())

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	require.NoError(t, srv.Stop(stopCtx))
	err = <-startErrCh
	assert.ErrorIs(t, err, http.ErrServerClosed)
}

func TestStartBindsListeners(t *testing.T) {
	cfg := testConfig("127.0.0.1:0", "127.0.0.1:0", "127.0.0.1:0")
	srv := New(cfg)

	require.NoError(t, srv.bindListeners())
	require.NotNil(t, srv.grpcListener)
	require.NotNil(t, srv.httpListener)
	require.NotNil(t, srv.metricsListener)
	assert.Contains(t, srv.grpcListener.Addr().String(), "127.0.0.1:")
	assert.Contains(t, srv.httpListener.Addr().String(), "127.0.0.1:")
	assert.Contains(t, srv.metricsListener.Addr().String(), "127.0.0.1:")

	startErrCh := startTestServer(t, srv)
	waitReady(t, srv.httpListener.Addr().String())

	resp, err := http.Get("http://" + srv.httpListener.Addr().String() + "/boom")
	require.NoError(t, err)
	resp.Body.Close()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	require.NoError(t, srv.Stop(stopCtx))
	err = <-startErrCh
	assert.ErrorIs(t, err, http.ErrServerClosed)
}

func TestStopShutsDownMetricsServer(t *testing.T) {
	grpcLis, httpLis, metricsLis := newTestListeners(t)

	cfg := testConfig(grpcLis.Addr().String(), httpLis.Addr().String(), metricsLis.Addr().String())
	srv := New(cfg)
	srv.grpcListener = grpcLis
	srv.httpListener = httpLis
	srv.metricsListener = metricsLis

	startErrCh := startTestServer(t, srv)
	waitReady(t, httpLis.Addr().String())

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	require.NoError(t, srv.Stop(stopCtx))
	<-startErrCh

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", metricsLis.Addr().String(), 200*time.Millisecond)
		if err != nil {
			return true
		}
		conn.Close()
		return false
	}, 2*time.Second, 50*time.Millisecond)
}

func TestRateLimiting(t *testing.T) {
	grpcLis, httpLis, metricsLis := newTestListeners(t)

	cfg := testConfig(grpcLis.Addr().String(), httpLis.Addr().String(), metricsLis.Addr().String())
	cfg.RateLimit = RateLimitConfig{Limit: 1, Burst: 1, ExpireIn: time.Minute}
	srv := New(cfg)
	srv.grpcListener = grpcLis
	srv.httpListener = httpLis
	srv.metricsListener = metricsLis

	srv.EchoServer.GET("/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, "pong")
	})

	startErrCh := startTestServer(t, srv)
	httpAddr := "http://" + httpLis.Addr().String()
	waitReady(t, httpLis.Addr().String())

	first, err := http.Get(httpAddr + "/ping")
	require.NoError(t, err)
	first.Body.Close()
	assert.Equal(t, http.StatusOK, first.StatusCode)

	for i := 0; i < 3; i++ {
		limited, err := http.Get(httpAddr + "/ping")
		require.NoError(t, err)
		limited.Body.Close()
		assert.Equal(t, http.StatusTooManyRequests, limited.StatusCode)
	}

	time.Sleep(1100 * time.Millisecond)
	after, err := http.Get(httpAddr + "/ping")
	require.NoError(t, err)
	after.Body.Close()
	assert.Equal(t, http.StatusOK, after.StatusCode)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	require.NoError(t, srv.Stop(stopCtx))
	metricsLis.Close()
	<-startErrCh
}

func TestRateLimiterHandlers(t *testing.T) {
	srv := New(testConfig("127.0.0.1:0", "127.0.0.1:0", "127.0.0.1:0"))
	e := echo.New()

	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)

	identifier, err := srv.rateLimitConfig.IdentifierExtractor(c)
	require.NoError(t, err)
	assert.NotEmpty(t, identifier)

	err = srv.rateLimitConfig.ErrorHandler(c, errors.New("extract failed"))
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	denyRec := httptest.NewRecorder()
	denyCtx := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), denyRec)
	err = srv.rateLimitConfig.DenyHandler(denyCtx, "127.0.0.1", errors.New("rate limit exceeded"))
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, denyRec.Code)
	assert.Contains(t, denyRec.Body.String(), "rate limit exceeded")
}

func TestRegisterHTTPGateway(t *testing.T) {
	t.Run("success with metadata", func(t *testing.T) {
		cfg := testConfig(":9290", ":9291", ":9292")
		srv := New(cfg)

		err := srv.RegisterHTTPGateway(context.Background(), func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) error {
			assert.Equal(t, ":9290", endpoint)
			assert.Len(t, opts, 1)

			annReq := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
			annReq.Header.Set(echo.HeaderXRequestID, "rid-meta")
			annCtx, err := runtime.AnnotateContext(ctx, mux, annReq, "/test.Service/Method")
			require.NoError(t, err)
			md, ok := metadata.FromOutgoingContext(annCtx)
			require.True(t, ok)
			assert.Equal(t, []string{"rid-meta"}, md.Get("x-request-id"))

			annReq = httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
			annCtx, err = runtime.AnnotateContext(ctx, mux, annReq, "/test.Service/Method")
			require.NoError(t, err)
			md, ok = metadata.FromOutgoingContext(annCtx)
			require.True(t, ok)
			assert.Empty(t, md.Get("x-request-id"))

			return mux.HandlePath("GET", "/v1/ping", func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("pong"))
			})
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
		req.Header.Set(echo.HeaderXRequestID, "req-1")
		rec := httptest.NewRecorder()
		srv.EchoServer.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "pong", rec.Body.String())

		req = httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
		rec = httptest.NewRecorder()
		srv.EchoServer.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("registration error", func(t *testing.T) {
		cfg := testConfig(":9293", ":9294", ":9295")
		srv := New(cfg)

		err := srv.RegisterHTTPGateway(context.Background(), func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) error {
			return errors.New("registration failed")
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to register HTTP gateway")
	})
}

func TestNewWithTracerProviderAndMiddleware(t *testing.T) {
	cfg := testConfig(":9296", ":9297", ":9298")
	middlewareRan := false
	mw := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			middlewareRan = true
			return next(c)
		}
	}

	srv := New(cfg, WithTracerProvider(noop.NewTracerProvider()), WithEchoMiddleware(mw))
	require.NotNil(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.EchoServer.ServeHTTP(rec, req)
	assert.True(t, middlewareRan)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRequestLoggerErrorPath(t *testing.T) {
	cfg := testConfig(":9299", ":9300", ":9301")
	srv := New(cfg)
	srv.EchoServer.GET("/fail", func(c echo.Context) error {
		return fmt.Errorf("handler failed")
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rec := httptest.NewRecorder()
	srv.EchoServer.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
