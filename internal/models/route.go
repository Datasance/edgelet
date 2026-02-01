package models

import (
	"sort"
)

// Route represents microservice routings
type Route struct {
	Receivers []string `json:"receivers" yaml:"receivers"`
}

// NewRoute creates a new Route with empty receivers list
func NewRoute() *Route {
	return &Route{
		Receivers: make([]string, 0),
	}
}

// SetReceivers sets the receivers list and sorts it
func (r *Route) SetReceivers(receivers []string) {
	r.Receivers = make([]string, len(receivers))
	copy(r.Receivers, receivers)
	sort.Strings(r.Receivers)
}

// Equals checks if two Routes are equal
func (r *Route) Equals(other *Route) bool {
	if other == nil {
		return false
	}
	if len(r.Receivers) != len(other.Receivers) {
		return false
	}
	for i := range r.Receivers {
		if r.Receivers[i] != other.Receivers[i] {
			return false
		}
	}
	return true
}
