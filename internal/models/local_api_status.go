package models

// LocalAPIStatus represents the Local API status
type LocalAPIStatus struct {
	OpenConfigSocketsCount  int `json:"openConfigSocketsCount" yaml:"openConfigSocketsCount"`   // Number of open control/config sockets
	OpenMessageSocketsCount int `json:"openMessageSocketsCount" yaml:"openMessageSocketsCount"` // Number of open message sockets
}

// NewLocalAPIStatus creates a new LocalAPIStatus
func NewLocalAPIStatus() *LocalAPIStatus {
	return &LocalAPIStatus{}
}

// SetOpenConfigSocketsCount sets the open config sockets count and returns the status for chaining
func (l *LocalAPIStatus) SetOpenConfigSocketsCount(count int) *LocalAPIStatus {
	l.OpenConfigSocketsCount = count
	return l
}

// SetOpenMessageSocketsCount sets the open message sockets count and returns the status for chaining
func (l *LocalAPIStatus) SetOpenMessageSocketsCount(count int) *LocalAPIStatus {
	l.OpenMessageSocketsCount = count
	return l
}
