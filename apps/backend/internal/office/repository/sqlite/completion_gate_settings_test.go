package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestCompletionGateSettingsLifecycle(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	if _, found, err := repo.GetCompletionGateSettings(ctx, "ws-1"); err != nil || found {
		t.Fatalf("empty read found=%v err=%v", found, err)
	}
	want := models.CompletionGateSettings{
		WorkspaceID:        "ws-1",
		DefaultCriteria:    []models.CompletionGateCriterionSpec{{Kind: "ci_at_head", ProtectedPaths: []string{"tests/**"}}},
		DefaultMaxAttempts: 2, DefaultMaxRework: 4, CeilingAttempts: 8, CeilingRework: 9,
	}
	if err := repo.SetCompletionGateSettings(ctx, want); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, found, err := repo.GetCompletionGateSettings(ctx, "ws-1")
	if err != nil || !found || got.CeilingRework != 9 || len(got.DefaultCriteria) != 1 || got.DefaultCriteria[0].ProtectedPaths[0] != "tests/**" {
		t.Fatalf("got %+v found=%v err=%v", got, found, err)
	}
	if err := repo.DeleteWorkspaceData(ctx, "ws-1"); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}
	if _, found, err := repo.GetCompletionGateSettings(ctx, "ws-1"); err != nil || found {
		t.Fatalf("after delete found=%v err=%v", found, err)
	}
}
