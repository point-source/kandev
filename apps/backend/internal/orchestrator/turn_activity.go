package orchestrator

import (
	"sync"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// turnActivity tracks, in memory, whether a session's open turn is being driven
// by the foreground agent (actively generating) or is merely held open while a
// spawned background task runs.
//
// It is the finer-grained signal behind checkSessionPromptable: a session whose
// foreground turn has yielded to background work should accept a new message
// even though its DB state still reads RUNNING. The single-scalar session state
// cannot tell "the foreground agent is generating" from "the foreground turn is
// idle but a spawned background task (a subagent, a run-in-background shell) is
// still running", which is how a long background job used to lock the operator
// out of the conversation.
//
// The absent/zero state means "foreground generating": callers default to the
// pre-existing behaviour (reject a new prompt while RUNNING) unless a background
// task has been explicitly registered for the session, so nothing changes for a
// session that has no background work outstanding.
type turnActivity struct {
	mu         sync.Mutex
	background map[string]struct{} // outstanding background/spawned tool-call IDs
	yielded    bool                // foreground handed off to background work
}

// turnActivityFor returns the per-session activity record, creating it when
// create is true. Returns nil when the record is absent and create is false.
func (s *Service) turnActivityFor(sessionID string, create bool) *turnActivity {
	if v, ok := s.foregroundActivity.Load(sessionID); ok {
		return v.(*turnActivity)
	}
	if !create {
		return nil
	}
	ta := &turnActivity{background: make(map[string]struct{})}
	actual, _ := s.foregroundActivity.LoadOrStore(sessionID, ta)
	return actual.(*turnActivity)
}

// markForegroundGenerating records that the foreground agent produced output
// (streamed a message/thinking chunk, or a fresh foreground prompt was
// dispatched), so the turn is once again driven by the foreground even if a
// background task is still outstanding.
func (s *Service) markForegroundGenerating(sessionID string) {
	if sessionID == "" {
		return
	}
	ta := s.turnActivityFor(sessionID, true)
	ta.mu.Lock()
	ta.yielded = false
	ta.mu.Unlock()
}

// registerBackgroundTask records a spawned background task (a subagent Task or a
// run-in-background shell). While at least one is outstanding, the foreground
// turn is treated as "waiting on background" rather than "actively generating".
func (s *Service) registerBackgroundTask(sessionID, toolCallID string) {
	if sessionID == "" || toolCallID == "" {
		return
	}
	ta := s.turnActivityFor(sessionID, true)
	ta.mu.Lock()
	ta.background[toolCallID] = struct{}{}
	ta.yielded = true
	ta.mu.Unlock()
}

// hasBackgroundTask reports whether toolCallID is already tracked as outstanding
// background work for the session. Used to make the tool_call_update
// registration path fire only on the first recognizable frame: re-registering
// on later updates would re-set `yielded` and clobber a foreground stream that
// marked the turn generating again (see markForegroundGenerating).
func (s *Service) hasBackgroundTask(sessionID, toolCallID string) bool {
	ta := s.turnActivityFor(sessionID, false)
	if ta == nil {
		return false
	}
	ta.mu.Lock()
	defer ta.mu.Unlock()
	_, ok := ta.background[toolCallID]
	return ok
}

// completeBackgroundTask clears a previously-registered background task. When no
// background task remains, the foreground turn is no longer "waiting on
// background".
func (s *Service) completeBackgroundTask(sessionID, toolCallID string) {
	if sessionID == "" || toolCallID == "" {
		return
	}
	ta := s.turnActivityFor(sessionID, false)
	if ta == nil {
		return
	}
	ta.mu.Lock()
	delete(ta.background, toolCallID)
	if len(ta.background) == 0 {
		ta.yielded = false
	}
	ta.mu.Unlock()
}

// clearTurnActivity drops all tracked activity for a session. Called from every
// turn-close path (turn complete, error, cancel, teardown) so a fresh turn
// starts from the "foreground generating" default.
func (s *Service) clearTurnActivity(sessionID string) {
	if sessionID == "" {
		return
	}
	s.foregroundActivity.Delete(sessionID)
}

// isForegroundTurnGenerating reports whether the session's foreground agent turn
// is actively generating. It returns true (generating) unless the turn has
// yielded to an outstanding background task. An untracked session defaults to
// true, preserving the historical "reject a new prompt while RUNNING" contract.
func (s *Service) isForegroundTurnGenerating(sessionID string) bool {
	ta := s.turnActivityFor(sessionID, false)
	if ta == nil {
		return true
	}
	ta.mu.Lock()
	defer ta.mu.Unlock()
	return !ta.yielded
}

// normalizedIsBackgroundTask reports whether a normalized tool payload represents
// spawned background work the foreground turn waits on: a subagent Task, a
// run-in-background shell command, or an active Claude Monitor watch.
//
// A create_task tool is deliberately NOT background — it spawns an independent
// task/session with its own lifecycle and does not hold the spawning turn open.
func normalizedIsBackgroundTask(n *streams.NormalizedPayload) bool {
	if n == nil {
		return false
	}
	if n.Kind() == streams.ToolKindSubagentTask {
		return true
	}
	if se := n.ShellExec(); se != nil && se.Background {
		return true
	}
	// A Monitor is a long-running watch the foreground turn is not actively
	// generating against. It normalizes to a Generic payload, so it is
	// recognized via the shared streams predicate rather than a tool-name match.
	if n.IsActiveMonitor() {
		return true
	}
	return false
}
