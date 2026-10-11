package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// CORSConfig 跨域配置
type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
}

type AuthConfig struct {
	Mode                   string `yaml:"mode"`              // builtin | external
	ExternalUserAPI        string `yaml:"external_user_api"` // 外部用户信息API
	RateLimitPerMinute     int    `yaml:"rate_limit_per_minute"`
	ExternalTimeoutSeconds int    `yaml:"external_timeout_seconds"`
}

// Config 全局配置
type Config struct {
	Environment string       `yaml:"environment"`
	CORS        CORSConfig   `yaml:"cors"`
	Server      ServerConfig `yaml:"server"`
	Auth        AuthConfig   `yaml:"auth"`
	MySQL       MySQLConfig  `yaml:"mysql"`
	Redis       RedisConfig  `yaml:"redis"`
	Etcd        EtcdConfig   `yaml:"etcd"`
	MinIO       MinIOConfig  `yaml:"minio"`
	JWT         JWTConfig    `yaml:"jwt"`
	Logger      LoggerConfig `yaml:"logger"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	APIAddr     string `yaml:"api_addr"`
	ConnectAddr string `yaml:"connect_addr"`
	LogicAddr   string `yaml:"logic_addr"`
	TaskAddr    string `yaml:"task_addr"`
}

// MySQLConfig MySQL配置
type MySQLConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

// RedisConfig Redis配置
type RedisConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// EtcdConfig etcd配置
type EtcdConfig struct {
	Enabled   bool     `yaml:"enabled"`
	Endpoints []string `yaml:"endpoints"`
}

// MinIOConfig MinIO配置
type MinIOConfig struct {
	Endpoint        string `yaml:"endpoint"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	Bucket          string `yaml:"bucket"`
	UseSSL          bool   `yaml:"use_ssl"`
}

// JWTConfig JWT配置
type JWTConfig struct {
	Secret     string `yaml:"secret"`
	ExpireTime int    `yaml:"expire_time"` // 小时
}

// LoggerConfig 日志配置
type LoggerConfig struct {
	Level  string `yaml:"level"`  // debug/info/warn/error
	Output string `yaml:"output"` // stdout/file
}

var GlobalConfig *Config

// Validate checks the configuration required by the single-instance runtime.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	if strings.TrimSpace(c.Server.APIAddr) == "" {
		return fmt.Errorf("server.api_addr is required")
	}
	if strings.TrimSpace(c.Server.ConnectAddr) == "" {
		return fmt.Errorf("server.connect_addr is required")
	}
	if strings.TrimSpace(c.MySQL.Host) == "" || c.MySQL.Port <= 0 || strings.TrimSpace(c.MySQL.Database) == "" {
		return fmt.Errorf("mysql host, port and database are required")
	}
	if strings.TrimSpace(c.MinIO.Endpoint) == "" || strings.TrimSpace(c.MinIO.Bucket) == "" {
		return fmt.Errorf("minio endpoint and bucket are required")
	}
	if strings.TrimSpace(c.JWT.Secret) == "" {
		return fmt.Errorf("jwt.secret is required")
	}
	if c.JWT.ExpireTime <= 0 {
		return fmt.Errorf("jwt.expire_time must be greater than zero")
	}

	if c.Redis.Enabled && (strings.TrimSpace(c.Redis.Host) == "" || c.Redis.Port <= 0) {
		return fmt.Errorf("redis host and port are required when enabled")
	}
	if c.Etcd.Enabled && len(c.Etcd.Endpoints) == 0 {
		return fmt.Errorf("etcd endpoints are required when enabled")
	}
	environment := strings.ToLower(strings.TrimSpace(c.Environment))
	if environment == "" {
		environment = strings.ToLower(strings.TrimSpace(os.Getenv("GOCHAT_ENV")))
	}
	if environment == "production" {
		if c.JWT.Secret == "gochat-secret-key-change-in-production" {
			return fmt.Errorf("jwt.secret must be changed in production")
		}
		if len(c.CORS.AllowedOrigins) == 0 {
			return fmt.Errorf("cors.allowed_origins is required in production")
		}
	}

	switch strings.ToLower(strings.TrimSpace(c.Auth.Mode)) {
	case "", "builtin":
		return nil
	case "external":
		if strings.TrimSpace(c.Auth.ExternalUserAPI) == "" {
			return fmt.Errorf("auth.external_user_api is required in external mode")
		}
		return nil
	default:
		return fmt.Errorf("auth.mode must be builtin or external")
	}
}

// Load 加载配置文件
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file failed: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config failed: %w", err)
	}

	GlobalConfig = &cfg
	return &cfg, nil
}

// MustLoad 加载配置，失败则panic
func MustLoad(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		panic(err)
	}
	return cfg
}
