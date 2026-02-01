package messagebus

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const (
	preGeneratedIDsCount = 100_000
)

// MessageIdGenerator generates unique message IDs
type MessageIdGenerator struct {
	generatedIDs chan string
	refillMutex  sync.Mutex
	isRefilling  bool
	stopChan     chan struct{}
	wg           sync.WaitGroup
}

// NewMessageIdGenerator creates a new MessageIdGenerator
func NewMessageIdGenerator() *MessageIdGenerator {
	gen := &MessageIdGenerator{
		generatedIDs: make(chan string, preGeneratedIDsCount),
		stopChan:     make(chan struct{}),
	}

	// Start refilling goroutine
	gen.wg.Add(1)
	go gen.refillLoop()

	return gen
}

// GetNextId returns the next generated ID from the pool
func (g *MessageIdGenerator) GetNextId() string {
	return <-g.generatedIDs
}

// refillLoop continuously refills the ID pool
func (g *MessageIdGenerator) refillLoop() {
	defer g.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Initial refill
	g.refill()

	for {
		select {
		case <-ticker.C:
			g.refill()
		case <-g.stopChan:
			return
		}
	}
}

// refill generates UUIDs and adds them to the pool
func (g *MessageIdGenerator) refill() {
	g.refillMutex.Lock()
	defer g.refillMutex.Unlock()

	if g.isRefilling {
		return
	}

	g.isRefilling = true
	defer func() {
		g.isRefilling = false
	}()

	// Generate IDs until pool is full
	for len(g.generatedIDs) < preGeneratedIDsCount {
		id := g.generateUUID()
		select {
		case g.generatedIDs <- id:
		default:
			// Channel is full, stop generating
			return
		}
	}
}

// generateUUID generates a UUID without dashes (similar to Java UUID.randomUUID().toString().replaceAll("-", ""))
func (g *MessageIdGenerator) generateUUID() string {
	uuid := make([]byte, 16)
	_, err := rand.Read(uuid)
	if err != nil {
		// Fallback: use timestamp-based ID if random generation fails
		return g.generateTimestampID()
	}

	// Set version (4) and variant bits
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant 10

	return hex.EncodeToString(uuid)
}

// generateTimestampID generates a timestamp-based ID as fallback
func (g *MessageIdGenerator) generateTimestampID() string {
	now := time.Now().UnixNano()
	randomBytes := make([]byte, 8)
	rand.Read(randomBytes)
	timestampBytes := []byte{
		byte(now >> 56), byte(now >> 48), byte(now >> 40), byte(now >> 32),
		byte(now >> 24), byte(now >> 16), byte(now >> 8), byte(now),
	}
	return hex.EncodeToString(timestampBytes) + hex.EncodeToString(randomBytes)
}

// Close stops the generator and cleans up resources
func (g *MessageIdGenerator) Close() {
	close(g.stopChan)
	g.wg.Wait()
	close(g.generatedIDs)
}
