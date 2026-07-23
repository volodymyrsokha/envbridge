// Package agecrypt wraps filippo.io/age for envbridge's needs: encrypt to
// the team's X25519 recipients, decrypt with the local identity, and manage
// the identity file with enforced permissions.
package agecrypt

import (
	"bytes"
	"fmt"
	"io"

	"filippo.io/age"
)

func ParseRecipient(key string) (*age.X25519Recipient, error) {
	r, err := age.ParseX25519Recipient(key)
	if err != nil {
		return nil, fmt.Errorf("invalid age public key %q: %w", key, err)
	}
	return r, nil
}

func Encrypt(plaintext []byte, recipientKeys []string) ([]byte, error) {
	if len(recipientKeys) == 0 {
		return nil, fmt.Errorf("refusing to encrypt to zero recipients")
	}
	recipients := make([]age.Recipient, 0, len(recipientKeys))
	for _, key := range recipientKeys {
		r, err := ParseRecipient(key)
		if err != nil {
			return nil, err
		}
		recipients = append(recipients, r)
	}
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipients...)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(plaintext); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Decrypt(blob []byte, identity *age.X25519Identity) ([]byte, error) {
	r, err := age.Decrypt(bytes.NewReader(blob), identity)
	if err != nil {
		var noMatch *age.NoIdentityMatchError
		if ok := errorsAs(err, &noMatch); ok {
			return nil, fmt.Errorf("this file is not encrypted for your key — ask a teammate to add you to recipients and run `envbridge team sync`")
		}
		return nil, err
	}
	return io.ReadAll(r)
}
