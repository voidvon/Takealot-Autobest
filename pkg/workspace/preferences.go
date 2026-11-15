package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"takealot/pkg/localstate"
)

type Preferences struct {
	Theme         string `json:"theme"`
	ActiveStoreID string `json:"active_store_id"`
}

type PreferencesStore struct {
	mu    sync.Mutex
	path  string
	value Preferences
}

func NewPreferencesStore(root string) (*PreferencesStore, error) {
	s := &PreferencesStore{path: filepath.Join(root, "preferences.json"), value: Preferences{Theme: "system"}}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &s.value); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *PreferencesStore) Get() Preferences {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value
}

func (s *PreferencesStore) Update(theme, storeID *string) (Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.value
	if theme != nil {
		if *theme != "system" && *theme != "light" && *theme != "dark" {
			return s.value, errors.New("不支持的主题")
		}
		next.Theme = *theme
	}
	if storeID != nil {
		if len(*storeID) > 128 {
			return s.value, errors.New("店铺标识过长")
		}
		next.ActiveStoreID = *storeID
	}
	data, err := json.Marshal(next)
	if err != nil {
		return s.value, err
	}
	if err := localstate.WriteFile(s.path, data, 0600); err != nil {
		return s.value, err
	}
	s.value = next
	return s.value, nil
}
