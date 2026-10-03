package model

import "encoding/json"

// DnsTunnel is a MasterDnsVPN server run by the panel. It answers DNS queries
// for Domain and forwards every tunneled connection, over SOCKS5, into the
// local socks/mixed inbound named by Inbound, authenticating as Client. The
// traffic is therefore counted and limited like any other of that client's.
type DnsTunnel struct {
	Id     uint   `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Tag    string `json:"tag" form:"tag" gorm:"unique"`
	Enable bool   `json:"enable" form:"enable"`
	// Domain is the delegated (NS) zone, e.g. "t.example.com".
	Domain string `json:"domain" form:"domain"`
	Listen string `json:"listen" form:"listen"`
	Port   int    `json:"port" form:"port"`
	// Method is MasterDnsVPN's DATA_ENCRYPTION_METHOD (0 none .. 5 AES-256-GCM).
	Method  int    `json:"method" form:"method"`
	Key     string `json:"key" form:"key"`
	Inbound string `json:"inbound" form:"inbound"`
	Client  string `json:"client" form:"client"`
	// Options holds extra MasterDnsVPN server keys (upper-case TOML names),
	// applied over the generated ones.
	Options json.RawMessage `json:"options" form:"options"`
}
