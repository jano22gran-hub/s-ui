package util

import (
	"encoding/base64"
	"testing"
)

// External IPv6 nodes must reach the outbound with a bare address, whatever
// the link looks like (#1281).
func TestGetOutboundIPv6Server(t *testing.T) {
	vm := base64.StdEncoding.EncodeToString([]byte(`{"v":"2","ps":"vm","add":"[240e:1::1234]","port":"443","id":"a3482e88-686a-4a58-8126-99c9df64b7bf","net":"tcp"}`))
	cases := map[string]string{
		"vless bracketed": "vless://a3482e88-686a-4a58-8126-99c9df64b7bf@[240e:1::1234]:443?security=tls#v",
		"vless no port":   "vless://a3482e88-686a-4a58-8126-99c9df64b7bf@[240e:1::1234]?security=tls#v",
		"trojan":          "trojan://pw@[240e:1::1234]:443#t",
		"hysteria2":       "hysteria2://pw@[240e:1::1234]:443#h",
		"vmess bracketed": "vmess://" + vm,
	}
	for name, link := range cases {
		out, _, err := GetOutbound(link, 0)
		if err != nil || out == nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := (*out)["server"]; got != "240e:1::1234" {
			t.Errorf("%s: server = %v, want 240e:1::1234", name, got)
		}
	}
}
