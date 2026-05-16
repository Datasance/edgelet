package handlers

import (
	"strings"
	"unicode"
)

func normalizeReportKey(raw string) string {
	key := strings.TrimSpace(strings.ToLower(raw))
	switch key {
	case "gps-coordinates(lat,lon)":
		return "gpsCoordinates"
	case "developer's-mode":
		return "developerMode"
	}

	replacer := strings.NewReplacer("-", " ", "_", " ", "/", " ", "(", " ", ")", " ", ",", " ", "'", "")
	key = replacer.Replace(key)
	parts := strings.Fields(key)
	if len(parts) == 0 {
		return ""
	}
	camel := parts[0]
	for _, p := range parts[1:] {
		if p == "" {
			continue
		}
		runes := []rune(p)
		runes[0] = unicode.ToUpper(runes[0])
		camel += string(runes)
	}
	return camel
}
