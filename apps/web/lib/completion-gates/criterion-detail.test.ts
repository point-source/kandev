import { describe, expect, it } from "vitest";
import type { TaskCompletionCriterion } from "@/lib/api/domains/task-completion-gates-api";
import { criterionDetail, type DecisionLike } from "./criterion-detail";

const HEADS = "row-1@a1b2c3d4e5f6";

function ci(extra: Partial<TaskCompletionCriterion> = {}): TaskCompletionCriterion {
  return {
    id: "github_check",
    kind: "check",
    result_subject_revision: HEADS,
    ...extra,
  } as TaskCompletionCriterion;
}

function review(extra: Partial<TaskCompletionCriterion> = {}): TaskCompletionCriterion {
  return { ...ci(), id: "review", kind: "review", ...extra };
}

const decisions: DecisionLike[] = [
  { id: "d-1", deciderType: "agent", deciderId: "ceo", deciderName: "CEO", decision: "approved" },
  { id: "d-2", deciderType: "user", deciderId: "u-1", deciderName: "Sam", decision: "approved" },
  {
    id: "d-3",
    deciderType: "agent",
    deciderId: "qa",
    deciderName: "QA",
    decision: "changes_requested",
  },
];

describe("criterionDetail", () => {
  it("shows only the commit for passing or waiting CI", () => {
    expect(criterionDetail(ci({ result: "pass" }), undefined, [])).toEqual({
      commits: ["a1b2c3d"],
    });
    expect(
      criterionDetail(
        ci({ result: "pending", result_reason: "awaiting_checks" }),
        "unverified",
        [],
      ),
    ).toEqual({ commits: ["a1b2c3d"] });
  });

  it("names failing checks, protected files and a send-back reason", () => {
    const failing = ci({ result: "fail", result_reason: "checks_failed", result_detail: ["unit"] });
    expect(criterionDetail(failing, "unverified", []).text).toEqual({
      key: "office:completionGatesDetailFailing",
      values: { checks: "unit" },
    });
    const held = ci({ hold_kind: "protected_files_changed", result_detail: ["tests/a.ts"] });
    expect(criterionDetail(held, "unverified", []).text).toEqual({
      key: "office:completionGatesHoldProtectedFilesChanged",
      values: { files: "tests/a.ts", count: 1 },
    });
    const sentBack = ci({ result: "fail", result_reason: "sent_back", result_detail: ["redo it"] });
    expect(criterionDetail(sentBack, "unverified", []).text).toEqual({
      key: "office:completionGatesDetailSentBack",
      values: { reason: "redo it" },
    });
  });

  it("keeps the reason for other CI states", () => {
    const missing = ci({ result: "fail", result_reason: "checks_missing" });
    expect(criterionDetail(missing, "unverified", []).text?.key).toBe(
      "office:completionGatesReasonChecksMissing",
    );
  });

  it("names the approvers from the evidence of a passed review", () => {
    const passed = review({
      result: "pass",
      evidence: {
        subject: { kind: "github_pr_heads", id: "t", revision: HEADS },
        reference: "d-1,d-2",
      },
    } as Partial<TaskCompletionCriterion>);
    const detail = criterionDetail(passed, undefined, decisions);
    expect(detail.reviewers).toEqual({
      key: "office:completionGatesDetailApprovedBy",
      reviewers: [
        { id: "ceo", name: "CEO", type: "agent" },
        { id: "u-1", name: "Sam", type: "user" },
      ],
    });
  });

  it("names who requested changes on a failed review", () => {
    const failed = review({ result: "fail", result_reason: "review_rejected" });
    expect(criterionDetail(failed, "unverified", decisions).reviewers).toEqual({
      key: "office:completionGatesDetailChangesRequestedBy",
      reviewers: [{ id: "qa", name: "QA", type: "agent" }],
    });
  });

  it("falls back to the reason when the deciding decisions are not loaded", () => {
    const failed = review({ result: "fail", result_reason: "review_rejected" });
    expect(criterionDetail(failed, "unverified", []).text?.key).toBe(
      "office:completionGatesReasonReviewRejected",
    );
  });

  it("asks for an approval while a review is open", () => {
    expect(criterionDetail(review(), "unverified", decisions).text?.key).toBe(
      "office:completionGatesDetailNeedsApproval",
    );
    expect(criterionDetail(review({ result: "pass" }), "evidence_stale", decisions).text?.key).toBe(
      "office:completionGatesDetailNeedsApproval",
    );
  });
});

describe("criterionDetail for holds and waits", () => {
  const limits = {
    criterion_id: "review",
    verification_limit: 3,
    rework_limit: 3,
    attempt_count: 0,
    rework_count: 3,
  };

  it("names a limit hold with its labelled count", () => {
    const rework = review({ hold_kind: "rework_limit_reached", rework_count: 3 });
    expect(criterionDetail(rework, "held", [], limits).text).toEqual({
      key: "office:completionGatesHoldReworkLimitReached",
      values: { used: 3, limit: 3 },
    });
    const attempts = ci({ hold_kind: "verification_limit_reached", attempt_count: 2 });
    expect(criterionDetail(attempts, "held", []).text).toEqual({
      key: "office:completionGatesHoldVerificationLimitReached",
      values: { used: 2, limit: 2 },
    });
  });

  it("names why no reviewer is available", () => {
    const cases = {
      assignee_only: "office:completionGatesHoldNoReviewerAssigneeOnly",
      no_reviewer_role: "office:completionGatesHoldNoReviewerNoRole",
      author_provider_only: "office:completionGatesHoldNoReviewerAuthorProviderOnly",
    };
    for (const [cause, key] of Object.entries(cases)) {
      const held = review({ hold_kind: "no_reviewer_available", result_detail: [cause] });
      expect(criterionDetail(held, "held", []).text?.key).toBe(key);
    }
  });

  it("says how long a review waited before its limit", () => {
    const held = review({
      hold_kind: "review_wait_limit_reached",
      result_detail: ["2026-11-21T10:00:00Z"],
      held_at: "2026-11-21T12:00:00Z",
    });
    expect(criterionDetail(held, "held", []).text).toEqual({
      key: "office:completionGatesHoldReviewWaitHours",
      values: { value: 2 },
    });
  });

  it("shows a parked reviewer as a wait, with the retry time when known", () => {
    const waiting = review({
      result: "pending",
      result_reason: "waiting_for_reviewer",
      result_detail: ["waiting_for_capacity", "", "2026-11-21T10:00:00Z"],
    });
    expect(criterionDetail(waiting, "unverified", []).text?.key).toBe(
      "office:completionGatesDetailWaitingForReviewer",
    );
    waiting.result_detail = [
      "waiting_for_capacity",
      "2026-11-21T15:00:00Z",
      "2026-11-21T10:00:00Z",
    ];
    const detail = criterionDetail(waiting, "unverified", []).text;
    expect(detail?.key).toBe("office:completionGatesDetailWaitingForReviewerUntil");
    expect(detail?.values?.time).toBeTruthy();
  });

  it("shows a person's acceptance and send back of a review", () => {
    const accepted = review({
      result: "pass",
      result_reason: "accepted",
      result_detail: ["read it"],
    });
    expect(criterionDetail(accepted, undefined, decisions).text).toEqual({
      key: "office:completionGatesDetailAccepted",
      values: { reason: "read it" },
    });
    const sentBack = review({
      result: "fail",
      result_reason: "sent_back",
      result_detail: ["fix it"],
    });
    expect(criterionDetail(sentBack, "unverified", decisions).text).toEqual({
      key: "office:completionGatesDetailSentBack",
      values: { reason: "fix it" },
    });
  });
});
