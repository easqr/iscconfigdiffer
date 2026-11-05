package v2

import (
	"errors"
	"strings"
)

// RuleError identifies the rules involved in a compiler validation failure.
// Adapters can attach provenance without interpreting human-readable error text.
// It carries no filesystem information and unwraps to the original error.
type RuleError struct {
	RuleIDs []string
	Err     error
}

func (e *RuleError) Error() string {
	return e.Err.Error() + " (rules: " + strings.Join(e.RuleIDs, ", ") + ")"
}
func (e *RuleError) Unwrap() error { return e.Err }

func ruleError(err error, ids ...string) error {
	if err == nil {
		return nil
	}
	var existing *RuleError
	if errors.As(err, &existing) {
		return err
	}
	unique := map[string]bool{}
	for _, id := range ids {
		if id != "" {
			unique[id] = true
		}
	}
	ordered := keys(unique)
	return &RuleError{RuleIDs: ordered, Err: err}
}

// ValidRuleID reports whether id satisfies the neutral policy identifier syntax.
func ValidRuleID(id string) bool { return validID.MatchString(id) }
