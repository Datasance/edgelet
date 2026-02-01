package models

// LocalApiStatus represents the Local API status
type LocalApiStatus struct {
	OpenConfigSocketsCount  int `json:"openConfigSocketsCount" yaml:"openConfigSocketsCount"`   // Number of open control/config sockets
	OpenMessageSocketsCount int `json:"openMessageSocketsCount" yaml:"openMessageSocketsCount"` // Number of open message sockets
}

// NewLocalApiStatus creates a new LocalApiStatus
func NewLocalApiStatus() *LocalApiStatus {
	return &LocalApiStatus{}
}

// SetOpenConfigSocketsCount sets the open config sockets count and returns the status for chaining
func (l *LocalApiStatus) SetOpenConfigSocketsCount(count int) *LocalApiStatus {
	l.OpenConfigSocketsCount = count
	return l
}

// SetOpenMessageSocketsCount sets the open message sockets count and returns the status for chaining
func (l *LocalApiStatus) SetOpenMessageSocketsCount(count int) *LocalApiStatus {
	l.OpenMessageSocketsCount = count
	return l
}
