package models

// PortMapping represents microservice port mappings
type PortMapping struct {
	Outside int  `json:"outside" yaml:"outside"`
	Inside  int  `json:"inside" yaml:"inside"`
	UDP     bool `json:"udp" yaml:"udp"`
}

// NewPortMapping creates a new PortMapping
func NewPortMapping(outside, inside int, udp bool) *PortMapping {
	return &PortMapping{
		Outside: outside,
		Inside:  inside,
		UDP:     udp,
	}
}

// Equals checks if two PortMappings are equal
func (p *PortMapping) Equals(other *PortMapping) bool {
	if other == nil {
		return false
	}
	return p.Outside == other.Outside && p.Inside == other.Inside
}

// CompareTo compares two PortMappings (for sorting)
// Returns: negative if p < other, zero if equal, positive if p > other
func (p *PortMapping) CompareTo(other *PortMapping) int {
	if p.Inside == other.Inside {
		return p.Outside - other.Outside
	}
	return p.Inside - other.Inside
}
