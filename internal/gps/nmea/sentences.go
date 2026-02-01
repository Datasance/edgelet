package nmea

// GGAMessage represents a GGA NMEA message
type GGAMessage struct {
	latitude   float64
	longitude  float64
	fixQuality int
	valid      bool
}

// IsValid returns whether the message is valid
func (m *GGAMessage) IsValid() bool {
	return m.valid
}

// GetLatitude returns the latitude
func (m *GGAMessage) GetLatitude() float64 {
	return m.latitude
}

// GetLongitude returns the longitude
func (m *GGAMessage) GetLongitude() float64 {
	return m.longitude
}

// RMCMessage represents an RMC NMEA message
type RMCMessage struct {
	status    string
	latitude  float64
	longitude float64
	valid     bool
}

// IsValid returns whether the message is valid
func (m *RMCMessage) IsValid() bool {
	return m.valid
}

// GetLatitude returns the latitude
func (m *RMCMessage) GetLatitude() float64 {
	return m.latitude
}

// GetLongitude returns the longitude
func (m *RMCMessage) GetLongitude() float64 {
	return m.longitude
}
