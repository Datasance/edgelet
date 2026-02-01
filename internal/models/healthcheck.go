package models

// Healthcheck represents container healthcheck configuration
type Healthcheck struct {
	Test          []string `json:"test" yaml:"test"`
	Interval      *int64   `json:"interval,omitempty" yaml:"interval,omitempty"`
	Timeout       *int64   `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	StartPeriod   *int64   `json:"startPeriod,omitempty" yaml:"startPeriod,omitempty"`
	StartInterval *int64   `json:"startInterval,omitempty" yaml:"startInterval,omitempty"`
	Retries       *int     `json:"retries,omitempty" yaml:"retries,omitempty"`
}

// NewHealthcheck creates a new Healthcheck
func NewHealthcheck(test []string, interval, timeout, startPeriod, startInterval *int64, retries *int) *Healthcheck {
	return &Healthcheck{
		Test:          test,
		Interval:      interval,
		Timeout:       timeout,
		StartPeriod:   startPeriod,
		StartInterval: startInterval,
		Retries:       retries,
	}
}

// Equals checks if two Healthchecks are equal
func (h *Healthcheck) Equals(other *Healthcheck) bool {
	if other == nil {
		return false
	}

	// Compare test arrays
	if len(h.Test) != len(other.Test) {
		return false
	}
	for i := range h.Test {
		if h.Test[i] != other.Test[i] {
			return false
		}
	}

	// Compare optional fields
	if !compareInt64Ptr(h.Interval, other.Interval) {
		return false
	}
	if !compareInt64Ptr(h.Timeout, other.Timeout) {
		return false
	}
	if !compareInt64Ptr(h.StartPeriod, other.StartPeriod) {
		return false
	}
	if !compareInt64Ptr(h.StartInterval, other.StartInterval) {
		return false
	}
	if !compareIntPtr(h.Retries, other.Retries) {
		return false
	}

	return true
}

// Helper functions for comparing pointers
func compareInt64Ptr(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func compareIntPtr(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
