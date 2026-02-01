package nmea

import (
	"fmt"
	"strings"
)

// Message represents a parsed NMEA message
type Message interface {
	IsValid() bool
	GetLatitude() float64
	GetLongitude() float64
}

// Parse parses an NMEA sentence and returns a Message
func Parse(sentence string) (Message, error) {
	sentence = strings.TrimSpace(sentence)
	if !strings.HasPrefix(sentence, "$") {
		return nil, fmt.Errorf("invalid NMEA sentence: missing $ prefix")
	}

	// Remove $ and checksum if present
	parts := strings.Split(sentence, "*")
	body := strings.TrimPrefix(parts[0], "$")

	// Split into fields
	fields := strings.Split(body, ",")
	if len(fields) < 2 {
		return nil, fmt.Errorf("invalid NMEA sentence: too few fields")
	}

	// Parse based on sentence type
	sentenceType := fields[0]
	switch sentenceType {
	case "GPGGA", "GNGGA":
		return parseGGA(fields)
	case "GPRMC", "GNRMC":
		return parseRMC(fields)
	default:
		return nil, fmt.Errorf("unsupported NMEA sentence type: %s", sentenceType)
	}
}

// parseGGA parses a GGA (Global Positioning System Fix Data) sentence
func parseGGA(fields []string) (*GGAMessage, error) {
	if len(fields) < 15 {
		return nil, fmt.Errorf("invalid GGA sentence: too few fields")
	}

	msg := &GGAMessage{}

	// Parse latitude
	if fields[2] != "" && fields[3] != "" {
		lat, err := parseCoordinate(fields[2], fields[3])
		if err == nil {
			msg.latitude = lat
		}
	}

	// Parse longitude
	if fields[4] != "" && fields[5] != "" {
		lon, err := parseCoordinate(fields[4], fields[5])
		if err == nil {
			msg.longitude = lon
		}
	}

	// Parse fix quality
	if fields[6] != "" {
		fmt.Sscanf(fields[6], "%d", &msg.fixQuality)
	}

	msg.valid = msg.latitude != 0 && msg.longitude != 0 && msg.fixQuality > 0

	return msg, nil
}

// parseRMC parses an RMC (Recommended Minimum Specific GPS/Transit Data) sentence
func parseRMC(fields []string) (*RMCMessage, error) {
	if len(fields) < 12 {
		return nil, fmt.Errorf("invalid RMC sentence: too few fields")
	}

	msg := &RMCMessage{}

	// Parse status
	if fields[2] == "A" {
		msg.status = "A"
	}

	// Parse latitude
	if fields[3] != "" && fields[4] != "" {
		lat, err := parseCoordinate(fields[3], fields[4])
		if err == nil {
			msg.latitude = lat
		}
	}

	// Parse longitude
	if fields[5] != "" && fields[6] != "" {
		lon, err := parseCoordinate(fields[5], fields[6])
		if err == nil {
			msg.longitude = lon
		}
	}

	msg.valid = msg.status == "A" && msg.latitude != 0 && msg.longitude != 0

	return msg, nil
}

// parseCoordinate parses a coordinate in NMEA format (DDMM.MMMM, N/S/E/W)
func parseCoordinate(coordStr, direction string) (float64, error) {
	if coordStr == "" {
		return 0, fmt.Errorf("empty coordinate")
	}

	// Parse degrees (first 2 digits) and minutes (rest)
	if len(coordStr) < 2 {
		return 0, fmt.Errorf("coordinate too short")
	}

	var degrees, minutes float64
	if _, err := fmt.Sscanf(coordStr[:2], "%f", &degrees); err != nil {
		return 0, fmt.Errorf("failed to parse degrees: %w", err)
	}
	if _, err := fmt.Sscanf(coordStr[2:], "%f", &minutes); err != nil {
		return 0, fmt.Errorf("failed to parse minutes: %w", err)
	}

	// Convert to decimal degrees
	decimal := degrees + minutes/60.0

	// Apply direction
	if direction == "S" || direction == "W" {
		decimal = -decimal
	}

	return decimal, nil
}
