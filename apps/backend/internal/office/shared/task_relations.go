package shared

import taskmodels "github.com/kandev/kandev/internal/task/models"

// AgentTaskRelations carries the relation fields an Office agent may declare
// when it creates a task through the runtime: the tasks the new task depends
// on, an explicit workspace mode, and requested completion criteria. The zero
// value declares nothing, so an omitted field keeps the creation path's
// existing default.
type AgentTaskRelations struct {
	BlockedBy     []string
	WorkspaceMode string
	// CompletionCriteria is nil when the agent named none, so the workspace
	// default applies.
	CompletionCriteria *[]taskmodels.TaskCompletionCriterionRequest
	// CreatorAgentID is the creating agent, recorded as criteria owner.
	CreatorAgentID string
}
