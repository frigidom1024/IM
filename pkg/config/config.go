package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 全局配置
type Config struct {
	Server ServerConfig `yaml:"server"`
	MySQL  MySQLConfig  `yaml:"mysql"`
	Redis  RedisConfig  `yaml:"redis"`
	Etcd   EtcdConfig   `yaml:"etcd"`
	MinIO  MinIOConfig  `yaml:"minio"`
	JWT    JWTConfig    `yaml:"jwt"`
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
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// EtcdConfig etcd配置
type EtcdConfig struct {
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

var GlobalConfig *Config

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
