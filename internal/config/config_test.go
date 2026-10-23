package config

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// env builds a getenv function from a map, so tests never touch the real process
// environment.
func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadSumoRuntimeFromHome(t *testing.T) {
	home := `C:\sumo`
	runtimeConfig, err := loadSumoRuntime(env(map[string]string{
		"SUMO_HOME":    home,
		"SUMO_NETWORK": `C:\data\jakarta.net.xml`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	extension := ""
	python := "python3"
	if runtime.GOOS == "windows" {
		extension = ".exe"
		python = "python"
	}
	if runtimeConfig.Binary != filepath.Join(home, "bin", "sumo"+extension) ||
		runtimeConfig.DuaRouter != filepath.Join(home, "bin", "duarouter"+extension) ||
		runtimeConfig.RandomTrips != filepath.Join(home, "tools", "randomTrips.py") ||
		runtimeConfig.Python != python {
		t.Fatalf("SUMO runtime = %#v", runtimeConfig)
	}
}

func TestLoadSumoRuntimeRequiresToolPaths(t *testing.T) {
	_, err := loadSumoRuntime(env(map[string]string{"SUMO_NETWORK": "jakarta.net.xml"}))
	if err == nil || !strings.Contains(err.Error(), "SUMO_BINARY") {
		t.Fatalf("loadSumoRuntime() error = %v, want missing binary", err)
	}
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
				Addr:    ":8080",
				Env:     EnvDevelopment,
				OSRMURL: DefaultOSRMURL,
			},
		},
		{
			name: "production with a port",
			env: map[string]string{
				"ENV":  EnvProduction,
				"PORT": "9090",
			},
			want: Config{
				Addr:    ":9090",
				Env:     EnvProduction,
				OSRMURL: DefaultOSRMURL,
			},
		},
		{
			name: "log level is set",
			env:  map[string]string{"LOG_LEVEL": "debug"},
			want: Config{
				Addr:     ":8080",
				Env:      EnvDevelopment,
				OSRMURL:  DefaultOSRMURL,
				LogLevel: LogDebug,
			},
		},
		{
			name: "log level is case-insensitive",
			env:  map[string]string{"LOG_LEVEL": "WARN"},
			want: Config{
				Addr:     ":8080",
				Env:      EnvDevelopment,
				OSRMURL:  DefaultOSRMURL,
				LogLevel: LogWarn,
			},
		},
		{
			name:    "log level is unknown",
			env:     map[string]string{"LOG_LEVEL": "verbose"},
			wantErr: "LOG_LEVEL",
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
			name: "OSRM URL is configurable",
			env:  map[string]string{"OSRM_URL": "http://router:5000"},
			want: Config{
				Addr:    ":8080",
				Env:     EnvDevelopment,
				OSRMURL: "http://router:5000",
			},
		},
		{
			name:    "OSRM URL rejects credentials",
			env:     map[string]string{"OSRM_URL": "http://user:pass@router:5000"},
			wantErr: "OSRM_URL",
		},
		{
			name: "database url with an explicit port",
			env:  map[string]string{"DATABASE_URL": "postgres://user:secret@db.example.com:6432/fleetflow"},
			want: Config{
				Addr:         ":8080",
				Env:          EnvDevelopment,
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
				DatabaseAddr: "127.0.0.1:55432",
			},
		},
		{
			name: "discrete postgres variables default the port",
			env:  map[string]string{"POSTGRES_HOST": "127.0.0.1"},
			want: Config{
				Addr:         ":8080",
				Env:          EnvDevelopment,
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

			// LogLevel is the one field every other case leaves alone, so it is
			// filled in rather than repeated in each expectation: a case that does
			// not name a level is asserting the default.
			want := tt.want
			if want.LogLevel == "" {
				want.LogLevel = DefaultLogLevel
			}
			if want.OSRMURL == "" {
				want.OSRMURL = DefaultOSRMURL
			}
			if got != want {
				t.Errorf("Load() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestLoadRequiresGetenv(t *testing.T) {
	if _, err := Load(nil); err == nil {
		t.Fatal("Load(nil) error = nil, want an error")
	}
}

// TestDiscreteDSN covers the path the local docker compose database takes: the
// POSTGRES_* variables give a host and port, and the credentials have to be
// folded in for the service to connect rather than merely probe.
func TestDiscreteDSN(t *testing.T) {
	env := env(map[string]string{
		"POSTGRES_HOST":     "127.0.0.1",
		"POSTGRES_PORT":     "55432",
		"POSTGRES_DATABASE": "fleetflow",
		"POSTGRES_USER":     "fleetflow",
		"POSTGRES_PASSWORD": "change-me",
	})

	cfg, err := Load(env)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	u, err := url.Parse(cfg.DatabaseDSN)
	if err != nil {
		t.Fatalf("DatabaseDSN = %q, which does not parse: %v", cfg.DatabaseDSN, err)
	}
	if u.Scheme != "postgres" {
		t.Errorf("scheme = %q, want postgres", u.Scheme)
	}
	if got := u.Host; got != "127.0.0.1:55432" {
		t.Errorf("host = %q, want 127.0.0.1:55432", got)
	}
	if got := u.Path; got != "/fleetflow" {
		t.Errorf("path = %q, want /fleetflow", got)
	}
	if got := u.User.Username(); got != "fleetflow" {
		t.Errorf("user = %q, want fleetflow", got)
	}
	if pw, _ := u.User.Password(); pw != "change-me" {
		t.Errorf("password did not survive the round trip")
	}
	if cfg.DatabaseAddr != "127.0.0.1:55432" {
		t.Errorf("DatabaseAddr = %q", cfg.DatabaseAddr)
	}
}

// TestDiscreteDSN_EscapesThePassword checks the DSN is built as a URL rather than
// by joining strings, so a password containing separators still works.
func TestDiscreteDSN_EscapesThePassword(t *testing.T) {
	const password = "p@ss:w/rd#1"

	cfg, err := Load(env(map[string]string{
		"POSTGRES_HOST":     "127.0.0.1",
		"POSTGRES_USER":     "fleetflow",
		"POSTGRES_PASSWORD": password,
	}))
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	u, err := url.Parse(cfg.DatabaseDSN)
	if err != nil {
		t.Fatalf("DatabaseDSN = %q, which does not parse: %v", cfg.DatabaseDSN, err)
	}
	if got, _ := u.User.Password(); got != password {
		t.Errorf("password round-tripped as %q, want %q", got, password)
	}
	if strings.Contains(cfg.DatabaseDSN, password) {
		t.Errorf("the raw password appears unescaped in the DSN")
	}
}

func TestLoad_RejectsAPasswordWithoutAUser(t *testing.T) {
	_, err := Load(env(map[string]string{
		"POSTGRES_HOST":     "127.0.0.1",
		"POSTGRES_PASSWORD": "change-me",
	}))
	if err == nil {
		t.Fatal("Load() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "POSTGRES_USER") {
		t.Errorf("error = %q, want it to name POSTGRES_USER", err)
	}
}

// TestLoad_DoesNotEchoTheDatabaseURL guards the startup log: a rejected
// connection string may carry a password, and it must not reach a message.
func TestLoad_DoesNotEchoTheDatabaseURL(t *testing.T) {
	_, err := Load(env(map[string]string{
		"DATABASE_URL": "mysql://user:secret@127.0.0.1/fleetflow",
	}))
	if err == nil {
		t.Fatal("Load() error = nil, want an error")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("error = %q, which leaks the password", err)
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error = %q, want it to name DATABASE_URL", err)
	}
}
