package sub

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/core"

	"github.com/sagernet/sing-box/option"
)

// The defaults a client gets when the panel has no JSON template must be a
// config sing-box accepts: the DNS detour, the domain resolver and the
// hijack-dns rule all reference tags that have to exist.
func TestDefaultClientConfigIsValid(t *testing.T) {
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(defaultJson), &cfg); err != nil {
		t.Fatal(err)
	}
	outbounds := []map[string]interface{}{{"type": "socks", "tag": "node", "server": "127.0.0.1", "server_port": 1080}}
	tags := []string{"node"}
	(&JsonService{}).addDefaultOutbounds(&outbounds, &tags)
	cfg["outbounds"] = outbounds
	cfg["dns"] = defaultDNS()
	cfg["route"] = map[string]interface{}{
		"final":                   "proxy",
		"default_domain_resolver": defaultLocalDNSTag,
		"rules": []interface{}{
			map[string]interface{}{"action": "sniff"},
			map[string]interface{}{"protocol": "dns", "action": "hijack-dns"},
			map[string]interface{}{"ip_is_private": true, "action": "route", "outbound": "direct"},
		},
	}
	raw, _ := json.Marshal(cfg)

	ctx := core.NewCore().GetCtx()
	var opt option.Options
	if err := opt.UnmarshalJSONContext(ctx, raw); err != nil {
		t.Fatalf("sing-box rejects the default client config: %v", err)
	}
	box, err := core.NewBox(core.Options{Context: ctx, Options: opt})
	if err != nil {
		t.Fatalf("sing-box cannot build the default client config: %v", err)
	}
	_ = box.Close()
}
