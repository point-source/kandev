package models

// CompletionGateCriterionSpec is a criterion requested at task creation or
// configured as a workspace default. Kind is check (with a runner) or review.
type CompletionGateCriterionSpec struct {
	Kind              string   `json:"kind"`
	Runner            string   `json:"runner,omitempty"`
	ProtectedPaths    []string `json:"protected_paths,omitempty"`
	VerificationLimit int      `json:"verification_limit,omitempty"`
	ReworkLimit       int      `json:"rework_limit,omitempty"`
	ReviewWaitLimit   int      `json:"review_wait_limit,omitempty"`
}

// CompletionGateSettings are a workspace's default criteria, limit defaults and
// limit ceilings. Zero limits mean "use the shipped value"; a zero review wait
// limit means a parked review waits indefinitely. The review wait limit has no
// ceiling because waiting starts no runs.
type CompletionGateSettings struct {
	WorkspaceID              string                        `json:"workspace_id"`
	DefaultCriteria          []CompletionGateCriterionSpec `json:"default_criteria"`
	DefaultVerificationLimit int                           `json:"default_verification_limit"`
	DefaultReworkLimit       int                           `json:"default_rework_limit"`
	VerificationLimitCeiling int                           `json:"verification_limit_ceiling"`
	ReworkLimitCeiling       int                           `json:"rework_limit_ceiling"`
	DefaultReviewWaitLimit   int                           `json:"default_review_wait_limit"`
}
