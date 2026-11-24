import { describe, expect, it } from "vitest";
import type { TaskCompletionCriterion } from "@/lib/api/domains/task-completion-gates-api";
import { DIALOG_KEYS, KIND_KEYS, REASON_REQUIRED_ACTIONS, actionLabelKey } from "./copy-keys";
import {
  asksPerson,
  criterionActions,
  criterionAtShownCommit,
  criterionDisplayStatus,
} from "./criteria-display";
import { criterionDetail } from "./criterion-detail";
import {
  criteriaFromSelection,
  EMPTY_SELECTION,
  selectionFromCriteria,
} from "./criteria-selection";

const HEADS = "pr-1@a1b2c3d4e5f6";

function approval(fields: Partial<TaskCompletionCriterion> = {}): TaskCompletionCriterion {
  return {
    id: "approval",
    description: "",
    evidence_subject: { kind: "github_pr_heads", id: "t1" },
    criterion_revision: 1,
    kind: "approval",
    result: "pending",
    result_subject_revision: HEADS,
    ...fields,
  };
}

describe("approval criterion", () => {
  it("needs the person only once it asks them", () => {
    const asked = approval({ result_reason: "awaiting_person" });
    const waiting = approval({ result_reason: "awaiting_criteria", result_detail: ["check"] });
    expect(asksPerson(asked)).toBe(true);
    expect(criterionDisplayStatus(asked, "unverified")).toBe("needs_you");
    expect(criterionDisplayStatus(waiting, "unverified")).toBe("pending");
    expect(
      criterionDisplayStatus(approval({ result: "pass", result_reason: "approved" }), undefined),
    ).toBe("pass");
  });

  it("offers Approve and Send back when asked, Send back on the task page at any time", () => {
    const asked = approval({ result_reason: "awaiting_person" });
    expect(criterionActions(asked, "inbox")).toEqual(["approve", "send_back"]);
    expect(criterionActions(asked, "task")).toEqual(["approve", "send_back"]);
    const waiting = approval({ result_reason: "awaiting_criteria" });
    expect(criterionActions(waiting, "task")).toEqual(["send_back"]);
    expect(criterionActions(waiting, "inbox")).toEqual([]);
    expect(criterionActions(approval({ hold_kind: "rework_limit_reached" }), "inbox")).toEqual([
      "extend_rounds",
    ]);
  });

  it("offers no Send back and says so while no pull request is linked", () => {
    const unlinked = approval({ result: undefined, result_subject_revision: undefined });
    expect(criterionActions(criterionAtShownCommit(unlinked, undefined), "task")).toEqual([]);
    expect(criterionDetail(unlinked, "unverified", []).text?.key).toBe(
      "office:completionGatesReasonPrMissing",
    );
    const detached = approval({
      result_reason: "awaiting_criteria",
      result_detail: ["pull_request"],
      result_subject_revision: "",
    });
    expect(criterionActions(detached, "task")).toEqual([]);
    expect(criterionDetail(detached, "unverified", []).text?.key).toBe(
      "office:completionGatesReasonPrMissing",
    );
  });

  it("says what it waits for, that it asks you, or who approved", () => {
    expect(
      criterionDetail(
        approval({ result_reason: "awaiting_criteria", result_detail: ["work_stage"] }),
        "unverified",
        [],
      ).text?.key,
    ).toBe("office:completionGatesDetailAfterWork");
    expect(
      criterionDetail(
        approval({ result_reason: "awaiting_criteria", result_detail: ["check", "review"] }),
        "unverified",
        [],
      ).waitingFor,
    ).toEqual([KIND_KEYS.check, KIND_KEYS.review]);
    const asked = criterionDetail(approval({ result_reason: "awaiting_person" }), "unverified", []);
    expect(asked.text?.key).toBe("office:completionGatesDetailAwaitingYou");
    expect(asked.commits).toEqual(["a1b2c3d"]);
    expect(
      criterionDetail(
        approval({
          result: "pass",
          result_reason: "approved",
          result_detail: ["checked the redirect"],
        }),
        undefined,
        [],
      ).text,
    ).toEqual({
      key: "office:completionGatesDetailApprovedNote",
      values: { reason: "checked the redirect" },
    });
  });

  it("shows an approval given at earlier heads as pending at the live heads", () => {
    const shown = criterionAtShownCommit(
      approval({ result: "pass", result_reason: "approved" }),
      "pr-1@ffffffffffff",
    );
    expect(shown).toMatchObject({
      result: "pending",
      result_subject_revision: "pr-1@ffffffffffff",
    });
  });

  it("approves with an optional note", () => {
    expect(REASON_REQUIRED_ACTIONS.has("approve")).toBe(false);
    expect(DIALOG_KEYS.approve.confirm).toBe(actionLabelKey("approve", "approval"));
    expect(KIND_KEYS.approval).toBe("office:completionGatesKindApproval");
  });

  it("selects an approval with only a rework limit", () => {
    const selection = {
      ...EMPTY_SELECTION,
      approval: true,
      reworkLimit: "2",
      verificationLimit: "4",
    };
    expect(criteriaFromSelection(selection)).toEqual([{ kind: "approval", rework_limit: 2 }]);
    expect(selectionFromCriteria([{ kind: "approval", rework_limit: 2 }])).toMatchObject({
      approval: true,
      reworkLimit: "2",
    });
  });
});
