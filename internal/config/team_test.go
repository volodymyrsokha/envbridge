package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const teamFixture = `# team config — reviewed in PRs
version: 1
project: acme
store: /var/lib/envbridge/acme

environments:
  production:
    host: prod-1 # our main box
    materialize: /srv/acme/.env
    local: .env.production

recipients:
  # devops first
  - name: Dana
    email: dana@acme.dev
    key: age1dana000000000000000000000000000000000000000000000000000000
  - name: Vlad
    email: vlad@acme.dev
    key: age1vlad000000000000000000000000000000000000000000000000000000
`

func writeFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ProjectFileName)
	if err := os.WriteFile(path, []byte(teamFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAddRecipientPreservesCommentsAndOrder(t *testing.T) {
	path := writeFixture(t)
	err := AddRecipient(path, Recipient{
		Name:  "Bob",
		Email: "bob@acme.dev",
		Key:   "age1bob0000000000000000000000000000000000000000000000000000000",
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# team config — reviewed in PRs",
		"# our main box",
		"# devops first",
		"name: Bob",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output lost %q:\n%s", want, out)
		}
	}

	p, err := LoadProject(path)
	if err != nil {
		t.Fatalf("file no longer loads: %v", err)
	}
	if len(p.Recipients) != 3 || p.Recipients[2].Name != "Bob" {
		t.Errorf("recipients = %+v", p.Recipients)
	}
}

func TestAddRecipientRejectsDuplicateKey(t *testing.T) {
	path := writeFixture(t)
	err := AddRecipient(path, Recipient{
		Name:  "Imposter",
		Email: "x@x",
		Key:   "age1vlad000000000000000000000000000000000000000000000000000000",
	})
	if err == nil || !strings.Contains(err.Error(), "already a recipient") {
		t.Errorf("duplicate add: err = %v", err)
	}
}

func TestRemoveRecipientByNameAndEmail(t *testing.T) {
	path := writeFixture(t)
	removed, err := RemoveRecipient(path, "DANA@acme.dev")
	if err != nil {
		t.Fatal(err)
	}
	if removed.Name != "Dana" {
		t.Errorf("removed = %+v", removed)
	}
	p, err := LoadProject(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Recipients) != 1 || p.Recipients[0].Name != "Vlad" {
		t.Errorf("recipients = %+v", p.Recipients)
	}

	if _, err := RemoveRecipient(path, "Vlad"); err == nil || !strings.Contains(err.Error(), "last recipient") {
		t.Errorf("removing last recipient: err = %v", err)
	}
}

func TestRemoveRecipientUnknown(t *testing.T) {
	path := writeFixture(t)
	if _, err := RemoveRecipient(path, "nobody"); err == nil || !strings.Contains(err.Error(), "team list") {
		t.Errorf("unknown removal: err = %v", err)
	}
}
