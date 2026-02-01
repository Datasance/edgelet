package utils

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	localLogReaderModuleName = "LocalLogReader"
	logFilePattern           = "iofog-agent.%d.log"
	latestLogFile            = "iofog-agent.0.log"
)

// LocalLogHandler is the interface for handling log lines
type LocalLogHandler interface {
	OnLogLine(sessionID, iofogUUID, line string)
	OnComplete(sessionID string)
	OnError(sessionID string, err error)
}

// TailConfig represents configuration for tailing logs
type TailConfig struct {
	Follow bool   `json:"follow"`
	Lines  int    `json:"lines"`
	Since  string `json:"since,omitempty"`
	Until  string `json:"until,omitempty"`
}

// LocalLogReader reads and streams agent self logs from local log files
type LocalLogReader struct {
	sessionID    string
	iofogUUID    string
	tailConfig   *TailConfig
	handler      LocalLogHandler
	isRunning    bool
	mu           sync.Mutex
	stopChan     chan struct{}
	logDirectory string
	currentLogFile string
}

// NewLocalLogReader creates a new LocalLogReader
func NewLocalLogReader(sessionID, iofogUUID, logDirectory string, tailConfig *TailConfig, handler LocalLogHandler) *LocalLogReader {
	return &LocalLogReader{
		sessionID:     sessionID,
		iofogUUID:     iofogUUID,
		tailConfig:   tailConfig,
		handler:      handler,
		isRunning:    false,
		stopChan:     make(chan struct{}),
		logDirectory: logDirectory,
		currentLogFile: filepath.Join(logDirectory, latestLogFile),
	}
}

// Start starts reading logs
func (llr *LocalLogReader) Start() {
	llr.mu.Lock()
	defer llr.mu.Unlock()
	
	if llr.isRunning {
		return
	}
	
	llr.isRunning = true
	go llr.readLogs()
	logging.LogInfo(localLogReaderModuleName, fmt.Sprintf("Started LocalLogReader: sessionId=%s", llr.sessionID))
}

// Stop stops reading logs
func (llr *LocalLogReader) Stop() {
	llr.mu.Lock()
	defer llr.mu.Unlock()
	
	if !llr.isRunning {
		return
	}
	
	llr.isRunning = false
	close(llr.stopChan)
	logging.LogInfo(localLogReaderModuleName, fmt.Sprintf("Stopped LocalLogReader: sessionId=%s", llr.sessionID))
}

// IsRunning returns whether the reader is running
func (llr *LocalLogReader) IsRunning() bool {
	llr.mu.Lock()
	defer llr.mu.Unlock()
	return llr.isRunning
}

// readLogs reads logs based on tail configuration
func (llr *LocalLogReader) readLogs() {
	defer func() {
		if llr.handler != nil {
			llr.handler.OnComplete(llr.sessionID)
		}
	}()
	
	// Parse tail config
	follow := true
	lines := 100
	if llr.tailConfig != nil {
		follow = llr.tailConfig.Follow
		if llr.tailConfig.Lines > 0 {
			lines = llr.tailConfig.Lines
		}
		// Validate lines
		if lines < 1 {
			lines = 100
		}
		if lines > 10000 {
			lines = 10000
		}
	}
	
	logging.LogDebug(localLogReaderModuleName, fmt.Sprintf("Reading logs: follow=%v, lines=%d", follow, lines))
	
	// Check if log file exists
	if _, err := os.Stat(llr.currentLogFile); os.IsNotExist(err) {
		logging.LogWarn(localLogReaderModuleName, fmt.Sprintf("Log file does not exist: %s", llr.currentLogFile))
		if llr.handler != nil {
			llr.handler.OnError(llr.sessionID, fmt.Errorf("log file not found: %s", llr.currentLogFile))
		}
		return
	}
	
	// Read initial lines (tail)
	initialLines, err := llr.readTailLines(llr.currentLogFile, lines)
	if err != nil {
		logging.LogError(localLogReaderModuleName, "Error reading tail lines", err)
		if llr.handler != nil {
			llr.handler.OnError(llr.sessionID, err)
		}
		return
	}
	
	// Send initial lines
	for _, line := range initialLines {
		select {
		case <-llr.stopChan:
			return
		default:
			if llr.handler != nil {
				llr.handler.OnLogLine(llr.sessionID, llr.iofogUUID, line)
			}
		}
	}
	
	// If follow is true, watch for new lines
	if follow {
		llr.watchForNewLines(llr.currentLogFile)
	}
}

// readTailLines reads the last N lines from a file
func (llr *LocalLogReader) readTailLines(filePath string, lines int) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	// Read file line by line into a slice
	var allLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	
	// Return last N lines
	start := len(allLines) - lines
	if start < 0 {
		start = 0
	}
	
	return allLines[start:], nil
}

// watchForNewLines watches for new lines in the log file
func (llr *LocalLogReader) watchForNewLines(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		logging.LogError(localLogReaderModuleName, "Error opening log file for watching", err)
		if llr.handler != nil {
			llr.handler.OnError(llr.sessionID, err)
		}
		return
	}
	defer file.Close()
	
	// Seek to end of file
	stat, err := file.Stat()
	if err != nil {
		logging.LogError(localLogReaderModuleName, "Error getting file stat", err)
		return
	}
	
	lastPosition := stat.Size()
	lastInode := stat.Sys()
	
	// Watch for changes
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-llr.stopChan:
			return
		case <-ticker.C:
			// Check if file was rotated (inode changed)
			currentStat, err := os.Stat(filePath)
			if err != nil {
				// File might have been deleted/rotated
				// Try to find the new log file
				newLogFile := llr.findLatestLogFile()
				if newLogFile != "" {
					llr.currentLogFile = newLogFile
					file.Close()
					llr.watchForNewLines(newLogFile)
					return
				}
				continue
			}
			
			currentInode := currentStat.Sys()
			if currentInode != lastInode {
				// File was rotated, reopen
				file.Close()
				file, err = os.Open(filePath)
				if err != nil {
					logging.LogError(localLogReaderModuleName, "Error reopening log file after rotation", err)
					continue
				}
				lastPosition = 0
				lastInode = currentInode
			}
			
			// Check if file size increased
			currentSize := currentStat.Size()
			if currentSize > lastPosition {
				// Read new lines
				file.Seek(lastPosition, 0)
				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					line := scanner.Text()
					if line != "" && llr.handler != nil {
						llr.handler.OnLogLine(llr.sessionID, llr.iofogUUID, line)
					}
				}
				lastPosition = currentSize
			}
		}
	}
}

// findLatestLogFile finds the latest log file
func (llr *LocalLogReader) findLatestLogFile() string {
	// Try latest log file first
	latestPath := filepath.Join(llr.logDirectory, latestLogFile)
	if _, err := os.Stat(latestPath); err == nil {
		return latestPath
	}
	
	// Try to find log files with pattern
	for i := 0; i < 10; i++ {
		logFile := filepath.Join(llr.logDirectory, fmt.Sprintf(logFilePattern, i))
		if _, err := os.Stat(logFile); err == nil {
			return logFile
		}
	}
	
	return ""
}
