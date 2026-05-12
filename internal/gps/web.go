package gps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const (
	webHandlerModuleName = "GPS Web Handler"
	defaultIPAPIURL      = "http://ip-api.com/json"
	timeout              = 10 * time.Second
)

var ipAPIURL = defaultIPAPIURL

// WebHandler handles IP-based GPS location
type WebHandler struct {
	manager *Manager
	config  *config.Config
	client  *http.Client
}

// NewWebHandler creates a new WebHandler
func NewWebHandler(manager *Manager) *WebHandler {
	return &WebHandler{
		manager: manager,
		config:  config.GetInstance(),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Start starts the web handler
func (w *WebHandler) Start() error {
	logging.LogDebug(webHandlerModuleName, "Starting GPS Web Handler")
	// Initial coordinate update
	return w.UpdateCoordinates()
}

// Stop stops the web handler
func (w *WebHandler) Stop() error {
	logging.LogDebug(webHandlerModuleName, "Stopping GPS Web Handler")
	return nil
}

// UpdateCoordinates updates coordinates using IP-based location service
func (w *WebHandler) UpdateCoordinates() error {
	logging.LogDebug(webHandlerModuleName, "Updating coordinates from IP-based location service")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ipAPIURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get location: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var locationData struct {
		Status    string   `json:"status"`
		Message   string   `json:"message"`
		Latitude  *float64 `json:"lat"`
		Longitude *float64 `json:"lon"`
	}

	if err := json.Unmarshal(body, &locationData); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(locationData.Status), "fail") {
		return fmt.Errorf("location provider returned failure: %s", strings.TrimSpace(locationData.Message))
	}
	if locationData.Latitude == nil || locationData.Longitude == nil {
		return fmt.Errorf("location provider missing lat/lon fields")
	}

	// Format coordinates as "lat,lon"
	coordinates := fmt.Sprintf("%.5f,%.5f", *locationData.Latitude, *locationData.Longitude)
	w.config.GPSCoordinates = coordinates

	logging.LogDebug(webHandlerModuleName, fmt.Sprintf("Updated GPS coordinates: %s", coordinates))
	return nil
}

// GetCoordinates returns the current coordinates
func (w *WebHandler) GetCoordinates() string {
	coords := w.config.GPSCoordinates
	if coords == "" {
		return "0.00000,0.00000"
	}
	return coords
}

// ParseCoordinates parses coordinates string "lat,lon" into latitude and longitude
func ParseCoordinates(coords string) (float64, float64, error) {
	parts := strings.Split(coords, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid coordinates format: %s", coords)
	}

	var lat, lon float64
	if _, err := fmt.Sscanf(parts[0], "%f", &lat); err != nil {
		return 0, 0, fmt.Errorf("invalid latitude: %w", err)
	}
	if _, err := fmt.Sscanf(parts[1], "%f", &lon); err != nil {
		return 0, 0, fmt.Errorf("invalid longitude: %w", err)
	}

	return lat, lon, nil
}
