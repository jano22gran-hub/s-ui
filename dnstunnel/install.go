package dnstunnel

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/alireza0/s-ui/util/common"
)

const releaseBase = "https://github.com/masterking32/MasterDnsVPN/releases"

// BinaryPath returns the MasterDnsVPN server binary: $SUI_MDV_BIN when set,
// otherwise bin/masterdnsvpn-server next to the panel binary.
func BinaryPath() (string, error) {
	if p := os.Getenv("SUI_MDV_BIN"); p != "" {
		return p, nil
	}
	p := defaultBinaryPath()
	if _, err := os.Stat(p); err != nil {
		return "", common.NewError("MasterDnsVPN server is not installed")
	}
	return p, nil
}

func defaultBinaryPath() string {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "bin", "masterdnsvpn-server")
}

// Version runs the installed binary with -version, "" when not installed.
func Version() string {
	bin, err := BinaryPath()
	if err != nil {
		return ""
	}
	out, err := exec.Command(bin, "-version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "MasterDnsVPN Server Version:"))
}

func assetName() (string, error) {
	if runtime.GOOS != "linux" {
		return "", common.NewErrorf("dns tunnel is only supported on linux, not %s", runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "amd64":
		return "MasterDnsVPN_Server_Linux_AMD64", nil
	case "arm64":
		return "MasterDnsVPN_Server_Linux_ARM64", nil
	case "arm":
		return "MasterDnsVPN_Server_Linux_ARMV7", nil
	case "386":
		return "MasterDnsVPN_Server_Linux_X86", nil
	}
	return "", common.NewErrorf("unsupported architecture %s", runtime.GOARCH)
}

// Install downloads a MasterDnsVPN server release ("" for the latest) into
// the default binary path, replacing any previous one atomically.
func Install(version string) (string, error) {
	name, err := assetName()
	if err != nil {
		return "", err
	}
	url := releaseBase + "/latest/download/" + name + ".zip"
	if version != "" {
		url = fmt.Sprintf("%s/download/%s/%s.zip", releaseBase, version, name)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", common.NewErrorf("download %s: %s", url, resp.Status)
	}
	// Release zips are ~7MB; cap well above that.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return "", err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	var entry *zip.File
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if strings.HasPrefix(base, "MasterDnsVPN_Server") && !strings.Contains(base, ".") {
			entry = f
			break
		}
	}
	if entry == nil {
		return "", common.NewError("server binary not found in release archive")
	}

	dst := defaultBinaryPath()
	if err = os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	rc, err := entry.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".mdv-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err = io.Copy(tmp, rc); err != nil {
		tmp.Close()
		return "", err
	}
	if err = tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(tmp.Name(), dst); err != nil {
		return "", err
	}
	return filepath.Base(entry.Name), nil
}
