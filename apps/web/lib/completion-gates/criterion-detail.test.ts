import { describe, expect, it } from "vitest";
import type { TaskCompletionCriterion } from "@/lib/api/domains/task-completion-gates-api";
import { criterionDetail, type DecisionLike } from "./criterion-detail";

const HEADS = "row-1@a1b2c3d4e5f6";

function ci(extra: Partial<TaskCompletionCriterion> = {}): TaskCompletionCriterion {
  return {
    id: "ci_at_head",
    kind: "ci_at_head",
    result_subject_revision: HEADS,
    ...extra,
  } as TaskCompletionCriterion;
}

function review(extra: Partial<TaskCompletionCriterion> = {}): TaskCompletionCriterion {
  return { ...ci(), id: "review_at_head", kind: "review_at_head", ...extra };
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
    const held = ci({ held_reason: "protected_path_changed", result_detail: ["tests/a.ts"] });
    expect(criterionDetail(held, "unverified", []).text).toEqual({
      key: "office:completionGatesDetailProtectedChanged",
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
