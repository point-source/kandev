package repository

import (
	"encoding/json"

	"github.com/kandev/kandev/internal/workflow/models"
)

func encodeCompletionGateLimits(limits *models.CompletionGateLimits) string {
	if limits == nil || (limits.VerificationLimit == 0 && limits.ReworkLimit == 0 && limits.ReviewWaitLimit == 0) {
		return ""
	}
	data, err := json.Marshal(limits)
	if err != nil {
		return ""
	}
	return string(data)
}

func decodeCompletionGateLimits(raw string) *models.CompletionGateLimits {
	if raw == "" {
		return nil
	}
	var limits models.CompletionGateLimits
	if json.Unmarshal([]byte(raw), &limits) != nil || (limits.VerificationLimit == 0 && limits.ReworkLimit == 0 && limits.ReviewWaitLimit == 0) {
		return nil
	}
	return &limits
}
