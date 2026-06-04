// Package token provides token lifecycle management with state machine and pool selection.
package token

import (
	"strconv"
	"strings"

	"github.com/javaswahid/grokpi/internal/config"
	"github.com/javaswahid/grokpi/internal/store"
)

// QuotaCategory represents a quota consumption category.
type QuotaCategory string

const (
	CategoryChat  QuotaCategory = "chat"
	CategoryImage QuotaCategory = "image"
	CategoryVideo QuotaCategory = "video"
)

// GetQuota returns the quota value for the given category.
func GetQuota(t *store.Token, cat QuotaCategory) int {
	switch cat {
	case CategoryImage:
		return t.ImageQuota
	case CategoryVideo:
		return t.VideoQuota
	default:
		return t.ChatQuota
	}
}

// SetQuota sets the quota value for the given category.
func SetQuota(t *store.Token, cat QuotaCategory, val int) {
	switch cat {
	case CategoryImage:
		t.ImageQuota = val
	case CategoryVideo:
		t.VideoQuota = val
	default:
		t.ChatQuota = val
	}
}

// NormalizeQuotaMode returns a supported quota mode, defaulting to limited.
func NormalizeQuotaMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case QuotaModeLimited, QuotaModeDailyReset, QuotaModeUnlimited, QuotaModeManual:
		return strings.TrimSpace(strings.ToLower(mode))
	default:
		return QuotaModeLimited
	}
}

// ValidQuotaMode checks if the given value is a supported token quota mode.
func ValidQuotaMode(mode string) bool {
	return NormalizeQuotaMode(mode) == strings.TrimSpace(strings.ToLower(mode))
}

// IsUnlimitedQuota returns true when local quota deduction should be skipped.
func IsUnlimitedQuota(t *store.Token, cfg *config.TokenConfig) bool {
	return t != nil && cfg != nil && cfg.AllowUnlimitedQuota && NormalizeQuotaMode(t.QuotaMode) == QuotaModeUnlimited
}

// ClampQuota applies configured max quotas unless unlimited quota is requested.
// A zero/negative max means no max cap for that category.
func ClampQuota(cat QuotaCategory, val int, cfg *config.TokenConfig, quotaMode string) int {
	if val < 0 {
		val = 0
	}
	if NormalizeQuotaMode(quotaMode) == QuotaModeUnlimited {
		return val
	}
	if cfg == nil {
		return val
	}
	max := maxQuotaForCategory(cat, cfg)
	if max > 0 && val > max {
		return max
	}
	return val
}

func maxQuotaForCategory(cat QuotaCategory, cfg *config.TokenConfig) int {
	if cfg == nil {
		return 0
	}
	switch cat {
	case CategoryImage:
		return cfg.MaxImageQuota
	case CategoryVideo:
		return cfg.MaxVideoQuota
	default:
		return cfg.MaxChatQuota
	}
}

// ParseModelEntry parses "model#cost" format, returning (modelName, cost).
// Without #cost suffix, cost defaults to 1.
func ParseModelEntry(entry string) (string, int) {
	if i := strings.LastIndex(entry, "#"); i > 0 {
		if c, err := strconv.Atoi(entry[i+1:]); err == nil && c > 0 {
			return entry[:i], c
		}
	}
	return entry, 1
}

// CostForModel looks up the cost for a model from basic_models/super_models lists.
func CostForModel(model string, cfg *config.TokenConfig) int {
	if cfg == nil {
		return 1
	}
	for _, entry := range cfg.BasicModels {
		name, cost := ParseModelEntry(entry)
		if name == model {
			return cost
		}
	}
	for _, entry := range cfg.SuperModels {
		name, cost := ParseModelEntry(entry)
		if name == model {
			return cost
		}
	}
	return 1
}
