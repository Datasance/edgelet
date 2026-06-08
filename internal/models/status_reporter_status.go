package models

import "time"

// StatusReporterStatus represents the Status Reporter status
type StatusReporterStatus struct {
	SystemTime int64 `json:"systemTime" yaml:"systemTime"`
	LastUpdate int64 `json:"lastUpdate" yaml:"lastUpdate"`
}

// NewStatusReporterStatus creates a new StatusReporterStatus with current time
func NewStatusReporterStatus() *StatusReporterStatus {
	nowMs := time.Now().UnixMilli()
	return &StatusReporterStatus{
		SystemTime: nowMs,
		LastUpdate: nowMs,
	}
}

// NewStatusReporterStatusWithTimes creates a new StatusReporterStatus with specified times
func NewStatusReporterStatusWithTimes(systemTime, lastUpdate int64) *StatusReporterStatus {
	return &StatusReporterStatus{
		SystemTime: systemTime,
		LastUpdate: lastUpdate,
	}
}

// SetSystemTime sets the system time and returns the status for chaining
func (s *StatusReporterStatus) SetSystemTime(systemTime int64) *StatusReporterStatus {
	s.SystemTime = systemTime
	return s
}

// SetLastUpdate sets the last update time and returns the status for chaining
func (s *StatusReporterStatus) SetLastUpdate(lastUpdate int64) *StatusReporterStatus {
	s.LastUpdate = lastUpdate
	return s
}
