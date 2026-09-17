package license

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"takealot/pkg/db"
)

type Status struct {
	Activated          bool   `json:"activated"`
	MachineID          string `json:"machine_id"`
	Customer           string `json:"customer,omitempty"`
	ExpiresAt          int64  `json:"expires_at"` // 0 = 永久
	ExpiresAtFormatted string `json:"expires_at_formatted"`
	Expired            bool   `json:"expired"`
	DaysLeft           int    `json:"days_left"`
	MaxStores          int    `json:"max_stores"`
	Message            string `json:"message"`
}

type Manager struct {
	mu        sync.RWMutex
	db        *db.DB
	machineID string
	status    Status
}

const (
	licenseKeyConfigName = "license_key"
	lastActiveConfigName = "last_active_time"
	licenseFileName      = ".license"
)

// NewManager initializes the license manager and checks for an existing license.
func NewManager(database *db.DB) *Manager {
	mid := GetMachineID()
	m := &Manager{
		db:        database,
		machineID: mid,
		status: Status{
			Activated: false,
			MachineID: mid,
			Message:   "软件未激活，请复制机器识别码获取激活码",
		},
	}

	// Try loading saved license
	rawKey := m.loadSavedLicenseKey()
	if rawKey != "" {
		_ = m.validateAndApply(rawKey, false)
	}

	// Anti-clock rollback check and heartbeat
	m.checkClockAndRecordHeartbeat()

	return m
}

func (m *Manager) MachineID() string {
	return m.machineID
}

func (m *Manager) GetStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Manager) IsValid() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status.Activated && !m.status.Expired
}

// Activate verifies and stores a new license key.
func (m *Manager) Activate(licenseKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.validateAndApply(licenseKey, true)
}

func (m *Manager) validateAndApply(licenseKey string, persist bool) error {
	licenseKey = strings.TrimSpace(licenseKey)
	if licenseKey == "" {
		return ErrInvalidFormat
	}

	payload, err := ParseAndVerify(licenseKey, m.machineID)
	if err != nil {
		m.status.Activated = false
		m.status.Expired = (err == ErrExpired)
		m.status.Message = err.Error()
		return err
	}

	// Calculate expiration details
	var daysLeft int = -1
	var expFormatted = "永久买断 (Permanent)"
	var isExpired = false

	if payload.ExpiresAt > 0 {
		now := time.Now().Unix()
		if now > payload.ExpiresAt {
			isExpired = true
			daysLeft = 0
		} else {
			daysLeft = int((payload.ExpiresAt - now) / 86400)
		}
		expFormatted = time.Unix(payload.ExpiresAt, 0).Format("2006-01-02 15:04:05")
	}

	m.status = Status{
		Activated:          !isExpired,
		MachineID:          m.machineID,
		Customer:           payload.Customer,
		ExpiresAt:          payload.ExpiresAt,
		ExpiresAtFormatted: expFormatted,
		Expired:            isExpired,
		DaysLeft:           daysLeft,
		MaxStores:          payload.MaxStores,
		Message:            "授权有效",
	}

	if isExpired {
		m.status.Message = fmt.Sprintf("软件授权已于 %s 到期", expFormatted)
	}

	if persist && !isExpired {
		m.saveLicenseKey(licenseKey)
		log.Printf("🔑 软件授权激活成功: 客户 [%s], 有效期至 [%s]", payload.Customer, expFormatted)
	}

	return nil
}

func (m *Manager) loadSavedLicenseKey() string {
	// 1. First try SQLite
	if m.db != nil {
		if val, err := m.db.GetSystemConfig(licenseKeyConfigName); err == nil && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}

	// 2. Fallback to local .license file
	if data, err := os.ReadFile(licenseFileName); err == nil {
		key := strings.TrimSpace(string(data))
		if key != "" {
			// Sync back to SQLite if available
			if m.db != nil {
				_ = m.db.SetSystemConfig(licenseKeyConfigName, key)
			}
			return key
		}
	}

	return ""
}

func (m *Manager) saveLicenseKey(key string) {
	// 1. Save to SQLite
	if m.db != nil {
		_ = m.db.SetSystemConfig(licenseKeyConfigName, key)
	}

	// 2. Save to local .license file as redundant backup
	_ = os.WriteFile(licenseFileName, []byte(key), 0644)
}

func (m *Manager) checkClockAndRecordHeartbeat() {
	now := time.Now().Unix()
	if m.db == nil {
		return
	}

	lastStr, err := m.db.GetSystemConfig(lastActiveConfigName)
	if err == nil && lastStr != "" {
		lastTime, err := strconv.ParseInt(lastStr, 10, 64)
		if err == nil {
			// Allow up to 1 hour backwards deviation for timezone/DST changes
			if now < lastTime-3600 {
				m.mu.Lock()
				m.status.Activated = false
				m.status.Expired = true
				m.status.Message = "检测到系统时钟异常回拨，软件已被锁定，请校准系统时间后重启"
				m.mu.Unlock()
				log.Printf("⚠️ 警告: 检测到系统时钟异常回拨 (当前时间: %d, 上次记录时间: %d)", now, lastTime)
				return
			}
		}
	}

	_ = m.db.SetSystemConfig(lastActiveConfigName, strconv.FormatInt(now, 10))
}
