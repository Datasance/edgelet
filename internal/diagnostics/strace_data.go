package diagnostics

import (
	"strings"
	"sync"
	"sync/atomic"
)

// MicroserviceStraceData holds strace monitoring data for a microservice
type MicroserviceStraceData struct {
	microserviceUUID string
	pid              int32
	straceRun        atomic.Bool
	resultBuffer     []string
	mu               sync.RWMutex
}

// NewMicroserviceStraceData creates a new MicroserviceStraceData instance
func NewMicroserviceStraceData(microserviceUUID string, pid int, straceRun bool) *MicroserviceStraceData {
	data := &MicroserviceStraceData{
		microserviceUUID: microserviceUUID,
		pid:              int32(pid),
		resultBuffer:     make([]string, 0),
	}
	data.straceRun.Store(straceRun)
	return data
}

// GetMicroserviceUUID returns the microservice UUID
func (m *MicroserviceStraceData) GetMicroserviceUUID() string {
	return m.microserviceUUID
}

// GetPid returns the process ID
func (m *MicroserviceStraceData) GetPid() int {
	return int(atomic.LoadInt32(&m.pid))
}

// SetPid sets the process ID
func (m *MicroserviceStraceData) SetPid(pid int) {
	atomic.StoreInt32(&m.pid, int32(pid))
}

// GetStraceRun returns whether strace is running
func (m *MicroserviceStraceData) GetStraceRun() bool {
	return m.straceRun.Load()
}

// SetStraceRun sets whether strace is running
func (m *MicroserviceStraceData) SetStraceRun(run bool) {
	m.straceRun.Store(run)
}

// GetResultBuffer returns a copy of the result buffer
func (m *MicroserviceStraceData) GetResultBuffer() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make([]string, len(m.resultBuffer))
	copy(result, m.resultBuffer)
	return result
}

// AppendToResultBuffer appends a line to the result buffer
func (m *MicroserviceStraceData) AppendToResultBuffer(line string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resultBuffer = append(m.resultBuffer, line)
}

// ClearResultBuffer clears the result buffer
func (m *MicroserviceStraceData) ClearResultBuffer() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resultBuffer = m.resultBuffer[:0]
}

// GetResultBufferAsString returns the result buffer as a string
func (m *MicroserviceStraceData) GetResultBufferAsString() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return strings.Join(m.resultBuffer, "\n")
}
