package agecrypt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	other, _ := age.GenerateX25519Identity()

	plaintext := []byte("DATABASE_URL=postgres://secret\n")
	blob, err := Encrypt(plaintext, []string{id.Recipient().String(), other.Recipient().String()})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), "postgres://secret") {
		t.Fatal("plaintext leaked into ciphertext")
	}

	for _, decryptor := range []*age.X25519Identity{id, other} {
		got, err := Decrypt(blob, decryptor)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(plaintext) {
			t.Errorf("decrypted %q, want %q", got, plaintext)
		}
	}
}

func TestDecryptWrongKey(t *testing.T) {
	id, _ := age.GenerateX25519Identity()
	stranger, _ := age.GenerateX25519Identity()

	blob, err := Encrypt([]byte("x"), []string{id.Recipient().String()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Decrypt(blob, stranger)
	if err == nil {
		t.Fatal("Decrypt with wrong key succeeded")
	}
	if !strings.Contains(err.Error(), "not encrypted for your key") {
		t.Errorf("unfriendly error: %v", err)
	}
}

func TestEncryptZeroRecipients(t *testing.T) {
	if _, err := Encrypt([]byte("x"), nil); err == nil {
		t.Fatal("Encrypt with no recipients succeeded")
	}
}

func TestEncryptInvalidRecipient(t *testing.T) {
	if _, err := Encrypt([]byte("x"), []string{"not-a-key"}); err == nil {
		t.Fatal("Encrypt with invalid recipient succeeded")
	}
}

func TestGenerateAndLoadIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "identity.txt")

	id, err := GenerateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("identity file mode = %04o, want 0600", info.Mode().Perm())
	}

	loaded, err := LoadIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Recipient().String() != id.Recipient().String() {
		t.Error("loaded identity does not match generated one")
	}

	if _, err := GenerateIdentity(path); err == nil {
		t.Fatal("GenerateIdentity overwrote an existing identity")
	}
}

func TestLoadIdentityRejectsLoosePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.txt")
	if _, err := GenerateIdentity(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadIdentity(path)
	if err == nil {
		t.Fatal("LoadIdentity accepted 0644 file")
	}
	if !strings.Contains(err.Error(), "chmod 600") {
		t.Errorf("error lacks remediation: %v", err)
	}
}

func TestLoadIdentityMissing(t *testing.T) {
	_, err := LoadIdentity(filepath.Join(t.TempDir(), "nope.txt"))
	if err == nil || !strings.Contains(err.Error(), "envbridge keygen") {
		t.Errorf("missing-identity error lacks keygen hint: %v", err)
	}
}
