package diagnostics

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/utils"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
	"github.com/eclipse-iofog/agent-go/pkg/docker"
)

const (
	straceModuleName = "STrace Diagnostic Manager"
)

// StraceDiagnosticManager manages strace diagnostics for microservices
type StraceDiagnosticManager struct {
	monitoringMicroservices []*MicroserviceStraceData
	mu                      sync.RWMutex
	dockerClient            *docker.Client
}

var (
	straceInstance *StraceDiagnosticManager
	straceOnce     sync.Once
)

// GetStraceInstance returns the singleton StraceDiagnosticManager instance
func GetStraceInstance() *StraceDiagnosticManager {
	straceOnce.Do(func() {
		straceInstance = &StraceDiagnosticManager{
			monitoringMicroservices: make([]*MicroserviceStraceData, 0),
			dockerClient:            docker.GetInstance(),
		}
	})
	return straceInstance
}

// GetMonitoringMicroservices returns a copy of the monitoring microservices list
func (sdm *StraceDiagnosticManager) GetMonitoringMicroservices() []*MicroserviceStraceData {
	sdm.mu.RLock()
	defer sdm.mu.RUnlock()
	
	result := make([]*MicroserviceStraceData, len(sdm.monitoringMicroservices))
	copy(result, sdm.monitoringMicroservices)
	return result
}

// UpdateMonitoringMicroservices updates the list of microservices to monitor based on diagnostic data
// diagnosticData should be a map with "straceValues" key containing an array of maps with:
//   - "microserviceUuid": string
//   - "straceRun": bool
func (sdm *StraceDiagnosticManager) UpdateMonitoringMicroservices(diagnosticData map[string]interface{}) error {
	logging.LogDebug(straceModuleName, "Trying to update strace monitoring microservices")

	if diagnosticData == nil {
		return nil
	}

	straceValues, ok := diagnosticData["straceValues"].([]interface{})
	if !ok {
		return nil
	}

	for _, microserviceValue := range straceValues {
		microservice, ok := microserviceValue.(map[string]interface{})
		if !ok {
			continue
		}

		microserviceUUID, ok := microservice["microserviceUuid"].(string)
		if !ok {
			continue
		}

		straceRun, ok := microservice["straceRun"].(bool)
		if !ok {
			continue
		}

		sdm.manageMicroservice(microserviceUUID, straceRun)
	}

	logging.LogDebug(straceModuleName, "Finished update strace monitoring microservices")
	return nil
}

// manageMicroservice enables or disables strace for a microservice
func (sdm *StraceDiagnosticManager) manageMicroservice(microserviceUUID string, strace bool) {
	if strace {
		sdm.EnableMicroserviceStraceDiagnostics(microserviceUUID)
	} else {
		sdm.DisableMicroserviceStraceDiagnostics(microserviceUUID)
	}
}

// getStraceDataByMicroserviceUuid finds strace data for a microservice
func (sdm *StraceDiagnosticManager) getStraceDataByMicroserviceUuid(microserviceUUID string) *MicroserviceStraceData {
	sdm.mu.RLock()
	defer sdm.mu.RUnlock()

	for _, data := range sdm.monitoringMicroservices {
		if data.GetMicroserviceUUID() == microserviceUUID {
			return data
		}
	}
	return nil
}

// EnableMicroserviceStraceDiagnostics enables strace diagnostics for a microservice
func (sdm *StraceDiagnosticManager) EnableMicroserviceStraceDiagnostics(microserviceUUID string) error {
	logging.LogInfo(straceModuleName, fmt.Sprintf("Start enable microservice for strace diagnostics: %s", microserviceUUID))

	// Get container name
	containerName := utils.IOFogDockerContainerNamePrefix + microserviceUUID

	// Get PID by container name
	pid, err := sdm.getPidByContainerName(containerName)
	if err != nil {
		logging.LogError(straceModuleName, "Can't get pid of process", err)
		return fmt.Errorf("can't get pid of process: %w", err)
	}

	// Create new strace data
	newMicroserviceStraceData := NewMicroserviceStraceData(microserviceUUID, pid, true)

	// Remove old entry if exists
	sdm.mu.Lock()
	for i, oldData := range sdm.monitoringMicroservices {
		if oldData.GetMicroserviceUUID() == microserviceUUID {
			sdm.monitoringMicroservices = append(sdm.monitoringMicroservices[:i], sdm.monitoringMicroservices[i+1:]...)
			break
		}
	}
	sdm.monitoringMicroservices = append(sdm.monitoringMicroservices, newMicroserviceStraceData)
	sdm.mu.Unlock()

	// Start strace
	go sdm.runStrace(newMicroserviceStraceData)

	logging.LogInfo(straceModuleName, fmt.Sprintf("Finished enable microservice for strace diagnostics: %s", microserviceUUID))
	return nil
}

