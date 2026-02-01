package models

// ModuleStatus represents the status of a module
type ModuleStatus string

const (
	ModuleStatusStarting ModuleStatus = "STARTING"
	ModuleStatusRunning  ModuleStatus = "RUNNING"
	ModuleStatusStopped  ModuleStatus = "STOPPED"
	ModuleStatusWarning  ModuleStatus = "WARNING"
	ModuleStatusUnknown  ModuleStatus = "UNKNOWN"
)

// SupervisorStatus represents the Supervisor status
type SupervisorStatus struct {
	DaemonStatus      ModuleStatus   `json:"daemonStatus" yaml:"daemonStatus"`           // Daemon status
	ModulesStatus     []ModuleStatus `json:"modulesStatus" yaml:"modulesStatus"`       // Status of each module
	DaemonLastStart   int64          `json:"daemonLastStart" yaml:"daemonLastStart"`   // Timestamp of last start
	OperationDuration int64          `json:"operationDuration" yaml:"operationDuration"` // Operating duration in milliseconds
	WarningMessage    string         `json:"warningMessage" yaml:"warningMessage"`      // Warning message
}

// NewSupervisorStatus creates a new SupervisorStatus with default values
func NewSupervisorStatus(numModules int) *SupervisorStatus {
	modulesStatus := make([]ModuleStatus, numModules)
	for i := range modulesStatus {
		modulesStatus[i] = ModuleStatusStarting
	}
	return &SupervisorStatus{
		ModulesStatus: modulesStatus,
	}
}

// SetModuleStatus sets the status of a module and returns the status for chaining
func (s *SupervisorStatus) SetModuleStatus(moduleIndex int, status ModuleStatus) *SupervisorStatus {
	if moduleIndex >= 0 && moduleIndex < len(s.ModulesStatus) {
		s.ModulesStatus[moduleIndex] = status
	}
	return s
}

// GetModuleStatus gets the status of a module
func (s *SupervisorStatus) GetModuleStatus(moduleIndex int) ModuleStatus {
	if moduleIndex >= 0 && moduleIndex < len(s.ModulesStatus) {
		return s.ModulesStatus[moduleIndex]
	}
	return ModuleStatusUnknown
}

// SetDaemonStatus sets the daemon status and returns the status for chaining
func (s *SupervisorStatus) SetDaemonStatus(status ModuleStatus) *SupervisorStatus {
	s.DaemonStatus = status
	return s
}

// SetDaemonLastStart sets the daemon last start time and returns the status for chaining
func (s *SupervisorStatus) SetDaemonLastStart(timestamp int64) *SupervisorStatus {
	s.DaemonLastStart = timestamp
	return s
}

// SetOperationDuration sets the operation duration and returns the status for chaining
func (s *SupervisorStatus) SetOperationDuration(duration int64) *SupervisorStatus {
	s.OperationDuration = duration
	return s
}

// GetOperationDuration returns the operation duration (current time - last start)
func (s *SupervisorStatus) GetOperationDuration() int64 {
	if s.DaemonLastStart > 0 {
		duration := s.OperationDuration - s.DaemonLastStart
		if duration >= 0 {
			return duration
		}
	}
	return 0
}

// SetWarningMessage sets the warning message and returns the status for chaining
func (s *SupervisorStatus) SetWarningMessage(message string) *SupervisorStatus {
	s.WarningMessage = message
	return s
}
