package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

const (
	ConfigFile    = "config.json"
	LegacyCanIni  = "can.ini"
	LegacyGJData  = "GJDATA"
)

type Target struct {
	Selected bool `json:"selected"`
	MinPrice int  `json:"min_price"`
	MaxPrice int  `json:"max_price"`
}

type Config struct {
	Authorization     string            `json:"authorization"`
	PriceDecreaseStep int               `json:"price_decrease_step"`
	PriceIncreaseStep int               `json:"price_increase_step"`
	RRPPercentage     float64           `json:"rrp_percentage"`
	IntervalMinutes   int               `json:"interval_minutes"`
	BulkStock         int               `json:"bulk_stock"`
	MaxFetchOffers    int               `json:"max_fetch_offers"`
	Targets           map[string]Target `json:"targets"`
}

type Manager struct {
	mu     sync.RWMutex
	cfg    Config
	file   string
	baseDir string
}

func NewManager(baseDir string) *Manager {
	m := &Manager{
		file:    ConfigFile,
		baseDir: baseDir,
		cfg: Config{
			PriceDecreaseStep: 1,
			PriceIncreaseStep: 1,
			RRPPercentage:     120.0,
			IntervalMinutes:   5,
			BulkStock:         1,
			MaxFetchOffers:    1000,
			Targets:           make(map[string]Target),
		},
	}
	m.Load()
	return m
}

func (m *Manager) Load() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Try config.json
	data, err := os.ReadFile(ConfigFile)
	if err == nil {
		var loaded Config
		if err := json.Unmarshal(data, &loaded); err == nil {
			m.cfg = loaded
			if m.cfg.Targets == nil {
				m.cfg.Targets = make(map[string]Target)
			}
			return
		}
	}

	// 2. Fallback to can.ini
	if iniData, err := os.ReadFile(LegacyCanIni); err == nil {
		lines := strings.Split(string(iniData), "\n")
		if len(lines) >= 7 {
			if v, err := strconv.Atoi(strings.TrimSpace(lines[2])); err == nil {
				m.cfg.BulkStock = v
			}
			if v, err := strconv.Atoi(strings.TrimSpace(lines[3])); err == nil {
				m.cfg.PriceIncreaseStep = v
			}
			if v, err := strconv.ParseFloat(strings.TrimSpace(lines[4]), 64); err == nil {
				m.cfg.RRPPercentage = v
			}
			if v, err := strconv.Atoi(strings.TrimSpace(lines[5])); err == nil {
				m.cfg.PriceDecreaseStep = v
			}
			if v, err := strconv.Atoi(strings.TrimSpace(lines[6])); err == nil {
				m.cfg.IntervalMinutes = v
			}
		}
	}

	// 3. Fallback to GJDATA
	if gjData, err := os.ReadFile(LegacyGJData); err == nil {
		content := strings.TrimSpace(string(gjData))
		if len(content) > 0 {
			items := strings.Split(content, "#")
			for _, item := range items {
				parts := strings.Split(strings.TrimSpace(item), "/")
				if len(parts) >= 5 {
					key := fmt.Sprintf("%s/%s", parts[0], parts[1])
					minP, _ := strconv.Atoi(parts[3])
					maxP, _ := strconv.Atoi(parts[4])
					m.cfg.Targets[key] = Target{
						Selected: parts[2] == "1",
						MinPrice: minP,
						MaxPrice: maxP,
					}
				}
			}
		}
	}

	// Save migrated config immediately
	m.saveLocked()
}

func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Clone targets
	targetsCopy := make(map[string]Target, len(m.cfg.Targets))
	for k, v := range m.cfg.Targets {
		targetsCopy[k] = v
	}
	res := m.cfg
	res.Targets = targetsCopy
	return res
}

func (m *Manager) Update(newCfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cfg.Authorization = newCfg.Authorization
	if newCfg.PriceDecreaseStep > 0 {
		m.cfg.PriceDecreaseStep = newCfg.PriceDecreaseStep
	}
	if newCfg.PriceIncreaseStep > 0 {
		m.cfg.PriceIncreaseStep = newCfg.PriceIncreaseStep
	}
	if newCfg.RRPPercentage >= 100 {
		m.cfg.RRPPercentage = newCfg.RRPPercentage
	}
	if newCfg.IntervalMinutes > 0 {
		m.cfg.IntervalMinutes = newCfg.IntervalMinutes
	}
	if newCfg.BulkStock > 0 {
		m.cfg.BulkStock = newCfg.BulkStock
	}
	if newCfg.MaxFetchOffers > 0 {
		m.cfg.MaxFetchOffers = newCfg.MaxFetchOffers
	}
	return m.saveLocked()
}

func (m *Manager) UpdateTargets(newTargets map[string]Target) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for k, v := range newTargets {
		m.cfg.Targets[k] = v
	}
	return m.saveLocked()
}

func (m *Manager) saveLocked() error {
	data, err := json.MarshalIndent(m.cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(ConfigFile, data, 0644); err != nil {
		return err
	}

	// Sync back to GJDATA for backwards compatibility
	var lines []string
	for k, v := range m.cfg.Targets {
		parts := strings.Split(k, "/")
		if len(parts) == 2 {
			sel := "0"
			if v.Selected {
				sel = "1"
			}
			lines = append(lines, fmt.Sprintf("%s/%s/%s/%d/%d", parts[0], parts[1], sel, v.MinPrice, v.MaxPrice))
		}
	}
	if len(lines) > 0 {
		_ = os.WriteFile(LegacyGJData, []byte(strings.Join(lines, "#")), 0644)
	}
	return nil
}
