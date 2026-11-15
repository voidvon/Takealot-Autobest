package account

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"takealot/pkg/localstate"
)

type FileStore struct{ path string }

func credentialKey(base, group string) string {
	sum := sha256.Sum256([]byte(strings.TrimRight(base, "/") + "|" + group))
	return hex.EncodeToString(sum[:])
}

// NewFileStore keeps the credential portable and scoped to this directory and
// member service. No machine key, absolute path, or system keyring is required.
func NewFileStore(root, base, group string) *FileStore {
	return &FileStore{path: filepath.Join(root, credentialKey(base, group)+".json")}
}

func (s *FileStore) Load() (string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return "", err
	}
	var record struct {
		Token string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return "", err
	}
	return record.Token, nil
}

func (s *FileStore) Save(token string) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(struct {
		Token string `json:"access_token"`
	}{token})
	if err != nil {
		return err
	}
	return localstate.WriteFile(s.path, data, 0600)
}

func (s *FileStore) Delete() error {
	err := os.Remove(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
