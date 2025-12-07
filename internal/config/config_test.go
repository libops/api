package config

import (
	"os"
	"testing"
)

// TestLoad tests the Load function to ensure it correctly loads configuration from environment variables and handles missing required values.
func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		setup   func()
		cleanup func()
		wantErr bool
	}{
		{
			name: "valid configuration",
			setup: func() {
				_ = os.Setenv("DATABASE_URL", "user:pass@tcp(localhost:3306)/dbname")
			},
			cleanup: func() {
				_ = os.Unsetenv("DATABASE_URL")
			},
			wantErr: false,
		},
		{
			name: "missing DATABASE_URL",
			setup: func() {
				_ = os.Unsetenv("DATABASE_URL")
			},
			cleanup: func() {},
			wantErr: true,
		},
		{
			name: "custom port",
			setup: func() {
				_ = os.Setenv("DATABASE_URL", "user:pass@tcp(localhost:3306)/dbname")
				_ = os.Setenv("PORT", "9090")
			},
			cleanup: func() {
				_ = os.Unsetenv("DATABASE_URL")
				_ = os.Unsetenv("PORT")
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			defer tt.cleanup()

			cfg, err := Load()
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if cfg == nil {
					t.Error("Load() returned nil config")
					return
				}
				if cfg.DatabaseURL == "" {
					t.Error("DatabaseURL should not be empty")
				}
			}
		})
	}
}

// TestValidate tests the Validate method of the Config struct.
func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				DatabaseURL: "user:pass@tcp(localhost:3306)/dbname",
				Port:        "8080",
			},
			wantErr: false,
		},
		{
			name: "missing database URL",
			config: &Config{
				Port: "8080",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestGetEnv tests the getEnv helper function for retrieving environment variables.
func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		want         string
	}{
		{
			name:         "environment variable set",
			key:          "TEST_VAR",
			defaultValue: "default",
			envValue:     "custom",
			want:         "custom",
		},
		{
			name:         "environment variable not set",
			key:          "TEST_VAR",
			defaultValue: "default",
			envValue:     "",
			want:         "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				_ = os.Setenv(tt.key, tt.envValue)
				defer func() { _ = os.Unsetenv(tt.key) }()
			}

			got := getEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}
