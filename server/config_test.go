package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MetricsAddress = ":9102"

	assert.Equal(t, "default-service", cfg.Service)
	assert.Equal(t, ":8080", cfg.HttpAddress)
	assert.Equal(t, ":9090", cfg.GrpcAddress)
	assert.Equal(t, ":9102", cfg.MetricsAddress)
	assert.Equal(t, 120*time.Second, cfg.IdleTimeout)
	assert.Equal(t, 30*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.WriteTimeout)
	assert.Equal(t, 5*time.Second, cfg.WriteContextTimeout)
	assert.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
	assert.True(t, cfg.KeepAlive)
	assert.Equal(t, float64(100), cfg.RateLimit.Limit)
	assert.Equal(t, 20, cfg.RateLimit.Burst)
	assert.Equal(t, 60*time.Second, cfg.RateLimit.ExpireIn)
	assert.Equal(t, 4*1024*1024, cfg.MaxRecvMsgSize)
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			cfg: func() *Config {
				c := DefaultConfig()
				c.MetricsAddress = ":9104"
				return c
			}(),
			wantErr: false,
		},
		{
			name: "missing service",
			cfg: func() *Config {
				c := DefaultConfig()
				c.Service = ""
				c.MetricsAddress = ":9105"
				return c
			}(),
			wantErr: true,
			errMsg:  "Key: 'Config.Service' Error:Field validation for 'Service' failed on the 'required' tag",
		},
		{
			name: "missing http address",
			cfg: func() *Config {
				c := DefaultConfig()
				c.HttpAddress = ""
				c.MetricsAddress = ":9106"
				return c
			}(),
			wantErr: true,
			errMsg:  "Key: 'Config.HttpAddress' Error:Field validation for 'HttpAddress' failed on the 'required' tag",
		},
		{
			name: "missing grpc address",
			cfg: func() *Config {
				c := DefaultConfig()
				c.GrpcAddress = ""
				c.MetricsAddress = ":9107"
				return c
			}(),
			wantErr: true,
			errMsg:  "Key: 'Config.GrpcAddress' Error:Field validation for 'GrpcAddress' failed on the 'required' tag",
		},
		{
			name: "missing metrics address",
			cfg: func() *Config {
				c := DefaultConfig()
				c.MetricsAddress = ""
				return c
			}(),
			wantErr: true,
			errMsg:  "Key: 'Config.MetricsAddress' Error:Field validation for 'MetricsAddress' failed on the 'required' tag",
		},
		{
			name: "negative idle timeout",
			cfg: func() *Config {
				c := DefaultConfig()
				c.IdleTimeout = -1
				c.MetricsAddress = ":9109"
				return c
			}(),
			wantErr: true,
			errMsg:  "idle timeout must be positive",
		},
		{
			name: "negative read timeout",
			cfg: func() *Config {
				c := DefaultConfig()
				c.ReadTimeout = -1
				c.MetricsAddress = ":9110"
				return c
			}(),
			wantErr: true,
			errMsg:  "read timeout must be positive",
		},
		{
			name: "negative write timeout",
			cfg: func() *Config {
				c := DefaultConfig()
				c.WriteTimeout = -1
				c.MetricsAddress = ":9111"
				return c
			}(),
			wantErr: true,
			errMsg:  "write timeout must be positive",
		},
		{
			name: "negative shutdown timeout",
			cfg: func() *Config {
				c := DefaultConfig()
				c.ShutdownTimeout = -1
				c.MetricsAddress = ":9112"
				return c
			}(),
			wantErr: true,
			errMsg:  "shutdown timeout must be positive",
		},
		{
			name: "negative max recv msg size",
			cfg: func() *Config {
				c := DefaultConfig()
				c.MaxRecvMsgSize = -1
				c.MetricsAddress = ":9113"
				return c
			}(),
			wantErr: true,
			errMsg:  "max recv msg size must be positive",
		},
		{
			name: "zero rate limit",
			cfg: func() *Config {
				c := DefaultConfig()
				c.RateLimit.Limit = 0
				c.MetricsAddress = ":9114"
				return c
			}(),
			wantErr: true,
			errMsg:  "Key: 'Config.RateLimit.Limit' Error:Field validation for 'Limit' failed on the 'required' tag",
		},
		{
			name: "zero rate limit burst",
			cfg: func() *Config {
				c := DefaultConfig()
				c.RateLimit.Burst = 0
				c.MetricsAddress = ":9115"
				return c
			}(),
			wantErr: true,
			errMsg:  "Key: 'Config.RateLimit.Burst' Error:Field validation for 'Burst' failed on the 'required' tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
