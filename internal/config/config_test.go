package config

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

// env builds a getenv function from a map, so tests never touch the real process
// environment.
func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr string // substring the error must contain, "" when none expected
	}{
		{
			name: "defaults",
			env:  map[string]string{},
			want: Config{
				Addr:      ":8080",
				Env:       EnvDevelopment,
				GraphPath: DefaultGraphPath,
			},
		},
		{
			name: "production with a port and graph path",
			env: map[string]string{
				"ENV":        EnvProduction,
				"PORT":       "9090",
				"GRAPH_PATH": "/srv/graph.json",
			},
			want: Config{
				Addr:      ":9090",
				Env:       EnvProduction,
				GraphPath: "/srv/graph.json",
			},
		},
		{
			name:    "port is not a number",
			env:     map[string]string{"PORT": "http"},
			wantErr: "PORT",
		},
		{
			name:    "port is zero",
			env:     map[string]string{"PORT": "0"},
			wantErr: "PORT",
		},
		{
			name:    "port is out of range",
			env:     map[string]string{"PORT": "70000"},
			wantErr: "PORT",
		},
		{
			name:    "unknown environment",
			env:     map[string]string{"ENV": "staging"},
			wantErr: "ENV",
		},
		{
			name: "database url with an explicit port",
			env:  map[string]string{"DATABASE_URL": "postgres://user:secret@db.example.com:6432/fleetflow"},
			want: Config{
				Addr:         ":8080",
				Env:          EnvDevelopment,
				GraphPath:    DefaultGraphPath,
				DatabaseDSN:  "postgres://user:secret@db.example.com:6432/fleetflow",
				DatabaseAddr: "db.example.com:6432",
			},
		},
		{
			name: "database url falls back to the default port",
			env:  map[string]string{"DATABASE_URL": "postgresql://user:secret@127.0.0.1/fleetflow"},
			want: Config{
				Addr:         ":8080",
				Env:          EnvDevelopment,
				GraphPath:    DefaultGraphPath,
				DatabaseDSN:  "postgresql://user:secret@127.0.0.1/fleetflow",
				DatabaseAddr: "127.0.0.1:5432",
			},
		},
		{
			name:    "database url with an unsupported scheme",
			env:     map[string]string{"DATABASE_URL": "mysql://user:secret@127.0.0.1/fleetflow"},
			wantErr: "DATABASE_URL",
		},
		{
			name:    "database url without a host",
			env:     map[string]string{"DATABASE_URL": "postgres:///fleetflow"},
			wantErr: "DATABASE_URL",
		},
		{
			name: "discrete postgres variables",
			env: map[string]string{
				"POSTGRES_HOST": "127.0.0.1",
				"POSTGRES_PORT": "55432",
			},
			want: Config{
				Addr:         ":8080",
				Env:          EnvDevelopment,
				GraphPath:    DefaultGraphPath,
				DatabaseAddr: "127.0.0.1:55432",
			},
		},
		{
			name: "discrete postgres variables default the port",
			env:  map[string]string{"POSTGRES_HOST": "127.0.0.1"},
			want: Config{
				Addr:         ":8080",
				Env:          EnvDevelopment,
				GraphPath:    DefaultGraphPath,
				DatabaseAddr: "127.0.0.1:5432",
			},
		},
		{
			name: "postgres port is not a number",
			env: map[string]string{
				"POSTGRES_HOST": "127.0.0.1",
				"POSTGRES_PORT": "five",
			},
			wantErr: "POSTGRES_PORT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Load(env(tt.env))

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Load() error = nil, want an error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadRequiresGetenv(t *testing.T) {
	if _, err := Load(nil); err == nil {
		t.Fatal("Load(nil) error = nil, want an error")
	}
}

func TestTCPReachable(t *testing.T) {
	// A live listener on an ephemeral port is reachable.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer lis.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := TCPReachable(lis.Addr().String())(ctx); err != nil {
		t.Errorf("TCPReachable(%s) = %v, want nil", lis.Addr(), err)
	}

	// Closing the listener frees the port, so the same address stops accepting
	// connections.
	addr := lis.Addr().String()
	if err := lis.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}
	if err := TCPReachable(addr)(ctx); err == nil {
		t.Errorf("TCPReachable(%s) = nil, want an error for a closed port", addr)
	}
}
