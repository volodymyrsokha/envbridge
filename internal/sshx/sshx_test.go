package sshx

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
)

func mapDialer(t *testing.T, options map[string]map[string]string) *Dialer {
	t.Helper()
	return &Dialer{
		GetOption: func(alias, key string) string {
			return options[alias][key]
		},
		KnownHostsPath: filepath.Join(t.TempDir(), "known_hosts"),
		Timeout:        5 * time.Second,
	}
}

func TestResolveExplicitUserHostPort(t *testing.T) {
	d := mapDialer(t, nil)
	got := d.Resolve("deploy@prod.example.com:2222")
	if got.User != "deploy" || got.HostName != "prod.example.com" || got.Port != "2222" {
		t.Errorf("Resolve = %+v", got)
	}
}

func TestResolveConfigAlias(t *testing.T) {
	d := mapDialer(t, map[string]map[string]string{
		"prod-1": {"HostName": "10.1.2.3", "User": "deploy", "Port": "2200", "ProxyJump": "bastion"},
	})
	got := d.Resolve("prod-1")
	if got.HostName != "10.1.2.3" || got.User != "deploy" || got.Port != "2200" || got.ProxyJump != "bastion" {
		t.Errorf("Resolve = %+v", got)
	}
}

func TestResolveExplicitWinsOverConfig(t *testing.T) {
	d := mapDialer(t, map[string]map[string]string{
		"prod-1": {"HostName": "10.1.2.3", "User": "deploy", "Port": "2200"},
	})
	got := d.Resolve("root@prod-1:22")
	if got.User != "root" || got.Port != "22" || got.HostName != "10.1.2.3" {
		t.Errorf("Resolve = %+v", got)
	}
}

func TestResolveDefaults(t *testing.T) {
	d := mapDialer(t, nil)
	got := d.Resolve("example.com")
	if got.HostName != "example.com" || got.Port != "22" {
		t.Errorf("Resolve = %+v", got)
	}
	if got.User == "" {
		t.Error("Resolve did not default User to the current user")
	}
	if len(got.IdentityFiles) == 0 {
		t.Error("Resolve did not provide default identity files")
	}
}

func TestResolveProxyJumpNoneDisables(t *testing.T) {
	d := mapDialer(t, map[string]map[string]string{
		"direct": {"ProxyJump": "none"},
	})
	if got := d.Resolve("direct"); got.ProxyJump != "" {
		t.Errorf("ProxyJump = %q, want disabled", got.ProxyJump)
	}
}

func startServer(t *testing.T) string {
	t.Helper()
	srv := &gliderssh.Server{Handler: func(s gliderssh.Session) {}}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().String()
}

func TestDialTrustOnFirstUse(t *testing.T) {
	addr := startServer(t)
	d := mapDialer(t, nil)

	prompted := 0
	d.TrustPrompt = func(host, fingerprint string) bool {
		prompted++
		if !strings.HasPrefix(fingerprint, "SHA256:") {
			t.Errorf("fingerprint = %q, want SHA256 form", fingerprint)
		}
		return true
	}

	c1, err := d.Dial(addr)
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	c1.Close()
	if prompted != 1 {
		t.Fatalf("prompted %d times, want 1", prompted)
	}

	data, err := os.ReadFile(d.KnownHostsPath)
	if err != nil || len(data) == 0 {
		t.Fatalf("known_hosts not written: %v", err)
	}

	c2, err := d.Dial(addr)
	if err != nil {
		t.Fatalf("second dial: %v", err)
	}
	c2.Close()
	if prompted != 1 {
		t.Errorf("prompted again for a known host (%d times)", prompted)
	}
}

func TestDialRefusesUnknownHostWithoutPrompt(t *testing.T) {
	addr := startServer(t)
	d := mapDialer(t, nil)

	_, err := d.Dial(addr)
	if err == nil {
		t.Fatal("dial to unknown host succeeded without a trust prompt")
	}
	if !strings.Contains(err.Error(), "not trusted") {
		t.Errorf("error = %v, want a not-trusted refusal", err)
	}
}

func TestDialRefusesChangedHostKey(t *testing.T) {
	addr := startServer(t)
	d := mapDialer(t, nil)
	d.TrustPrompt = func(host, fingerprint string) bool { return true }
	c, err := d.Dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()

	// A different server on a new port presents a different host key for the
	// same known_hosts entry when we rewrite the file to claim that address.
	addr2 := startServer(t)
	data, err := os.ReadFile(d.KnownHostsPath)
	if err != nil {
		t.Fatal(err)
	}
	swapped := strings.ReplaceAll(string(data), hostPart(addr), hostPart(addr2))
	if err := os.WriteFile(d.KnownHostsPath, []byte(swapped), 0o600); err != nil {
		t.Fatal(err)
	}
	d.TrustPrompt = func(host, fingerprint string) bool {
		t.Error("TrustPrompt called for a key MISMATCH — must be a hard error")
		return true
	}
	_, err = d.Dial(addr2)
	if err == nil || !strings.Contains(err.Error(), "MISMATCH") {
		t.Errorf("changed host key: err = %v, want hard mismatch error", err)
	}
}

func hostPart(addr string) string {
	host, port, _ := net.SplitHostPort(addr)
	return "[" + host + "]:" + port
}
