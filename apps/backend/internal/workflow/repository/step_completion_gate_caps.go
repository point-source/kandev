package repository

import (
	"encoding/json"

	"github.com/kandev/kandev/internal/workflow/models"
)

func encodeCompletionGateCaps(caps *models.CompletionGateCaps) string {
	if caps == nil || (caps.MaxVerificationAttempts == 0 && caps.MaxReworkRounds == 0) {
		return ""
	}
	data, err := json.Marshal(caps)
	if err != nil {
		return ""
	}
	return string(data)
}

func decodeCompletionGateCaps(raw string) *models.CompletionGateCaps {
	if raw == "" {
		return nil
	}
	var caps models.CompletionGateCaps
	if json.Unmarshal([]byte(raw), &caps) != nil || (caps.MaxVerificationAttempts == 0 && caps.MaxReworkRounds == 0) {
		return nil
	}
	return &caps
}
