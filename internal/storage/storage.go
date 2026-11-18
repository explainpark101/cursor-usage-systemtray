// Package storage persists app configuration and the encrypted session
// cookie on disk.
package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Tray percent / style identifiers stored in config.json.
const (
	TrayPercentTotal = "total"
	TrayPercentAuto  = "auto"
	TrayPercentAPI   = "api"

	TrayStyleBar     = "bar"
	TrayStyleCircle  = "circle"
	TrayStyleBattery = "battery"
	TrayStyleNumber  = "number"
	TrayStyleColored = "colored"
)

// Config holds user settings.
type Config struct {
	PollIntervalSeconds int    `json:"pollIntervalSeconds"` // default 10
	AutoStart           bool   `json:"autoStart"`
	TrayPercent         string `json:"trayPercent"` // total|auto|api
	TrayStyle           string `json:"trayStyle"`   // bar|circle|battery|number|colored
}

// Defaults for Config.
var DefaultConfig = Config{
	PollIntervalSeconds: 10,
	AutoStart:           false,
	TrayPercent:         TrayPercentTotal,
	TrayStyle:           TrayStyleBar,
}

// Store manages on-disk state.
type Store struct {
	Dir string

	mu     sync.RWMutex
	config Config
	cookie string
}

func dir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(base, "cursor-tray"), nil
}

func normalizeConfig(c *Config) {
	if c.PollIntervalSeconds <= 0 {
		c.PollIntervalSeconds = DefaultConfig.PollIntervalSeconds
	}
	switch c.TrayPercent {
	case TrayPercentTotal, TrayPercentAuto, TrayPercentAPI:
	default:
		c.TrayPercent = DefaultConfig.TrayPercent
	}
	switch c.TrayStyle {
	case TrayStyleBar, TrayStyleCircle, TrayStyleBattery, TrayStyleNumber, TrayStyleColored:
	default:
		c.TrayStyle = DefaultConfig.TrayStyle
	}
}

// Open loads (or initializes) the store under %LOCALAPPDATA%\cursor-tray.
func Open() (*Store, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return nil, err
	}

	s := &Store{Dir: d, config: DefaultConfig}

	if b, err := os.ReadFile(filepath.Join(d, "config.json")); err == nil {
		_ = json.Unmarshal(b, &s.config)
		normalizeConfig(&s.config)
	}

	if b, err := os.ReadFile(filepath.Join(d, "cookies.bin")); err == nil {
		if plain, err := decrypt(b); err == nil {
			s.cookie = string(plain)
		}
	}
	return s, nil
}

// Config returns the current configuration.
func (s *Store) Config() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// SetConfig persists configuration changes.
func (s *Store) SetConfig(c Config) error {
	normalizeConfig(&c)
	s.mu.Lock()
	s.config = c
	s.mu.Unlock()
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.Dir, "config.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.Dir, "config.json"))
}

// Cookie returns the stored cookie header value (may be empty).
func (s *Store) Cookie() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cookie
}

// SetCookie encrypts and persists the cookie header value. Empty clears it.
func (s *Store) SetCookie(v string) error {
	s.mu.Lock()
	s.cookie = v
	s.mu.Unlock()

	path := filepath.Join(s.Dir, "cookies.bin")
	if v == "" {
		_ = os.Remove(path)
		return nil
	}

	enc, err := encrypt([]byte(v))
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, enc, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
