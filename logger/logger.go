package logger

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/op/go-logging"
)

type logEntry struct {
	time  string
	level logging.Level
	log   string
}

var (
	logger  *logging.Logger
	leveled logging.LeveledBackend

	// bufferMu guards logBuffer. Every cron job, HTTP handler and sing-box
	// connection goroutine appends to it, while GetLogs reads it from the
	// /logs endpoint -- an unsynchronised append against the re-slice below
	// can hand the reader a stale header and an out-of-range index.
	bufferMu  sync.Mutex
	logBuffer []logEntry
)

func InitLogger(level logging.Level) {
	newLogger := logging.MustGetLogger("s-ui")
	var err error
	var backend logging.Backend
	var format logging.Formatter

	_, inContainer := os.LookupEnv("container")
	if !inContainer {
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			inContainer = true
		}
	}
	if inContainer {
		backend = logging.NewLogBackend(os.Stderr, "", 0)
		format = logging.MustStringFormatter(`%{time:2006/01/02 15:04:05} %{level} - %{message}`)
	} else {
		backend, err = logging.NewSyslogBackend("")
		if err != nil {
			fmt.Println("Unable to use syslog: " + err.Error())
			backend = logging.NewLogBackend(os.Stderr, "", 0)
		}
		if err != nil {
			format = logging.MustStringFormatter(`%{time:2006/01/02 15:04:05} %{level} - %{message}`)
		} else {
			format = logging.MustStringFormatter(`%{level} - %{message}`)
		}
	}

	backendFormatter := logging.NewBackendFormatter(backend, format)
	backendLeveled := logging.AddModuleLevel(backendFormatter)
	backendLeveled.SetLevel(level, "s-ui")
	newLogger.SetBackend(backendLeveled)

	logger = newLogger
	leveled = backendLeveled
}

// ParseLevel maps a panel/env level name to a go-logging level.
func ParseLevel(name string) (logging.Level, bool) {
	switch name {
	case "debug":
		return logging.DEBUG, true
	case "info":
		return logging.INFO, true
	case "warn", "warning":
		return logging.WARNING, true
	case "error":
		return logging.ERROR, true
	}
	return logging.INFO, false
}

// SetLevel changes the level at runtime. It also gates what reaches the
// in-memory buffer shown in the panel.
func SetLevel(level logging.Level) {
	if leveled != nil {
		leveled.SetLevel(level, "s-ui")
	}
}

func enabled(level logging.Level) bool {
	// Not logger.IsEnabledFor: that asks go-logging's global default backend,
	// not the one InitLogger installed.
	return leveled == nil || leveled.IsEnabledFor(level, "s-ui")
}

func GetLogger() *logging.Logger {
	return logger
}

func Debug(args ...interface{}) {
	logger.Debug(args...)
	addToBuffer("DEBUG", fmt.Sprint(args...))
}

func Debugf(format string, args ...interface{}) {
	logger.Debugf(format, args...)
	addToBuffer("DEBUG", fmt.Sprintf(format, args...))
}

func Info(args ...interface{}) {
	logger.Info(args...)
	addToBuffer("INFO", fmt.Sprint(args...))
}

func Infof(format string, args ...interface{}) {
	logger.Infof(format, args...)
	addToBuffer("INFO", fmt.Sprintf(format, args...))
}

func Warning(args ...interface{}) {
	logger.Warning(args...)
	addToBuffer("WARNING", fmt.Sprint(args...))
}

func Warningf(format string, args ...interface{}) {
	logger.Warningf(format, args...)
	addToBuffer("WARNING", fmt.Sprintf(format, args...))
}

func Error(args ...interface{}) {
	logger.Error(args...)
	addToBuffer("ERROR", fmt.Sprint(args...))
}

func Errorf(format string, args ...interface{}) {
	logger.Errorf(format, args...)
	addToBuffer("ERROR", fmt.Sprintf(format, args...))
}

func addToBuffer(level string, newLog string) {
	t := time.Now()
	logLevel, _ := logging.LogLevel(level)
	// Lines below the configured level used to be buffered anyway, so the
	// panel showed debug output whatever level was chosen.
	if !enabled(logLevel) {
		return
	}

	bufferMu.Lock()
	defer bufferMu.Unlock()

	if len(logBuffer) >= 10240 {
		logBuffer = logBuffer[1:]
	}
	logBuffer = append(logBuffer, logEntry{
		time:  t.Format("2006/01/02 15:04:05"),
		level: logLevel,
		log:   newLog,
	})
}

func GetLogs(c int, level string) []string {
	var output []string
	logLevel, _ := logging.LogLevel(level)

	bufferMu.Lock()
	defer bufferMu.Unlock()

	// `len(output) < c`, not `<=`: the old condition was still true when the
	// slice already held c entries, so every call returned one more line than
	// the caller asked for.
	for i := len(logBuffer) - 1; i >= 0 && len(output) < c; i-- {
		if logBuffer[i].level <= logLevel {
			output = append(output, fmt.Sprintf("%s %s - %s", logBuffer[i].time, logBuffer[i].level, logBuffer[i].log))
		}
	}
	return output
}
