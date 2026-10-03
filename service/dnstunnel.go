package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/dnstunnel"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"

	"gorm.io/gorm"
)

type DnsTunnelService struct{}

var domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

// keyLength is the hex key length MasterDnsVPN expects for each method.
func keyLength(method int) int {
	switch method {
	case 3:
		return 16
	case 4:
		return 24
	default:
		return 32
	}
}

func newTunnelKey(method int) (string, error) {
	n := keyLength(method)
	buf := make([]byte, (n+1)/2)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf)[:n], nil
}

// GetAll returns every tunnel with its live process state under "status".
func (s *DnsTunnelService) GetAll() ([]map[string]any, error) {
	var tunnels []model.DnsTunnel
	if err := database.GetDB().Model(model.DnsTunnel{}).Order("id").Find(&tunnels).Error; err != nil {
		return nil, err
	}
	statuses := dnstunnel.Statuses()
	out := make([]map[string]any, 0, len(tunnels))
	for _, t := range tunnels {
		raw, _ := json.Marshal(t)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		m["status"] = statuses[t.Tag]
		out = append(out, m)
	}
	return out, nil
}

func (s *DnsTunnelService) Save(tx *gorm.DB, act string, data json.RawMessage) error {
	switch act {
	case "new", "edit":
		var t model.DnsTunnel
		if err := json.Unmarshal(data, &t); err != nil {
			return err
		}
		if err := s.normalize(tx, &t); err != nil {
			return err
		}
		return tx.Save(&t).Error
	case "del":
		var tag string
		if err := json.Unmarshal(data, &tag); err != nil {
			return err
		}
		return tx.Where("tag = ?", tag).Delete(model.DnsTunnel{}).Error
	}
	return common.NewErrorf("unknown action: %s", act)
}

func (s *DnsTunnelService) normalize(tx *gorm.DB, t *model.DnsTunnel) error {
	t.Tag = strings.TrimSpace(t.Tag)
	if t.Tag == "" || strings.ContainsAny(t.Tag, `/\.`) {
		return common.NewError("invalid tag")
	}
	t.Domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(t.Domain)), ".")
	if !domainRe.MatchString(t.Domain) {
		return common.NewError("invalid domain: ", t.Domain)
	}
	if t.Port == 0 {
		t.Port = 53
	}
	if t.Port < 1 || t.Port > 65535 {
		return common.NewError("invalid port")
	}
	if t.Listen != "" && net.ParseIP(t.Listen) == nil {
		return common.NewError("invalid listen address")
	}
	if t.Method < 0 || t.Method > 5 {
		return common.NewError("invalid encryption method")
	}
	t.Key = strings.TrimSpace(t.Key)
	if t.Method == 0 {
		t.Key = ""
	} else if len(t.Key) != keyLength(t.Method) {
		key, err := newTunnelKey(t.Method)
		if err != nil {
			return err
		}
		t.Key = key
	}
	if len(t.Options) > 0 && string(t.Options) != "null" {
		var opts map[string]any
		if err := json.Unmarshal(t.Options, &opts); err != nil {
			return common.NewError("options must be a JSON object")
		}
	} else {
		t.Options = nil
	}
	var inbound model.Inbound
	if err := tx.Model(model.Inbound{}).Where("tag = ?", t.Inbound).First(&inbound).Error; err != nil {
		return common.NewError("inbound not found: ", t.Inbound)
	}
	if inbound.Type != "socks" && inbound.Type != "mixed" {
		return common.NewError("inbound must be socks or mixed")
	}
	return nil
}

