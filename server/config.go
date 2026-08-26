package server

import (
	"errors"
	"time"

	"github.com/go-playground/validator/v10"
)

type Config struct {
	Service             string          `mapstructure:"service" validate:"required"`
	HttpAddress         string          `mapstructure:"http_address" validate:"required"`
	GrpcAddress         string          `mapstructure:"grpc_address" validate:"required"`
	MetricsAddress      string          `mapstructure:"metrics_address" validate:"required"`
	IdleTimeout         time.Duration   `mapstructure:"idle_timeout" validate:"required"`
	ReadTimeout         time.Duration   `mapstructure:"read_timeout" validate:"required"`
	WriteTimeout        time.Duration   `mapstructure:"write_timeout" validate:"required"`
	WriteContextTimeout time.Duration   `mapstructure:"write_context_timeout" validate:"required"`
	ShutdownTimeout     time.Duration   `mapstructure:"shutdown_timeout"`
	KeepAlive           bool            `mapstructure:"keep_alive"`
	RateLimit           RateLimitConfig `mapstructure:"rate_limit" validate:"required"`
	MaxRecvMsgSize      int             `mapstructure:"max_recv_msg_size" validate:"required"`
	MaxConnectionIdle   time.Duration   `mapstructure:"max_connection_idle" validate:"required"`
	Timeout             time.Duration   `mapstructure:"timeout" validate:"required"`
	MaxConnectionAge    time.Duration   `mapstructure:"max_connection_age" validate:"required"`
	Time                time.Duration   `mapstructure:"time" validate:"required"`
}

type RateLimitConfig struct {
	Limit    float64       `mapstructure:"limit" validate:"required"`
	Burst    int           `mapstructure:"burst" validate:"required"`
	ExpireIn time.Duration `mapstructure:"expire_in" validate:"required"`
}

func DefaultConfig() *Config {
	return &Config{
		Service:             "default-service",
		HttpAddress:         ":8080",
		GrpcAddress:         ":9090",
		MetricsAddress:      ":9091",
		IdleTimeout:         120 * time.Second,
		ReadTimeout:         30 * time.Second,
		WriteTimeout:        30 * time.Second,
		WriteContextTimeout: 5 * time.Second,
		ShutdownTimeout:     10 * time.Second,
		KeepAlive:           true,
		RateLimit: RateLimitConfig{
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
}

func (c Config) Validate() error {
	if err := validator.New().Struct(c); err != nil {
		return err
	}
	if c.IdleTimeout <= 0 {
		return errors.New("idle timeout must be positive")
	}
	if c.ReadTimeout <= 0 {
		return errors.New("read timeout must be positive")
	}
	if c.WriteTimeout <= 0 {
		return errors.New("write timeout must be positive")
	}
	if c.ShutdownTimeout <= 0 {
		return errors.New("shutdown timeout must be positive")
	}
	if c.MaxRecvMsgSize <= 0 {
		return errors.New("max recv msg size must be positive")
	}
	if c.RateLimit.Limit <= 0 {
		return errors.New("rate limit must be positive")
	}
	if c.RateLimit.Burst <= 0 {
		return errors.New("rate limit burst must be positive")
	}
	return nil
}
