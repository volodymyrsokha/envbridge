package config

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// AddRecipient appends a recipient to the file's recipients list through the
// yaml AST, so comments and the user's formatting survive — this file is
// committed and reviewed in PRs.
func AddRecipient(path string, r Recipient) error {
	doc, err := loadDoc(path)
	if err != nil {
		return err
	}
	seq, err := recipientsNode(doc)
	if err != nil {
		return err
	}
	for _, existing := range seq.Content {
		if mapValue(existing, "key") == r.Key {
			return fmt.Errorf("%s is already a recipient (%s)", mapValue(existing, "name"), r.Key)
		}
	}
	seq.Content = append(seq.Content, &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			scalar("name"), scalar(r.Name),
			scalar("email"), scalar(r.Email),
			scalar("key"), scalar(r.Key),
		},
	})
	return saveDoc(path, doc)
}

// RemoveRecipient deletes the recipient matching name or email (email
// case-insensitively) and returns what was removed.
func RemoveRecipient(path, nameOrEmail string) (Recipient, error) {
	doc, err := loadDoc(path)
	if err != nil {
		return Recipient{}, err
	}
	seq, err := recipientsNode(doc)
	if err != nil {
		return Recipient{}, err
	}
	for i, node := range seq.Content {
		r := Recipient{
			Name:  mapValue(node, "name"),
			Email: mapValue(node, "email"),
			Key:   mapValue(node, "key"),
		}
		if r.Name == nameOrEmail || strings.EqualFold(r.Email, nameOrEmail) {
			if len(seq.Content) == 1 {
				return Recipient{}, fmt.Errorf("cannot remove the last recipient — the environments would become undecryptable")
			}
			seq.Content = append(seq.Content[:i], seq.Content[i+1:]...)
			return r, saveDoc(path, doc)
		}
	}
	return Recipient{}, fmt.Errorf("no recipient named %q — check `envbridge team list`", nameOrEmail)
}

func loadDoc(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &doc, nil
}

func saveDoc(path string, doc *yaml.Node) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func recipientsNode(doc *yaml.Node) (*yaml.Node, error) {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("unexpected yaml document structure")
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "recipients" {
			seq := root.Content[i+1]
			if seq.Kind != yaml.SequenceNode {
				return nil, fmt.Errorf("recipients: is not a list")
			}
			return seq, nil
		}
	}
	return nil, fmt.Errorf("no recipients: section found")
}

func mapValue(node *yaml.Node, key string) string {
	if node.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1].Value
		}
	}
	return ""
}

func scalar(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: v}
}
