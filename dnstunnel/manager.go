// Package dnstunnel runs MasterDnsVPN servers as child processes of the panel.
//
// MasterDnsVPN keeps its code under internal/, so it cannot be linked in; the
// panel downloads its release binary instead (see install.go) and supervises
// one process per enabled tunnel, restarting it when it dies.
package dnstunnel

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/logger"
)

const (
	minBackoff = 2 * time.Second
	maxBackoff = time.Minute
	// A process that stayed up this long is healthy again; its backoff resets.
	healthyRun = 30 * time.Second
)

// Spec is everything a tunnel process needs, resolved by the service layer.
type Spec struct {
	Tag    string
	Key    string
	Config map[string]any
}

func (s Spec) hash() string {
	raw, _ := json.Marshal(struct {
		K string
		C map[string]any
	}{s.Key, s.Config})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Status is reported to the panel.
type Status struct {
	Running   bool   `json:"running"`
	Pid       int    `json:"pid,omitempty"`
	Restarts  int    `json:"restarts"`
	LastError string `json:"lastError,omitempty"`
	Since     int64  `json:"since,omitempty"`
}

type proc struct {
	spec   Spec
	hash   string
	cancel context.CancelFunc
	done   chan struct{}

	mu     sync.Mutex
	status Status
}

var (
	mu    sync.Mutex
	procs = map[string]*proc{}
)

// WorkDir holds one directory per tunnel: its config and key file.
func WorkDir() string {
	return filepath.Join(config.GetDBFolderPath(), "dnstunnel")
}

// Apply makes the running set match specs: unchanged tunnels keep running,
// changed ones restart, missing ones stop.
func Apply(specs []Spec) {
	mu.Lock()
	defer mu.Unlock()

	want := make(map[string]Spec, len(specs))
	for _, s := range specs {
		want[s.Tag] = s
	}
	for tag, p := range procs {
		if s, ok := want[tag]; !ok || s.hash() != p.hash {
			p.stop()
			delete(procs, tag)
		}
	}
	for tag, s := range want {
		if _, ok := procs[tag]; ok {
			continue
		}
		p := &proc{spec: s, hash: s.hash(), done: make(chan struct{})}
		var ctx context.Context
		ctx, p.cancel = context.WithCancel(context.Background())
		procs[tag] = p
		go p.supervise(ctx)
	}
}

// StopAll stops every tunnel, for panel shutdown.
func StopAll() {
	Apply(nil)
}

// Statuses returns the state of every managed tunnel by tag.
func Statuses() map[string]Status {
	mu.Lock()
	defer mu.Unlock()
	out := make(map[string]Status, len(procs))
	for tag, p := range procs {
		p.mu.Lock()
		out[tag] = p.status
		p.mu.Unlock()
	}
	return out
}

func (p *proc) stop() {
	p.cancel()
	<-p.done
}

func (p *proc) setStatus(f func(*Status)) {
	p.mu.Lock()
	f(&p.status)
	p.mu.Unlock()
}

func (p *proc) supervise(ctx context.Context) {
	defer close(p.done)
	backoff := minBackoff
	for {
		started := time.Now()
		err := p.run(ctx)
		if ctx.Err() != nil {
			return
		}
		msg := "exited"
		if err != nil {
			msg = err.Error()
		}
		logger.Warning("dns tunnel ", p.spec.Tag, ": ", msg, "; restarting in ", backoff)
		p.setStatus(func(s *Status) {
			s.Running = false
			s.Pid = 0
			s.LastError = msg
			s.Restarts++
		})
		if time.Since(started) > healthyRun {
			backoff = minBackoff
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func (p *proc) run(ctx context.Context) error {
	bin, err := BinaryPath()
	if err != nil {
		return err
	}
	dir := filepath.Join(WorkDir(), p.spec.Tag)
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	keyFile := filepath.Join(dir, "encrypt_key.txt")
	if err = os.WriteFile(keyFile, []byte(p.spec.Key), 0o600); err != nil {
		return err
	}
	cfg := make(map[string]any, len(p.spec.Config)+1)
	for k, v := range p.spec.Config {
		cfg[k] = v
	}
	cfg["ENCRYPTION_KEY_FILE"] = keyFile
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	cfgFile := filepath.Join(dir, "server_config.json")
	if err = os.WriteFile(cfgFile, raw, 0o600); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, bin, "-json", cfgFile, "-nowait")
	cmd.Dir = dir
	cmd.WaitDelay = 5 * time.Second
	bindToParent(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err = cmd.Start(); err != nil {
		return err
	}
	p.setStatus(func(s *Status) {
		s.Running = true
		s.Pid = cmd.Process.Pid
		s.Since = time.Now().Unix()
	})
	logger.Info("dns tunnel ", p.spec.Tag, " started, pid ", cmd.Process.Pid)
	go pipeLog(p.spec.Tag, out)
	err = cmd.Wait()
	p.setStatus(func(s *Status) {
		s.Running = false
		s.Pid = 0
	})
	return err
}

func pipeLog(tag string, r io.Reader) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		logger.Debug("dns tunnel ", tag, ": ", sc.Text())
	}
}
