package service

import (
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/dnstunnel"
)

func TestDnsTunnelSaveValidates(t *testing.T) {
	db := clientTestDB(t)
	if err := db.Create(&model.Inbound{Type: "mixed", Tag: "m", Options: json.RawMessage(`{"listen":"::","listen_port":2080}`)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Inbound{Type: "vless", Tag: "v", Options: json.RawMessage(`{"listen_port":443}`)}).Error; err != nil {
		t.Fatal(err)
	}
	createClient(t, db, &model.Client{Name: "u1", Enable: true, Config: json.RawMessage(`{"mixed":{"username":"u1","password":"p1"}}`)})
	s := &DnsTunnelService{}

	bad := map[string]string{
		"domain":  `{"tag":"t","domain":"not a domain","inbound":"m","method":1}`,
		"inbound": `{"tag":"t","domain":"t.example.com","inbound":"v","method":1}`,
		"method":  `{"tag":"t","domain":"t.example.com","inbound":"m","method":9}`,
	}
	for name, raw := range bad {
		if err := s.Save(db, "new", json.RawMessage(raw)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}

	if err := s.Save(db, "new", json.RawMessage(`{"tag":"t","enable":true,"domain":"T.Example.com.","inbound":"m","client":"u1","method":5}`)); err != nil {
		t.Fatal(err)
	}
	var got model.DnsTunnel
	db.First(&got)
	if got.Domain != "t.example.com" || got.Port != 53 || len(got.Key) != 32 {
		t.Fatalf("normalized tunnel = %+v", got)
	}

	spec, err := s.spec(db, &got)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Config["FORWARD_IP"] != "127.0.0.1" || spec.Config["FORWARD_PORT"] != 2080 ||
		spec.Config["SOCKS5_USER"] != "u1" || spec.Config["SOCKS5_PASS"] != "p1" {
		t.Fatalf("spec = %+v", spec.Config)
	}
}

// Runs the real MasterDnsVPN server when SUI_MDV_BIN points at one.
func TestDnsTunnelProcessAnswersDNS(t *testing.T) {
	if os.Getenv("SUI_MDV_BIN") == "" {
		t.Skip("SUI_MDV_BIN not set")
	}
	t.Setenv("SUI_DB_FOLDER", t.TempDir())
	dnstunnel.Apply([]dnstunnel.Spec{{
		Tag: "e2e",
		Key: "0123456789abcdef0123456789abcdef",
		Config: map[string]any{
			"DOMAIN": []string{"t.example.com"}, "PROTOCOL_TYPE": "SOCKS5",
			"UDP_HOST": "127.0.0.1", "UDP_PORT": 15353, "DATA_ENCRYPTION_METHOD": 5,
		},
	}})
	defer dnstunnel.StopAll()

	// A plain query for the zone must get an answer once the server is up.
	query := []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0,
		1, 't', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0, 0, 16, 0, 1}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("udp", "127.0.0.1:15353")
		if err == nil {
			conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
			conn.Write(query)
			buf := make([]byte, 512)
			n, err := conn.Read(buf)
			conn.Close()
			if err == nil && n >= 12 && buf[0] == 0x12 && buf[1] == 0x34 {
				if st := dnstunnel.Statuses()["e2e"]; !st.Running {
					t.Fatalf("status = %+v", st)
				}
				return
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("no DNS answer; status = %+v", dnstunnel.Statuses()["e2e"])
}
