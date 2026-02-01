package fieldagent

import (
	"sync"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/models"
)

// State represents the current state of the FieldAgent
type State struct {
	mu                 sync.RWMutex
	lastGetChangesList int64
	initialization     bool
	connected          bool
	controllerStatus   models.ControllerStatus
	lastCommandTime    int64
	controllerVerified bool
}

// NewState creates a new State instance
func NewState() *State {
	return &State{
		initialization:     true,
		connected:          false,
		controllerStatus:   models.ControllerStatusNotConnected,
		controllerVerified: false,
	}
}

// GetLastGetChangesList returns the last time changes were retrieved
func (s *State) GetLastGetChangesList() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastGetChangesList
}

// SetLastGetChangesList sets the last time changes were retrieved
func (s *State) SetLastGetChangesList(timestamp int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastGetChangesList = timestamp
}

// IsInitialization returns whether we're in initialization phase
func (s *State) IsInitialization() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.initialization
}

// SetInitialization sets the initialization flag
func (s *State) SetInitialization(init bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initialization = init
}

// IsConnected returns whether we're connected to the controller
func (s *State) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected
}

// SetConnected sets the connected status
func (s *State) SetConnected(connected bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = connected
}

// GetControllerStatus returns the controller status
func (s *State) GetControllerStatus() models.ControllerStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.controllerStatus
}

// SetControllerStatus sets the controller status
func (s *State) SetControllerStatus(status models.ControllerStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.controllerStatus = status
}

// GetLastCommandTime returns the last command time
func (s *State) GetLastCommandTime() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastCommandTime
}

// SetLastCommandTime sets the last command time
func (s *State) SetLastCommandTime(timestamp int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCommandTime = timestamp
}

// IsControllerVerified returns whether the controller is verified
func (s *State) IsControllerVerified() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.controllerVerified
}

// SetControllerVerified sets the controller verified flag
func (s *State) SetControllerVerified(verified bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.controllerVerified = verified
}

// UpdateLastCommandTime updates the last command time to current time
func (s *State) UpdateLastCommandTime() {
	s.SetLastCommandTime(time.Now().UnixMilli())
}
