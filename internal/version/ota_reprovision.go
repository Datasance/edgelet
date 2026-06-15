package version

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultOTAReprovisionPending = "/var/backups/edgelet/ota-reprovision-pending"
	otaExpirySkew                = 30 * time.Second
	otaPreflightRefreshBefore    = 3 * time.Minute
)

var otaReprovisionPendingPath = defaultOTAReprovisionPending

// PendingOTAReprovision holds controller OTA reprovision state written before install.sh runs.
type PendingOTAReprovision struct {
	ProvisionKey   string    `json:"provisionKey"`
	ExpirationTime time.Time `json:"expirationTime"`
	Command        string    `json:"command"`
	TargetVersion  string    `json:"targetVersion"`
}

// SetOTAReprovisionPendingPath overrides the pending file path (tests).
func SetOTAReprovisionPendingPath(path string) {
	if strings.TrimSpace(path) == "" {
		otaReprovisionPendingPath = defaultOTAReprovisionPending
		return
	}
	otaReprovisionPendingPath = path
}

// ParseExpirationTimeMS parses controller expirationTime as Unix epoch milliseconds.
func ParseExpirationTimeMS(raw any) (time.Time, error) {
	switch v := raw.(type) {
	case nil:
		return time.Time{}, errors.New("expirationTime is required")
	case float64:
		return time.UnixMilli(int64(v)), nil
	case int:
		return time.UnixMilli(int64(v)), nil
	case int64:
		return time.UnixMilli(v), nil
	case json.Number:
		ms, err := v.Int64()
		if err != nil {
			return time.Time{}, fmt.Errorf("expirationTime: %w", err)
		}
		return time.UnixMilli(ms), nil
	case string:
		ms, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("expirationTime: %w", err)
		}
		return time.UnixMilli(ms), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported expirationTime type %T", raw)
	}
}

// WriteOTAReprovisionPending persists pending reprovision state before detached install.sh.
func WriteOTAReprovisionPending(key, command, targetVersion string, expiry time.Time) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("provisionKey is required")
	}
	if expiry.IsZero() {
		return errors.New("expirationTime is required")
	}

	pending := PendingOTAReprovision{
		ProvisionKey:   key,
		ExpirationTime: expiry.UTC(),
		Command:        strings.ToLower(strings.TrimSpace(command)),
		TargetVersion:  strings.TrimSpace(targetVersion),
	}
	data, err := json.Marshal(pending)
	if err != nil {
		return err
	}

	dir := filepath.Dir(otaReprovisionPendingPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	tmp := otaReprovisionPendingPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, otaReprovisionPendingPath)
}

// ReadOTAReprovisionPending reads pending reprovision state if present.
func ReadOTAReprovisionPending() (*PendingOTAReprovision, error) {
	data, err := os.ReadFile(otaReprovisionPendingPath) // #nosec G304 -- fixed OTA pending path
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	var pending PendingOTAReprovision
	if err := json.Unmarshal(data, &pending); err != nil {
		return nil, err
	}
	if strings.TrimSpace(pending.ProvisionKey) == "" {
		return nil, errors.New("pending reprovision missing provisionKey")
	}
	return &pending, nil
}

// DeleteOTAReprovisionPending removes pending reprovision state.
func DeleteOTAReprovisionPending() error {
	err := os.Remove(otaReprovisionPendingPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// IsExpired reports whether the pending key is no longer valid (30s skew buffer).
func (p *PendingOTAReprovision) IsExpired(now time.Time) bool {
	if p == nil || p.ExpirationTime.IsZero() {
		return true
	}
	return now.After(p.ExpirationTime.Add(otaExpirySkew))
}

// NeedsPreflightRefresh reports whether the key is close enough to expiry to refresh before OTA.
func (p *PendingOTAReprovision) NeedsPreflightRefresh(now time.Time) bool {
	if p == nil || p.ExpirationTime.IsZero() {
		return false
	}
	return now.After(p.ExpirationTime.Add(-otaPreflightRefreshBefore))
}

// PendingFromAction builds pending state from a normalized version action map.
func PendingFromAction(actionData map[string]any) (*PendingOTAReprovision, error) {
	if actionData == nil {
		return nil, errors.New("action data is required")
	}
	key, ok := actionData["provisionKey"].(string)
	if !ok || strings.TrimSpace(key) == "" {
		return nil, errors.New("provisionKey is required")
	}
	expiry, err := ParseExpirationTimeMS(actionData["expirationTime"])
	if err != nil {
		return nil, err
	}
	cmd, ok := actionData["command"].(string)
	if !ok {
		cmd = ""
	}
	return &PendingOTAReprovision{
		ProvisionKey:   key,
		ExpirationTime: expiry.UTC(),
		Command:        strings.ToLower(strings.TrimSpace(cmd)),
		TargetVersion:  TargetVersionFromAction(actionData),
	}, nil
}