// DisableMicroserviceStraceDiagnostics disables strace diagnostics for a microservice
func (sdm *StraceDiagnosticManager) DisableMicroserviceStraceDiagnostics(microserviceUUID string) {
	logging.LogDebug(straceModuleName, fmt.Sprintf("Disabling microservice strace diagnostics for microservice: %s", microserviceUUID))

	sdm.mu.Lock()
	defer sdm.mu.Unlock()

	for i, data := range sdm.monitoringMicroservices {
		if data.GetMicroserviceUUID() == microserviceUUID {
			data.SetStraceRun(false)
			sdm.monitoringMicroservices = append(sdm.monitoringMicroservices[:i], sdm.monitoringMicroservices[i+1:]...)
			break
		}
	}
}

// getPidByContainerName gets the PID of the main process in a container
func (sdm *StraceDiagnosticManager) getPidByContainerName(containerName string) (int, error) {
	logging.LogDebug(straceModuleName, fmt.Sprintf("Start getting pid of microservice by container name: %s", containerName))

	// Execute "docker top" command
	command := fmt.Sprintf("docker top %s", containerName)
	stdout, stderr, err := utils.ExecuteCommand(command)
	if err != nil {
		return 0, fmt.Errorf("failed to execute docker top: %w", err)
	}

	if stderr != "" {
		logging.LogWarn(straceModuleName, fmt.Sprintf("docker top stderr: %s", stderr))
	}

	// Parse output - first line is header, second line is the process
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("unexpected docker top output: %s", stdout)
	}

	// Parse PID from second line (format: "UID PID PPID C STIME TTY TIME CMD")
	fields := strings.Fields(lines[1])
	if len(fields) < 2 {
		return 0, fmt.Errorf("unexpected docker top output format: %s", lines[1])
	}

	pid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, fmt.Errorf("failed to parse PID: %w", err)
	}

	logging.LogInfo(straceModuleName, fmt.Sprintf("Finished getting pid of microservice by container name: %d", pid))
	return pid, nil
}

// runStrace runs strace on a microservice process
func (sdm *StraceDiagnosticManager) runStrace(microserviceStraceData *MicroserviceStraceData) {
	logging.LogDebug(straceModuleName, "Start running strace")

	pid := microserviceStraceData.GetPid()
	straceCommand := fmt.Sprintf("strace -p %d", pid)

	// Create command
	cmd := exec.Command("/bin/sh", "-c", straceCommand)

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logging.LogError(straceModuleName, "Failed to get stdout pipe", err)
		return
	}

	// Start command
	if err := cmd.Start(); err != nil {
		logging.LogError(straceModuleName, "Failed to start strace command", err)
		return
	}

	// Read output in a goroutine
	go func() {
		defer stdout.Close()
		buf := make([]byte, 4096)
		line := ""
		
		for microserviceStraceData.GetStraceRun() {
			n, err := stdout.Read(buf)
			if err != nil {
				break
			}

			// Process buffer
			data := string(buf[:n])
			for _, char := range data {
				if char == '\n' {
					if line != "" {
						microserviceStraceData.AppendToResultBuffer(line)
						line = ""
					}
				} else {
					line += string(char)
				}
			}
		}

		// Kill strace process when done
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	// Monitor and kill orphaned strace processes
	go sdm.killOrphanedStraceProcesses()

	logging.LogDebug(straceModuleName, "Finished running strace")
}

// killOrphanedStraceProcesses kills orphaned strace processes
func (sdm *StraceDiagnosticManager) killOrphanedStraceProcesses() {
	logging.LogDebug(straceModuleName, "Killing orphaned strace processes")

	// Find all strace processes
	command := "pgrep strace"
	stdout, _, err := utils.ExecuteCommand(command)
	if err != nil {
		// No strace processes found, which is fine
		return
	}

	// Parse PIDs and kill them
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}

		// Kill the process
		killCommand := fmt.Sprintf("kill -9 %d", pid)
		_, _, err = utils.ExecuteCommand(killCommand)
		if err != nil {
			logging.LogWarn(straceModuleName, fmt.Sprintf("Failed to kill strace process %d: %v", pid, err))
		}
	}
}

// StartMonitoring starts the strace monitoring loop (called periodically)
func (sdm *StraceDiagnosticManager) StartMonitoring(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check for stopped strace processes and clean them up
			sdm.mu.Lock()
			activeMicroservices := make([]*MicroserviceStraceData, 0)
			for _, data := range sdm.monitoringMicroservices {
				if data.GetStraceRun() {
					activeMicroservices = append(activeMicroservices, data)
				}
			}
			sdm.monitoringMicroservices = activeMicroservices
			sdm.mu.Unlock()
		}
	}
}
