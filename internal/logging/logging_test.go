package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"fleetflow/internal/config"
	"fleetflow/internal/logging"
)

// line parses the single line a logger wrote, failing the test if it wrote none.
func line(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	if buf.Len() == 0 {
		t.Fatal("nothing was logged")
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got); err != nil {
		t.Fatalf("line is not JSON: %v (%s)", err, buf.String())
	}
	return got
}

// TestNew_LevelIsTheFloor is the property the whole setting exists for: a level
// below the floor is dropped, and one at or above it is written.
func TestNew_LevelIsTheFloor(t *testing.T) {
	tests := []struct {
		level string
		// wroteDebug and wroteWarn say what the logger must keep.
		wroteDebug bool
		wroteWarn  bool
	}{
		{config.LogDebug, true, true},
		{config.LogInfo, false, true},
		{config.LogWarn, false, true},
		{config.LogError, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			var buf bytes.Buffer
			logger := logging.New(&buf, logging.Options{Level: tt.level, Format: logging.FormatJSON})

			logger.Debug("debug line")
			gotDebug := buf.Len() > 0
			buf.Reset()

			logger.Warn("warn line")
			gotWarn := buf.Len() > 0

			if gotDebug != tt.wroteDebug {
				t.Errorf("at level %q, debug written = %v, want %v", tt.level, gotDebug, tt.wroteDebug)
			}
			if gotWarn != tt.wroteWarn {
				t.Errorf("at level %q, warn written = %v, want %v", tt.level, gotWarn, tt.wroteWarn)
			}
		})
	}
}

// TestNew_UnknownLevelDoesNotBecomeDebug is the failure mode worth pinning: slog's
// zero level is debug, so an unrecognised name that fell through would turn on the
// most verbose setting the program has.
func TestNew_UnknownLevelDoesNotBecomeDebug(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, logging.Options{Level: "verbose", Format: logging.FormatJSON})

	logger.Debug("should not appear")
	if buf.Len() != 0 {
		t.Errorf("an unknown level enabled debug: %s", buf.String())
	}

	logger.Info("should appear")
	if buf.Len() == 0 {
		t.Error("an unknown level dropped info as well")
	}
}

// TestNew_JSONCarriesTheFieldsACollectorReads checks the shape Cloud Run parses:
// a level it can map to a severity and a message it can show.
func TestNew_JSONCarriesTheFieldsACollectorReads(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, logging.Options{
		Level:  config.LogInfo,
		Format: logging.FormatJSON,
		Env:    config.EnvProduction,
	})

	logger.Error("database unreachable", slog.String("error", "connection refused"))

	got := line(t, &buf)
	if got["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", got["level"])
	}
	if got["msg"] != "database unreachable" {
		t.Errorf("msg = %v", got["msg"])
	}
	if got["error"] != "connection refused" {
		t.Errorf("error = %v", got["error"])
	}
	if got["env"] != config.EnvProduction {
		t.Errorf("env = %v, want it on every line", got["env"])
	}
}

// TestNew_TextIsReadable keeps the development format honest: it is for a person, so
// it is not JSON, but it still carries the level and the message.
func TestNew_TextIsReadable(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, logging.Options{Level: config.LogInfo, Format: logging.FormatText})

	logger.Info("listening", slog.String("addr", ":8080"))

	out := buf.String()
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("text format emitted JSON: %s", out)
	}
	for _, want := range []string{"level=INFO", "msg=listening", "addr=:8080"} {
		if !strings.Contains(out, want) {
			t.Errorf("text line %q does not contain %q", out, want)
		}
	}
}

// TestFormatFor pins the default per environment, which is the only place the two
// formats are chosen automatically.
func TestFormatFor(t *testing.T) {
	if got := logging.FormatFor(config.EnvProduction); got != logging.FormatJSON {
		t.Errorf("FormatFor(production) = %q, want json", got)
	}
	if got := logging.FormatFor(config.EnvDevelopment); got != logging.FormatText {
		t.Errorf("FormatFor(development) = %q, want text", got)
	}
}
