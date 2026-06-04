package token

import "strings"

// ErrPolicyQuarantine is returned when an operation would bypass a policy block.
var ErrPolicyQuarantine = &LifecycleError{Code: "policy_quarantine", Message: "token is quarantined after a policy violation; repair the prompt before reuse"}

// LifecycleError is a stable operational token lifecycle error.
type LifecycleError struct {
	Code    string
	Message string
}

func (e *LifecycleError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

// IsPolicyViolationReason detects provider safety/policy failures. It is
// intentionally conservative and only matches content-policy language, not
// ordinary token, transport, quota, or cooldown errors.
func IsPolicyViolationReason(reason string) bool {
	lower := strings.ToLower(strings.TrimSpace(reason))
	if lower == "" {
		return false
	}
	needles := []string{
		"policy violation",
		"policy_violation",
		"policy block",
		"policy_block",
		"safety violation",
		"safety filter",
		"safety refusal",
		"unsafe prompt",
		"prompt unsafe",
		"blocked content",
		"content blocked",
		"moderation failed",
		"moderation block",
		"responsible ai",
		"nsfw policy",
	}
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}
