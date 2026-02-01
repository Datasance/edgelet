package messagebus

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/utils"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	headerSize              = 33
	maximumMessagePerFile   = 1000
	maximumArchiveSizeMB    = 1
	maximumArchiveSizeBytes = maximumArchiveSizeMB * 1_000_000
	freeMemoryThreshold     = 32 * utils.MiB
)

// MessageArchive archives messages to disk
type MessageArchive struct {
	name            string
	diskDirectory   string
	currentFileName string
	indexFile       *os.File
	dataFile        *os.File
	mu              sync.Mutex
	logger          *logging.ModuleLogger
}

// NewMessageArchive creates a new MessageArchive
func NewMessageArchive(name string) *MessageArchive {
	cfg := config.GetInstance()
	diskDir := cfg.DiskDirectory
	if diskDir == "" {
		diskDir = "/var/lib/iofog-agent/"
	}

	archive := &MessageArchive{
		name:          name,
		diskDirectory: filepath.Join(diskDir, "messages", "archive"),
		logger:        logging.NewModuleLogger("MessageArchive"),
	}

	archive.init()
	return archive
}

// init initializes the archive and finds the last file
func (a *MessageArchive) init() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.currentFileName = ""

	// Create directory if it doesn't exist
	if err := os.MkdirAll(a.diskDirectory, 0755); err != nil {
		a.logger.Errorf("Failed to create archive directory: %v", err)
		return
	}

	// Find the last index file for this publisher
	pattern := fmt.Sprintf("%s_*.idx", a.name)
	matches, err := filepath.Glob(filepath.Join(a.diskDirectory, pattern))
	if err != nil {
		a.logger.Errorf("Failed to glob archive files: %v", err)
		return
	}

	if len(matches) == 0 {
		return
	}

	// Sort files by timestamp (extracted from filename)
	sort.Strings(matches)

	// Find the file with the latest timestamp
	var lastFile string
	var lastTimestamp int64

	for _, match := range matches {
		// Extract timestamp from filename: name_timestamp.idx
		base := filepath.Base(match)
		if len(base) < len(a.name)+2 {
			continue
		}

		// Find the timestamp part
		start := len(a.name) + 1
		end := len(base) - 4 // Remove .idx
		if end <= start {
			continue
		}

		var timestamp int64
		if _, err := fmt.Sscanf(base[start:end], "%d", &timestamp); err != nil {
			continue
		}

		if timestamp > lastTimestamp {
			lastTimestamp = timestamp
			lastFile = match
		}
	}

	if lastFile != "" {
		// Check if the file is not full
		info, err := os.Stat(lastFile)
		if err == nil {
			maxSize := int64((headerSize + 8) * maximumMessagePerFile) // header + long (timestamp)
			if info.Size() < maxSize {
				a.currentFileName = lastFile
			}
		}
	}
}

// openFiles opens index and data files for a given timestamp
func (a *MessageArchive) openFiles(timestamp int64) error {
	if a.currentFileName == "" {
		a.currentFileName = filepath.Join(a.diskDirectory, fmt.Sprintf("%s_%d.idx", a.name, timestamp))
	}

	// Open index file
	indexFile, err := os.OpenFile(a.currentFileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open index file: %w", err)
	}

	// Open data file
	dataFileName := a.currentFileName[:len(a.currentFileName)-4] + ".iomsg"
	dataFile, err := os.OpenFile(dataFileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		indexFile.Close()
		return fmt.Errorf("failed to open data file: %w", err)
	}

	a.indexFile = indexFile
	a.dataFile = dataFile
	return nil
}

// Save saves a message to the archive
func (a *MessageArchive) Save(messageBytes []byte, timestamp int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.indexFile == nil {
		if err := a.openFiles(timestamp); err != nil {
			return err
		}
	}

	// Check if we need to create a new file
	dataFileInfo, err := a.dataFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat data file: %w", err)
	}

	if int64(len(messageBytes))+dataFileInfo.Size() >= maximumArchiveSizeBytes {
		// Close current files
		a.indexFile.Close()
		a.dataFile.Close()
		a.indexFile = nil
		a.dataFile = nil
		a.currentFileName = ""

		// Open new files
		if err := a.openFiles(timestamp); err != nil {
			return err
		}
	}

	// Get current position in data file
	dataPos, err := a.dataFile.Seek(0, os.SEEK_END)
	if err != nil {
		return fmt.Errorf("failed to seek data file: %w", err)
	}

	// Write header to index file
	if len(messageBytes) < headerSize {
		return fmt.Errorf("message too short: %d bytes", len(messageBytes))
	}

	header := messageBytes[:headerSize]
	if _, err := a.indexFile.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write data position (long = 8 bytes)
	if err := binary.Write(a.indexFile, binary.BigEndian, dataPos); err != nil {
		return fmt.Errorf("failed to write data position: %w", err)
	}

	// Write data to data file
	data := messageBytes[headerSize:]
	if _, err := a.dataFile.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

