package sub

import (
	"encoding/json"
	"testing"
)

// The system stack left TCP flows from Android apps stuck after pre-match
// (DNS resolved, no connection reached an outbound) on a Samsung phone over
// cellular; gvisor, which runs in the client process, works there. sing-box
// 1.15 still accepts "gvisor" while deprecating the option.
func TestDefaultTunUsesGvisorStack(t *testing.T) {
	var cfg struct {
		Inbounds []map[string]any `json:"inbounds"`
	}
	if err := json.Unmarshal([]byte(defaultJson), &cfg); err != nil {
		t.Fatal(err)
	}
	for _, in := range cfg.Inbounds {
		if in["type"] == "tun" {
			if in["stack"] != "gvisor" {
				t.Fatalf("tun stack = %v, want gvisor", in["stack"])
			}
			return
		}
	}
	t.Fatal("no tun inbound in the default subscription config")
}
