package models

// CompletionGateCriterionSpec is a criterion requested at task creation or
// configured as a workspace default. Kind is ci_at_head or review_at_head.
type CompletionGateCriterionSpec struct {
	Kind                    string   `json:"kind"`
	ProtectedPaths          []string `json:"protected_paths,omitempty"`
	MaxVerificationAttempts int      `json:"max_verification_attempts,omitempty"`
	MaxReworkRounds         int      `json:"max_rework_rounds,omitempty"`
}

// CompletionGateSettings are a workspace's default criteria, cap defaults and
// cap ceilings. Zero caps mean "use the shipped value".
type CompletionGateSettings struct {
	WorkspaceID        string                        `json:"workspace_id"`
	DefaultCriteria    []CompletionGateCriterionSpec `json:"default_criteria"`
	DefaultMaxAttempts int                           `json:"default_max_attempts"`
	DefaultMaxRework   int                           `json:"default_max_rework"`
	CeilingAttempts    int                           `json:"ceiling_attempts"`
	CeilingRework      int                           `json:"ceiling_rework"`
}
