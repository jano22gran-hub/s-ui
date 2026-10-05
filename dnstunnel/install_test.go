package dnstunnel

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestFindServerBinaryVersionedName(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range []string{"server_config.toml", "MasterDnsVPN_Server_Linux_AMD64_v2026.06.13.234407-7de2476"} {
		f, _ := w.Create(name)
		f.Write([]byte("x"))
	}
	w.Close()
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	got := findServerBinary(zr)
	if got == nil || got.Name != "MasterDnsVPN_Server_Linux_AMD64_v2026.06.13.234407-7de2476" {
		t.Fatalf("got %v", got)
	}
}
