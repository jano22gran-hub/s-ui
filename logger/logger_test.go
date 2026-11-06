package logger

import (
	"strings"
	"sync"
	"testing"

	"github.com/op/go-logging"
)

// Cron jobs, HTTP handlers and sing-box goroutines all log while /logs reads
// the buffer. Before addToBuffer and GetLogs shared a mutex this tripped -race
// on both the append and the re-slice; without -race it only proves no panic.
func TestBufferConcurrentAccess(t *testing.T) {
	InitLogger(logging.DEBUG)
	resetBuffer()

	const writers = 8
	const perWriter = 400

	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < perWriter; j++ {
				Info("writer", n, "line", j)
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < perWriter; j++ {
			_ = GetLogs(50, "debug")
		}
	}()
	wg.Wait()

	if got := len(GetLogs(10000, "debug")); got != writers*perWriter {
		t.Errorf("expected every logged line to survive, got %d of %d", got, writers*perWriter)
	}
}

// The loop condition was `len(output) <= c`, so each call returned c+1 lines.
func TestGetLogsRespectsCount(t *testing.T) {
	InitLogger(logging.DEBUG)
	resetBuffer()

	for i := 0; i < 20; i++ {
		Info("line", i)
	}

	for _, count := range []int{0, 1, 5, 20, 50} {
		if got := len(GetLogs(count, "debug")); got > count {
			t.Errorf("GetLogs(%d) returned %d lines, want at most %d", count, got, count)
		}
	}
}

// The mutex change must not alter which levels are returned.
func TestGetLogsFiltersByLevel(t *testing.T) {
	InitLogger(logging.DEBUG)
	resetBuffer()

	Debug("a debug line")
	Error("an error line")

	errorsOnly := GetLogs(100, "error")
	if len(errorsOnly) != 1 {
		t.Fatalf("expected only the error line at level error, got %d lines: %v", len(errorsOnly), errorsOnly)
	}
	if !strings.Contains(errorsOnly[0], "an error line") {
		t.Errorf("expected the error line, got %q", errorsOnly[0])
	}

	if got := len(GetLogs(100, "debug")); got != 2 {
		t.Errorf("expected both lines at level debug, got %d", got)
	}
}

// The ring-buffer trim, which the concurrent append used to race against.
func TestBufferIsBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("fills the 10240-entry buffer")
	}
	InitLogger(logging.DEBUG)
	resetBuffer()

	for i := 0; i < 10300; i++ {
		Info("line", i)
	}

	bufferMu.Lock()
	size := len(logBuffer)
	bufferMu.Unlock()
	if size == 0 {
		t.Fatal("nothing was buffered")
	}
	if size > 10240 {
		t.Errorf("buffer grew past its bound: %d entries", size)
	}
}

func resetBuffer() {
	bufferMu.Lock()
	logBuffer = nil
	bufferMu.Unlock()
}

// The buffer behind the panel's log view used to keep every line, so debug
// output showed up whatever level was configured.
func TestBufferHonoursLevel(t *testing.T) {
	InitLogger(logging.WARNING)
	resetBuffer()

	Debug("d")
	Info("i")
	Warning("w")
	Error("e")
	if got := len(GetLogs(100, "debug")); got != 2 {
		t.Fatalf("buffered %d lines at WARNING, want 2", got)
	}

	SetLevel(logging.DEBUG)
	Debug("d2")
	if got := len(GetLogs(100, "debug")); got != 3 {
		t.Fatalf("buffered %d lines after SetLevel(DEBUG), want 3", got)
	}
}

func TestParseLevel(t *testing.T) {
	for name, want := range map[string]logging.Level{
		"debug": logging.DEBUG, "info": logging.INFO, "warn": logging.WARNING,
		"warning": logging.WARNING, "error": logging.ERROR,
	} {
		if got, ok := ParseLevel(name); !ok || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v", name, got, ok)
		}
	}
	if _, ok := ParseLevel("loud"); ok {
		t.Error("unknown level accepted")
	}
}
