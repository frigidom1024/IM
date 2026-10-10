package config

import "testing"

func validTestConfig() *Config {
	return &Config{
		Server: ServerConfig{APIAddr: ":8080", ConnectAddr: ":7000"},
		MySQL:  MySQLConfig{Host: "localhost", Port: 3306, Database: "gochat"},
		MinIO:  MinIOConfig{Endpoint: "localhost:9000", Bucket: "gochat"},
		JWT:    JWTConfig{Secret: "test-secret", ExpireTime: 24},
		Auth:   AuthConfig{Mode: "builtin"},
	}
}

func TestConfigValidateAcceptsBuiltinConfig(t *testing.T) {
	if err := validTestConfig().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRequiresExternalUserAPI(t *testing.T) {
	cfg := validTestConfig()
	cfg.Auth.Mode = "external"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected external user API error")
	}
}

func TestConfigValidateRejectsInvalidAuthMode(t *testing.T) {
	cfg := validTestConfig()
	cfg.Auth.Mode = "unknown"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected invalid auth mode error")
	}
}
func TestConfigValidateRejectsDefaultSecretInProduction(t *testing.T) {
	cfg := validTestConfig()
	cfg.Environment = "production"
	cfg.JWT.Secret = "gochat-secret-key-change-in-production"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected default production secret error")
	}
}
func TestConfigValidateRequiresCORSInProduction(t *testing.T) {
	cfg := validTestConfig()
	cfg.Environment = "production"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected production CORS allowlist error")
	}
}
