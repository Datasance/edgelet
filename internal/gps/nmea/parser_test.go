package nmea

import (
	"testing"
)

func TestParseGGA(t *testing.T) {
	// Valid GGA sentence
	sentence := "$GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*47"
	msg, err := Parse(sentence)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !msg.IsValid() {
		t.Error("Message should be valid")
	}

	// Check coordinates (approximate)
	lat := msg.GetLatitude()
	lon := msg.GetLongitude()
	if lat == 0 || lon == 0 {
		t.Errorf("Expected non-zero coordinates, got lat=%f, lon=%f", lat, lon)
	}
}

func TestParseRMC(t *testing.T) {
	// Valid RMC sentence
	sentence := "$GPRMC,123519,A,4807.038,N,01131.000,E,022.4,084.4,230394,003.1,W*6A"
	msg, err := Parse(sentence)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !msg.IsValid() {
		t.Error("Message should be valid")
	}

	// Check coordinates
	lat := msg.GetLatitude()
	lon := msg.GetLongitude()
	if lat == 0 || lon == 0 {
		t.Errorf("Expected non-zero coordinates, got lat=%f, lon=%f", lat, lon)
	}
}
