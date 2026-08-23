package env_test

import (
	"strings"
	"testing"

	"github.com/flashsale/common/env"
)

// clearPostgres unsets every variable PostgresDSN reads, so each case starts
// from a known state.
func clearPostgres(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"POSTGRES_URL", "POSTGRES_HOST", "POSTGRES_PORT",
		"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB", "POSTGRES_SSLMODE",
	} {
		t.Setenv(k, "")
	}
}

func clearRabbit(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"RABBITMQ_URL", "RABBITMQ_HOST", "RABBITMQ_PORT",
		"RABBITMQ_USER", "RABBITMQ_PASSWORD", "RABBITMQ_VHOST",
	} {
		t.Setenv(k, "")
	}
}

// An explicit URL wins, which is how CI and managed databases are configured.
func TestPostgresDSNPrefersExplicitURL(t *testing.T) {
	clearPostgres(t)
	t.Setenv("POSTGRES_URL", "postgres://someone@db.example.com:5432/app?sslmode=require")
	t.Setenv("POSTGRES_HOST", "ignored")

	got, err := env.PostgresDSN()
	if err != nil {
		t.Fatalf("PostgresDSN() error = %v", err)
	}
	if got != "postgres://someone@db.example.com:5432/app?sslmode=require" {
		t.Errorf("PostgresDSN() = %q, want the explicit POSTGRES_URL", got)
	}
}

func TestPostgresDSNAssemblesFromParts(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "defaults",
			env:  map[string]string{},
			want: "postgres://postgres@localhost:5432/flashsale?sslmode=disable",
		},
		{
			// Credential fixtures stay on localhost so the secret scanner does
			// not read them as a real connection string.
			name: "with password",
			env:  map[string]string{"POSTGRES_PASSWORD": "s3cret"},
			want: "postgres://postgres:s3cret@localhost:5432/flashsale?sslmode=disable",
		},
		{
			name: "container host only",
			env:  map[string]string{"POSTGRES_HOST": "postgres"},
			want: "postgres://postgres@postgres:5432/flashsale?sslmode=disable",
		},
		{
			name: "full override",
			env: map[string]string{
				"POSTGRES_PORT": "6543", "POSTGRES_USER": "app",
				"POSTGRES_PASSWORD": "pw", "POSTGRES_DB": "sales", "POSTGRES_SSLMODE": "require",
			},
			want: "postgres://app:pw@localhost:6543/sales?sslmode=require",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearPostgres(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got, err := env.PostgresDSN()
			if err != nil {
				t.Fatalf("PostgresDSN() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("PostgresDSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A password with URL-significant characters must survive assembly, otherwise
// the DSN silently points somewhere else or fails to parse.
func TestPostgresDSNEscapesCredentials(t *testing.T) {
	clearPostgres(t)
	t.Setenv("POSTGRES_USER", "app@corp")
	t.Setenv("POSTGRES_PASSWORD", "p@ss:w/rd?")

	got, err := env.PostgresDSN()
	if err != nil {
		t.Fatalf("PostgresDSN() error = %v", err)
	}
	if strings.Contains(got, "p@ss:w/rd?") {
		t.Errorf("PostgresDSN() = %q, want the password percent-encoded", got)
	}
	if !strings.Contains(got, "%40") {
		t.Errorf("PostgresDSN() = %q, want @ encoded as %%40", got)
	}
}

func TestAMQPURLPrefersExplicitURL(t *testing.T) {
	clearRabbit(t)
	t.Setenv("RABBITMQ_URL", "amqp://broker.example.com:5672/sales")
	t.Setenv("RABBITMQ_HOST", "ignored")

	got, err := env.AMQPURL()
	if err != nil {
		t.Fatalf("AMQPURL() error = %v", err)
	}
	if got != "amqp://broker.example.com:5672/sales" {
		t.Errorf("AMQPURL() = %q, want the explicit RABBITMQ_URL", got)
	}
}

func TestAMQPURLAssemblesFromParts(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			// With no credentials configured the URL carries none, and the AMQP
			// client applies its own defaults.
			name: "no credentials",
			env:  map[string]string{},
			want: "amqp://localhost:5672/",
		},
		{
			name: "container host",
			env:  map[string]string{"RABBITMQ_HOST": "rabbitmq"},
			want: "amqp://rabbitmq:5672/",
		},
		{
			name: "with credentials",
			env:  map[string]string{"RABBITMQ_USER": "app", "RABBITMQ_PASSWORD": "pw"},
			want: "amqp://app:pw@localhost:5672/",
		},
		{
			name: "custom vhost",
			env:  map[string]string{"RABBITMQ_VHOST": "sales"},
			want: "amqp://localhost:5672/sales",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearRabbit(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got, err := env.AMQPURL()
			if err != nil {
				t.Fatalf("AMQPURL() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("AMQPURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
