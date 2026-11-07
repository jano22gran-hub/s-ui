package sub

import "testing"

func TestEnsureCacheFileDefaultsOn(t *testing.T) {
	cfg := map[string]interface{}{}
	ensureCacheFile(cfg)
	cf := cfg["experimental"].(map[string]interface{})["cache_file"].(map[string]interface{})
	if cf["enabled"] != true {
		t.Fatalf("cache_file = %v, want enabled", cf)
	}
}

func TestEnsureCacheFileKeepsOperatorChoice(t *testing.T) {
	cfg := map[string]interface{}{
		"experimental": map[string]interface{}{
			"cache_file": map[string]interface{}{"enabled": false},
			"clash_api":  map[string]interface{}{"external_controller": "127.0.0.1:9090"},
		},
	}
	ensureCacheFile(cfg)
	exp := cfg["experimental"].(map[string]interface{})
	if exp["cache_file"].(map[string]interface{})["enabled"] != false {
		t.Fatal("operator's cache_file was overridden")
	}
	if _, ok := exp["clash_api"]; !ok {
		t.Fatal("other experimental options were dropped")
	}
}

func TestEnsureCacheFileAddsToExistingExperimental(t *testing.T) {
	cfg := map[string]interface{}{"experimental": map[string]interface{}{"clash_api": map[string]interface{}{}}}
	ensureCacheFile(cfg)
	exp := cfg["experimental"].(map[string]interface{})
	if _, ok := exp["cache_file"]; !ok {
		t.Fatal("cache_file not added next to existing experimental options")
	}
}