// forwardTarget resolves where and as whom the tunnel enters sing-box.
func forwardTarget(db *gorm.DB, t *model.DnsTunnel) (host string, port int, user, pass string, err error) {
	var inbound model.Inbound
	if err = db.Model(model.Inbound{}).Where("tag = ?", t.Inbound).First(&inbound).Error; err != nil {
		return "", 0, "", "", common.NewError("inbound not found: ", t.Inbound)
	}
	var opts struct {
		Listen     string `json:"listen"`
		ListenPort int    `json:"listen_port"`
	}
	_ = json.Unmarshal(inbound.Options, &opts)
	if opts.ListenPort == 0 {
		return "", 0, "", "", common.NewError("inbound has no listen_port: ", t.Inbound)
	}
	host = opts.Listen
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	if t.Client == "" {
		return host, opts.ListenPort, "", "", nil
	}
	var client model.Client
	if err = db.Model(model.Client{}).Where("name = ?", t.Client).First(&client).Error; err != nil {
		return "", 0, "", "", common.NewError("client not found: ", t.Client)
	}
	var cfg map[string]struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	_ = json.Unmarshal(client.Config, &cfg)
	cred := cfg[inbound.Type]
	return host, opts.ListenPort, cred.Username, cred.Password, nil
}

func (s *DnsTunnelService) spec(db *gorm.DB, t *model.DnsTunnel) (dnstunnel.Spec, error) {
	host, port, user, pass, err := forwardTarget(db, t)
	if err != nil {
		return dnstunnel.Spec{}, err
	}
	cfg := map[string]any{
		"DOMAIN":                 []string{t.Domain},
		"PROTOCOL_TYPE":          "SOCKS5",
		"UDP_HOST":               t.Listen,
		"UDP_PORT":               t.Port,
		"DATA_ENCRYPTION_METHOD": t.Method,
		"USE_EXTERNAL_SOCKS5":    true,
		"FORWARD_IP":             host,
		"FORWARD_PORT":           port,
		"SOCKS5_AUTH":            user != "",
		"SOCKS5_USER":            user,
		"SOCKS5_PASS":            pass,
		"LOG_LEVEL":              "INFO",
	}
	if len(t.Options) > 0 {
		var opts map[string]any
		if err := json.Unmarshal(t.Options, &opts); err == nil {
			for k, v := range opts {
				cfg[strings.ToUpper(k)] = v
			}
		}
	}
	key := t.Key
	if t.Method == 0 {
		// The server still reads a key file; any value works with no cipher.
		key = strings.Repeat("0", 32)
	}
	return dnstunnel.Spec{Tag: t.Tag, Key: key, Config: cfg}, nil
}

// Sync starts, restarts and stops tunnel processes to match the database. It
// is cheap when nothing changed, so it runs after every related save.
func (s *DnsTunnelService) Sync() {
	db := database.GetDB()
	var tunnels []model.DnsTunnel
	if err := db.Model(model.DnsTunnel{}).Where("enable = true").Find(&tunnels).Error; err != nil {
		logger.Warning("dns tunnel sync: ", err)
		return
	}
	specs := make([]dnstunnel.Spec, 0, len(tunnels))
	for i := range tunnels {
		spec, err := s.spec(db, &tunnels[i])
		if err != nil {
			logger.Warning("dns tunnel ", tunnels[i].Tag, ": ", err)
			continue
		}
		specs = append(specs, spec)
	}
	dnstunnel.Apply(specs)
}

// Install downloads the MasterDnsVPN server and restarts running tunnels on it.
func (s *DnsTunnelService) Install(version string) (string, error) {
	name, err := dnstunnel.Install(version)
	if err != nil {
		return "", err
	}
	dnstunnel.StopAll()
	s.Sync()
	return name, nil
}

// ClientConfig renders the MasterDnsVPN client_config.toml lines a user needs
// for this tunnel. Resolvers are left to the client.
func (s *DnsTunnelService) ClientConfig(tag string) (string, error) {
	var t model.DnsTunnel
	if err := database.GetDB().Model(model.DnsTunnel{}).Where("tag = ?", tag).First(&t).Error; err != nil {
		return "", common.NewError("tunnel not found: ", tag)
	}
	return fmt.Sprintf(`# MasterDnsVPN client config for %s
DOMAINS = [%q]
DATA_ENCRYPTION_METHOD = %d
ENCRYPTION_KEY = %q
PROTOCOL_TYPE = "SOCKS5"
LISTEN_IP = "127.0.0.1"
LISTEN_PORT = 18000
`, t.Tag, t.Domain, t.Method, t.Key), nil
}
