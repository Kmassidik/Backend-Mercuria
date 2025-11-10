package config

import (
	"os"
	"testing"
	"time"
)


func TestLoad(t *testing.T)  {
	tests := []struct{
		name string
		serviceName string
		envVars map[string]string
		wantErr bool
	}{
		{
			name:        "default configuration",
			serviceName: "test-service",
			envVars:     map[string]string{},
			wantErr:     false,
		},
		{
			name:        "custom configuration",
			serviceName: "auth",
			envVars: map[string]string{
				"PORT":     "9000",
				"DB_HOST":  "postgres-server",
				"DB_PORT":  "5433",
				"ENV":      "staging",
			},
			wantErr: false,
		},{
			name:        "production without JWT secret should fail",
			serviceName: "auth",
			envVars: map[string]string{
				"ENV": "production",
			},
			wantErr: true,
		},
		{
			name:        "production without JWT secret should fail",
			serviceName: "auth",
			envVars: map[string]string{
				"ENV": "production",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T){
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer func() {
				for key := range tt.envVars {
					os.Unsetenv(key)
				}
			}()

			cfg, err := Load(tt.serviceName)
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if cfg.Service.Name != tt.serviceName {
					t.Errorf("Expected service name %s, got %s", tt.serviceName, cfg.Service.Name)
				}
			}
		})
	}
}

func TestGetEnvAsDuration(t *testing.T) {
	os.Setenv("TEST_DURATION", "30s")
	defer os.Unsetenv("TEST_DURATION")

	duration := getEnvAsDuration("TEST_DURATION", 1*time.Minute)
	if duration != 30*time.Second {
		t.Errorf("Expected 30s, got %v", duration)
	}

	// Test default value
	duration = getEnvAsDuration("NON_EXISTENT", 2*time.Minute)
	if duration != 2*time.Minute {
		t.Errorf("Expected 2m, got %v", duration)
	}
}