package scheduler_test

import (
	"context"
	"testing"
	"time"

	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/scheduler"
)

type spySeatObserver struct {
	parked    []bool
	statuses  []string
	launched  []string
	retryTime time.Time
}

func (s *spySeatObserver) ReviewSeatParked(_ context.Context, taskID string, onlyAuthor bool, status string, retryAt time.Time) {
	s.parked, s.statuses, s.retryTime = append(s.parked, onlyAuthor), append(s.statuses, status), retryAt
}

func (s *spySeatObserver) ReviewSeatLaunched(_ context.Context, taskID string) {
	s.launched = append(s.launched, taskID)
}

func dispatchObserved(t *testing.T, repo *officesqlite.Repository) *spySeatObserver {
	t.Helper()
	ss := buildScheduler(t, repo, newFakeTaskStarter())
	observer := &spySeatObserver{}
	ss.SetReviewSeatObserver(observer)
	run := seedRun(t, repo, exclReviewPayload)
	if _, _, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{}); err != nil {
		t.Fatal(err)
	}
	return observer
}

func exclusionRepo(t *testing.T, providers ...routing.ProviderID) *officesqlite.Repository {
	t.Helper()
	repo := newExclusionRepo(t, exclReviewEvents)
	seedRoutingConfig(t, repo, providers)
	seatExclusionReviewer(t, repo)
	seedWorkAuthor(t, repo, "claude-acp")
	return repo
}

// @covers AC-OFFICE-COMPLETION-GATES-009.2
func TestReviewSeatObserverHearsAnAuthorOnlyPark(t *testing.T) {
	observer := dispatchObserved(t, exclusionRepo(t, "claude-acp"))
	if len(observer.parked) != 1 || !observer.parked[0] || len(observer.launched) != 0 {
		t.Fatalf("observer = %+v", observer)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-009.3
func TestReviewSeatObserverHearsATemporaryPark(t *testing.T) {
	repo := exclusionRepo(t, "claude-acp", "codex-acp")
	retryAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	if err := repo.MarkProviderDegraded(context.Background(), officemodels.ProviderHealth{
		WorkspaceID: testWorkspaceID, ProviderID: "codex-acp",
		Scope: officesqlite.HealthScopeProvider, State: officesqlite.HealthStateDegraded,
		ErrorCode: "rate_limited", RetryAt: &retryAt,
	}); err != nil {
		t.Fatal(err)
	}
	observer := dispatchObserved(t, repo)
	if len(observer.parked) != 1 || observer.parked[0] || observer.statuses[0] != routing.StatusWaitingForCapacity ||
		!observer.retryTime.Equal(retryAt) {
		t.Fatalf("observer = %+v", observer)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-009.3
func TestReviewSeatObserverHearsALaunch(t *testing.T) {
	observer := dispatchObserved(t, exclusionRepo(t, "claude-acp", "codex-acp"))
	if len(observer.launched) != 1 || observer.launched[0] != exclTaskID || len(observer.parked) != 0 {
		t.Fatalf("observer = %+v", observer)
	}
}
