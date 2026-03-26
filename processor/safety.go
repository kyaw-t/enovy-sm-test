package processor

import (
	"encoding/json"
	"os"
	"strings"
)

type SafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// DefaultSafetySettings returns the default safety settings.
// Override via VERTEX_SAFETY_SETTINGS env var as JSON array.
func DefaultSafetySettings() []SafetySetting {
	if env := os.Getenv("VERTEX_SAFETY_SETTINGS"); env != "" {
		var settings []SafetySetting
		if err := json.Unmarshal([]byte(env), &settings); err == nil {
			return settings
		}
	}

	// Default: read individual thresholds from env or use defaults
	return []SafetySetting{
		{
			Category:  "HARM_CATEGORY_HARASSMENT",
			Threshold: getEnvOr("VERTEX_SAFETY_HARASSMENT", "BLOCK_MEDIUM_AND_ABOVE"),
		},
		{
			Category:  "HARM_CATEGORY_HATE_SPEECH",
			Threshold: getEnvOr("VERTEX_SAFETY_HATE_SPEECH", "BLOCK_MEDIUM_AND_ABOVE"),
		},
		{
			Category:  "HARM_CATEGORY_SEXUALLY_EXPLICIT",
			Threshold: getEnvOr("VERTEX_SAFETY_SEXUALLY_EXPLICIT", "BLOCK_MEDIUM_AND_ABOVE"),
		},
		{
			Category:  "HARM_CATEGORY_DANGEROUS_CONTENT",
			Threshold: getEnvOr("VERTEX_SAFETY_DANGEROUS_CONTENT", "BLOCK_MEDIUM_AND_ABOVE"),
		},
	}
}

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
