package github

import (
	"context"
	"slices"
	"testing"
)

func TestListPRFilesFollowsPagination(t *testing.T) {
	client, recorded := newLinkPaginatedPATServer(t, "/repos/o/r/pulls/7/files", []string{
		`[{"filename":"a.go","status":"modified"}]`,
		`[{"filename":"tests/b_test.go","status":"renamed","previous_filename":"tests/old_test.go"}]`,
	})
	files, err := client.ListPRFiles(context.Background(), "o", "r", 7)
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	if len(files) != 2 || len(*recorded) != 2 {
		t.Fatalf("files=%d requests=%d, want 2 and 2", len(files), len(*recorded))
	}
}

func TestObserveSelectedChecksReportsFailingNames(t *testing.T) {
	pr := &PR{HeadSHA: "h1"}
	observation := observeSelectedChecks(pr, []CheckRun{
		{Name: "lint", Status: checkStatusCompleted, Conclusion: checkConclusionFail},
		{Name: "unit", Status: checkStatusCompleted, Conclusion: "success"},
	}, nil)
	if observation.State != "failure" || !slices.Equal(observation.FailingNames, []string{"lint"}) || observation.Observed != 2 {
		t.Fatalf("observation = %+v", observation)
	}
}

func TestChangedFilesObservationCompleteness(t *testing.T) {
	files := []PRFile{{Filename: "a.go"}, {Filename: "tests/new_test.go", OldPath: "tests/old_test.go"}}
	complete := changedFilesObservation(
		&PR{HeadSHA: "h1"}, &PR{HeadSHA: "h1", ChangedFiles: 2, ChangedFilesObserved: true}, files)
	if !complete.Complete || !slices.Equal(complete.Paths, []string{"a.go", "tests/new_test.go", "tests/old_test.go"}) {
		t.Fatalf("complete observation = %+v", complete)
	}
	for name, after := range map[string]*PR{
		"head moved":       {HeadSHA: "h2", ChangedFiles: 2, ChangedFilesObserved: true},
		"short list":       {HeadSHA: "h1", ChangedFiles: 5, ChangedFilesObserved: true},
		"count unobserved": {HeadSHA: "h1"},
	} {
		if changedFilesObservation(&PR{HeadSHA: "h1"}, after, files).Complete {
			t.Fatalf("%s: observation marked complete", name)
		}
	}
}

func TestUnreadableWorkflowsKeepChecksPending(t *testing.T) {
	passing := &CheckObservation{State: "success"}
	markWorkflowsUnknown(passing)
	if passing.State != checkStatusPending || !passing.ActiveWorkflowRuns {
		t.Fatalf("passing checks with unknown workflows = %+v", passing)
	}
	failing := &CheckObservation{State: checkConclusionFail}
	markWorkflowsUnknown(failing)
	if failing.State != checkConclusionFail {
		t.Fatalf("a known failure was hidden: %+v", failing)
	}
}
