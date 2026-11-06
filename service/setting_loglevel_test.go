package service

import (
	"encoding/json"
	"testing"
)

// The panel log level used to come only from SUI_LOG_LEVEL, so whatever was
// picked in the panel was back to info after every restart.
func TestLogLevelSettingPersists(t *testing.T) {
	t.Setenv("SUI_LOG_LEVEL", "")
	t.Setenv("SUI_DEBUG", "")
	db := settingTestDB(t)
	s := &SettingService{}
	if _, err := s.GetAllSetting(); err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(map[string]string{"logLevel": "warn"})
	if err := s.Save(db, payload); err != nil {
		t.Fatal(err)
	}
	if got := s.GetLogLevel(); got != "warn" {
		t.Fatalf("logLevel = %q after save, want warn", got)
	}

	payload, _ = json.Marshal(map[string]string{"logLevel": "loud"})
	if err := s.Save(db, payload); err == nil {
		t.Error("an invalid log level was accepted")
	}
	if got := s.GetLogLevel(); got != "warn" {
		t.Errorf("logLevel = %q after a rejected save, want warn", got)
	}
}
