package models

// EdgeletAPIStatus represents the Edgelet API status
type EdgeletAPIStatus struct {
	OpenConfigSocketsCount  int `json:"openConfigSocketsCount" yaml:"openConfigSocketsCount"`   // Number of open control/config sockets
	OpenMessageSocketsCount int `json:"openMessageSocketsCount" yaml:"openMessageSocketsCount"` // Number of open message sockets
}

// NewEdgeletAPIStatus creates a new EdgeletAPIStatus
func NewEdgeletAPIStatus() *EdgeletAPIStatus {
	return &EdgeletAPIStatus{}
}

// SetOpenConfigSocketsCount sets the open config sockets count and returns the status for chaining
func (l *EdgeletAPIStatus) SetOpenConfigSocketsCount(count int) *EdgeletAPIStatus {
	l.OpenConfigSocketsCount = count
	return l
}

// SetOpenMessageSocketsCount sets the open message sockets count and returns the status for chaining
func (l *EdgeletAPIStatus) SetOpenMessageSocketsCount(count int) *EdgeletAPIStatus {
	l.OpenMessageSocketsCount = count
	return l
}
