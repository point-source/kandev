package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func approvalHostCriterion() models.TaskCompletionCriterion {
	criterion := ciCriterion()
	criterion.ID, criterion.Kind = "approval", models.TaskCompletionCriterionApproval
	criterion.Config = models.TaskCompletionCriterionConfig{ReworkLimit: 2}
	return criterion
}

func onApproval(change *models.TaskCompletionControlChange) { change.CriterionID = "approval" }

func askApproval(t *testing.T, repo *Repository, revision, reason string) {
	t.Helper()
	if _, err := evaluate(repo, revision, models.TaskCompletionResultPending, func(c *models.TaskCompletionEvaluationChange) {
		c.CriterionID, c.Reason, c.VerifierKind, c.VerifierID = "approval", reason, "approval", "approval"
	}); err != nil {
		t.Fatal(err)
	}
}

func approve(repo *Repository, subject, reason string) error {
	_, err := control(repo, models.TaskCompletionControlApprove, onApproval, func(c *models.TaskCompletionControlChange) {
		c.SubjectRevision, c.Reason = subject, reason
	})
	return err
}

// @covers AC-OFFICE-COMPLETION-GATES-010.1
func TestApprovalCriterionCarriesOnlyAReworkLimit(t *testing.T) {
	criterion := approvalHostCriterion()
	if err := validateCompletionCriteriaForTask(hostGateTaskID, []models.TaskCompletionCriterion{criterion}); err != nil {
		t.Fatalf("valid approval: %v", err)
	}
	for name, mutate := range map[string]func(*models.TaskCompletionCriterion){
		"runner":             func(c *models.TaskCompletionCriterion) { c.Config.Runner = "github" },
		"protected paths":    func(c *models.TaskCompletionCriterion) { c.Config.ProtectedPaths = []string{"tests/**"} },
		"verification limit": func(c *models.TaskCompletionCriterion) { c.Config.VerificationLimit = 3 },
		"review wait limit":  func(c *models.TaskCompletionCriterion) { c.Config.ReviewWaitLimit = 5 },
	} {
		invalid := approvalHostCriterion()
		mutate(&invalid)
		if err := validateCompletionCriteriaForTask(hostGateTaskID, []models.TaskCompletionCriterion{invalid}); !errors.Is(err, repoerrors.ErrTaskCompletionCriteriaInvalid) {
			t.Errorf("approval with %s err = %v", name, err)
		}
	}
	if err := validateCompletionCriteriaForTask(hostGateTaskID, []models.TaskCompletionCriterion{criterion, criterion}); !errors.Is(err, repoerrors.ErrTaskCompletionCriteriaInvalid) {
		t.Fatalf("two approvals err = %v", err)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-010.2, AC-OFFICE-COMPLETION-GATES-010.4, AC-TASKS-COMPLETION-006.10
func TestApproveOnlyPassesAnApprovalThatAsksAtTheHeadsShown(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, approvalHostCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	askApproval(t, repo, "pr-1@head-1", models.TaskCompletionReasonAwaitingCriteria)
	if err := approve(repo, "pr-1@head-1", ""); !errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged) {
		t.Fatalf("approve before asked err = %v", err)
	}
	askApproval(t, repo, "pr-1@head-1", models.TaskCompletionReasonAwaitingPerson)
	if err := approve(repo, "pr-1@head-0", ""); !errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged) {
		t.Fatalf("approve at stale heads err = %v", err)
	}
	if err := approve(repo, "pr-1@head-1", "checked the redirect"); err != nil {
		t.Fatal(err)
	}
	approval := gate(t, repo).Criteria[0]
	if approval.Result != models.TaskCompletionResultPass || approval.ResultReason != models.TaskCompletionReasonApproved ||
		approval.VerdictSource != models.TaskCompletionVerdictPerson || approval.VerifierID != "user-1" || gate(t, repo).Blocked {
		t.Fatalf("approved = %+v", approval)
	}

	askApproval(t, repo, "pr-1@head-1", models.TaskCompletionReasonAwaitingCriteria)
	if gate(t, repo).Criteria[0].ResultReason != models.TaskCompletionReasonApproved {
		t.Fatal("a verifier overwrote the person's approval at the same heads")
	}
	if _, err := evidenceSubmit(repo); !errors.Is(err, repoerrors.ErrTaskCompletionVerifierReserved) {
		t.Fatalf("evidence submission err = %v", err)
	}

	linkPR(t, writer, "pr-1", "head-2")
	if !gate(t, repo).Blocked {
		t.Fatal("approval of earlier heads still passes after a push")
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-010.4
func TestHeldListIncludesApprovalsThatAskAPerson(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, approvalHostCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	askApproval(t, repo, "pr-1@head-1", models.TaskCompletionReasonAwaitingCriteria)
	if held, err := repo.ListHeldCompletionCriteria(context.Background(), "ws-host"); err != nil || len(held) != 0 {
		t.Fatalf("waiting approval listed: %+v, %v", held, err)
	}
	askApproval(t, repo, "pr-1@head-1", models.TaskCompletionReasonAwaitingPerson)
	held, err := repo.ListHeldCompletionCriteria(context.Background(), "ws-host")
	if err != nil || len(held) != 1 || held[0].Criterion.ID != "approval" || held[0].TaskTitle != "Gated" {
		t.Fatalf("held = %+v, %v", held, err)
	}
}

func evidenceSubmit(repo *Repository) (*models.TaskCompletionGateSnapshot, error) {
	return repo.VerifyTaskCompletionCriterion(context.Background(), models.TaskCompletionEvidenceChange{
		TaskID: hostGateTaskID, WorkspaceID: "ws-host", ExpectedRevision: 1, CriterionID: "approval",
		Evidence: models.TaskCompletionEvidence{Subject: models.TaskCompletionEvidenceSubject{
			Kind: models.TaskCompletionEvidenceGitHubPRHeads, ID: hostGateTaskID, Revision: "pr-1@head-1",
		}},
		ActorKind: "agent", ActorID: "worker",
	})
}

// @covers AC-OFFICE-COMPLETION-GATES-010.1
func TestExtendRefusesAVerificationLimitOnAnApproval(t *testing.T) {
	repo, _ := newHostGateRepo(t)
	createHostGateTask(t, repo, approvalHostCriterion())
	_, err := control(repo, models.TaskCompletionControlExtendLimits, onApproval, func(c *models.TaskCompletionControlChange) {
		c.VerificationLimit = 5
	})
	if !errors.Is(err, repoerrors.ErrTaskCompletionCriteriaInvalid) {
		t.Fatalf("verification limit on approval err = %v", err)
	}
	if _, err := control(repo, models.TaskCompletionControlExtendLimits, onApproval, func(c *models.TaskCompletionControlChange) {
		c.ReworkLimit = 4
	}); err != nil {
		t.Fatal(err)
	}
	if config := gate(t, repo).Criteria[0].Config; config.VerificationLimit != 0 || config.ReworkLimit != 4 {
		t.Fatalf("approval config = %+v", config)
	}
}