// MessageQuery retrieves messages within a time range
func (a *MessageArchive) MessageQuery(from, to int64) []*models.Message {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.logger.Debug("Start message query")

	result := make([]*models.Message, 0)

	// Find all index files in the time range
	pattern := fmt.Sprintf("%s_*.idx", a.name)
	matches, err := filepath.Glob(filepath.Join(a.diskDirectory, pattern))
	if err != nil {
		a.logger.Errorf("Failed to glob archive files: %v", err)
		return result
	}

	if len(matches) == 0 {
		return result
	}

	// Sort files
	sort.Strings(matches)

	// Filter files by timestamp range
	var relevantFiles []string
	for i := len(matches) - 1; i >= 0; i-- {
		file := matches[i]
		base := filepath.Base(file)
		if len(base) < len(a.name)+2 {
			continue
		}

		start := len(a.name) + 1
		end := len(base) - 4
		if end <= start {
			continue
		}

		var fileTimestamp int64
		if _, err := fmt.Sscanf(base[start:end], "%d", &fileTimestamp); err != nil {
			continue
		}

		if fileTimestamp < from {
			break
		}

		if fileTimestamp >= from && fileTimestamp <= to {
			relevantFiles = append(relevantFiles, file)
		}
	}

	// Read messages from relevant files
	for _, indexFileName := range relevantFiles {
		messages := a.readMessagesFromFile(indexFileName, from, to)
		result = append(result, messages...)
	}

	a.logger.Debug("Finish message query")
	return result
}

// readMessagesFromFile reads messages from a specific archive file
func (a *MessageArchive) readMessagesFromFile(indexFileName string, from, to int64) []*models.Message {
	result := make([]*models.Message, 0)

	indexFile, err := os.Open(indexFileName)
	if err != nil {
		a.logger.Errorf("Failed to open index file %s: %v", indexFileName, err)
		return result
	}
	defer indexFile.Close()

	dataFileName := indexFileName[:len(indexFileName)-4] + ".iomsg"
	dataFile, err := os.Open(dataFileName)
	if err != nil {
		a.logger.Errorf("Failed to open data file %s: %v", dataFileName, err)
		return result
	}
	defer dataFile.Close()

	dataFileInfo, err := dataFile.Stat()
	if err != nil {
		return result
	}
	dataFileLength := dataFileInfo.Size()

	header := make([]byte, headerSize)
	for {
		// Check available memory (simplified check)
		// In production, might want to use runtime.MemStats

		// Read header
		if _, err := indexFile.Read(header); err != nil {
			break // EOF or error
		}

		// Check version
		version := utils.BytesToShort(utils.CopyOfRange(header, 0, 2))
		if version != models.MessageVersion {
			a.logger.Errorf("Invalid index file format: version %d", version)
			break
		}

		// Read data position
		var dataPos int64
		if err := binary.Read(indexFile, binary.BigEndian, &dataPos); err != nil {
			break
		}

		// Calculate data size from header
		dataSize := a.getDataSize(header)
		if dataPos+int64(dataSize) > dataFileLength || int64(dataSize) > dataFileLength {
			a.logger.Errorf("Invalid data file format")
			break
		}

		// Read data
		data := make([]byte, dataSize)
		if _, err := dataFile.ReadAt(data, dataPos); err != nil {
			break
		}

		// Reconstruct message bytes
		messageBytes := make([]byte, headerSize+dataSize)
		copy(messageBytes[:headerSize], header)
		copy(messageBytes[headerSize:], data)

		// Parse message (simplified - would need full binary parsing)
		// For now, we'll need to implement binary message parsing
		// This is a placeholder - full implementation would parse the binary format
		// and create a Message object
		_ = messageBytes
	}

	return result
}

// getDataSize calculates the data size from the header
func (a *MessageArchive) getDataSize(header []byte) int {
	if len(header) < headerSize {
		return 0
	}

	size := 0
	size += int(header[2])                                                    // id
	size += int(utils.BytesToShort(utils.CopyOfRange(header, 3, 5)))          // tag
	size += int(header[5])                                                     // groupid
	size += int(header[6])                                                    // sequencenumber
	size += int(header[7])                                                    // sequencetotal
	size += int(header[8])                                                    // priority
	size += int(header[9])                                                    // timestamp
	size += int(header[10])                                                   // publisher
	size += int(utils.BytesToShort(utils.CopyOfRange(header, 11, 13)))        // authid
	size += int(utils.BytesToShort(utils.CopyOfRange(header, 13, 15)))        // authgroup
	size += int(header[15])                                                   // chainposition
	size += int(utils.BytesToShort(utils.CopyOfRange(header, 16, 18)))       // hash
	size += int(utils.BytesToShort(utils.CopyOfRange(header, 18, 20)))       // previoushash
	size += int(utils.BytesToShort(utils.CopyOfRange(header, 20, 22)))        // nonce
	size += int(header[22])                                                   // difficultytarget
	size += int(header[23])                                                   // infotype
	size += int(header[24])                                                   // infoformat
	size += int(utils.BytesToInteger(utils.CopyOfRange(header, 25, 29)))      // contextdata
	size += int(utils.BytesToInteger(utils.CopyOfRange(header, 29, 33)))      // contentdata

	return size
}

// Close closes the archive files
func (a *MessageArchive) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.currentFileName = ""

	if a.indexFile != nil {
		a.indexFile.Close()
		a.indexFile = nil
	}

	if a.dataFile != nil {
		a.dataFile.Close()
		a.dataFile = nil
	}

	return nil
}
