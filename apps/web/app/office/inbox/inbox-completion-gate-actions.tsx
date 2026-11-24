"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconPlayerPause, IconUserCheck } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import type {
  TaskCompletionCriterion,
  TaskCompletionCriterionKind,
  TaskCompletionHoldKind,
} from "@/lib/api/domains/task-completion-gates-api";
import type { InboxItem } from "@/lib/state/slices/office/types";
import { CompletionActionDialog } from "@/components/task/simple/completion-action-dialog";
import { criterionActions, type CriterionAction } from "@/lib/completion-gates/criteria-display";
import { criterionDetail } from "@/lib/completion-gates/criterion-detail";
import {
  actionLabelKey,
  DIALOG_KEYS,
  REASON_REQUIRED_ACTIONS,
} from "@/lib/completion-gates/copy-keys";
import {
  actionTarget,
  applyCriterionAction,
  limitField,
  type ActionTarget,
  type ActionValues,
} from "@/lib/completion-gates/person-actions";
import { timeAgo } from "@/lib/utils/time";

const APPROVAL_INBOX_TYPE = "completion_gate_approval";

export const COMPLETION_GATE_INBOX_TYPES = new Set<string>([
  "completion_gate_hold",
  APPROVAL_INBOX_TYPE,
]);

const CRITERION_KINDS = new Set<string>(["check", "review", "approval"]);

function payloadString(item: InboxItem, key: string): string {
  const value = item.payload?.[key];
  return typeof value === "string" ? value : "";
}

function payloadNumber(item: InboxItem, key: string): number {
  const value = item.payload?.[key];
  return typeof value === "number" ? value : 0;
}

/** The criterion waiting for a person that an Inbox item describes. */
function heldCriterionFromItem(item: InboxItem): TaskCompletionCriterion {
  const detail = item.payload?.detail;
  const kind = payloadString(item, "criterion_kind");
  const approval = item.type === APPROVAL_INBOX_TYPE;
  return {
    id: payloadString(item, "criterion_id"),
    description: "",
    evidence_subject: { kind: "github_pr_heads", id: payloadString(item, "task_id") },
    criterion_revision: payloadNumber(item, "criterion_revision"),
    kind: (CRITERION_KINDS.has(kind) ? kind : "check") as TaskCompletionCriterionKind,
    hold_kind: (payloadString(item, "hold_kind") || undefined) as
      | TaskCompletionHoldKind
      | undefined,
    result_reason: approval ? "awaiting_person" : undefined,
    held_at: payloadString(item, approval ? "asked_at" : "held_at") || item.createdAt,
    result_detail: Array.isArray(detail) ? detail.map(String) : [],
    result_subject_revision: payloadString(item, "subject_revision"),
    rework_count: payloadNumber(item, "rework_count"),
    attempt_count: payloadNumber(item, "attempt_count"),
  };
}

/** One held criterion in the Inbox: the hold, its task, and the person's actions. */
export function InboxCompletionGateRow({
  item,
  onChanged,
}: {
  item: InboxItem;
  onChanged?: () => void;
}) {
  const { t } = useTranslation();
  const criterion = heldCriterionFromItem(item);
  const detail = criterionDetail(criterion, "held", []);
  const taskId = payloadString(item, "task_id");
  const approval = item.type === APPROVAL_INBOX_TYPE;
  const Icon = approval ? IconUserCheck : IconPlayerPause;
  const headline = approval
    ? t("office:completionGatesInboxApprovalNeeded")
    : detail.text && t(detail.text.key, detail.text.values);
  return (
    <div
      className="flex flex-col gap-2 px-4 py-2.5 hover:bg-accent/50 transition-colors sm:flex-row sm:items-center sm:gap-3"
      data-testid={`inbox-item-${item.type}`}
    >
      <div className="flex min-w-0 flex-1 items-center gap-3">
        <div className="h-8 w-8 rounded-full bg-muted flex items-center justify-center shrink-0">
          <Icon className="h-4 w-4 text-muted-foreground" />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium" data-testid="inbox-completion-hold">
            {headline || item.title}
          </p>
          <Link
            href={`/office/tasks/${taskId}`}
            className="text-xs text-muted-foreground underline-offset-2 hover:underline cursor-pointer truncate block"
          >
            {item.title}
          </Link>
          <HeldSince heldAt={criterion.held_at ?? ""} commits={detail.commits} />
        </div>
      </div>
      <InboxCompletionGateActions item={item} criterion={criterion} onChanged={onChanged} />
    </div>
  );
}

/** When the hold began and the commits it holds at. */
function HeldSince({ heldAt, commits }: { heldAt: string; commits: string[] }) {
  const when = timeAgo(heldAt);
  const shown = commits.join(", ");
  return (
    <p className="flex gap-1.5 text-xs text-muted-foreground">
      {when && <span>{when}</span>}
      {when && shown && <span aria-hidden="true">·</span>}
      {shown && (
        <span className="font-mono" data-testid="inbox-completion-commits">
          {shown}
        </span>
      )}
    </p>
  );
}

function InboxCompletionGateActions({
  item,
  criterion,
  onChanged,
}: {
  item: InboxItem;
  criterion: TaskCompletionCriterion;
  onChanged?: () => void;
}) {
  const { t } = useTranslation();
  // The target is captured when the dialog opens, so a refresh of the row
  // while it is open cannot redirect the action to another commit.
  const [pending, setPending] = useState<{ action: CriterionAction; target: ActionTarget } | null>(
    null,
  );
  const action = pending?.action;
  const apply = async (values: ActionValues) => {
    if (!pending) return;
    await applyCriterionAction(pending.target, pending.action, values);
    onChanged?.();
  };
  return (
    <div className="flex shrink-0 flex-col gap-2 sm:flex-row">
      {criterionActions(criterion, "inbox").map((candidate, index) => (
        <Button
          key={candidate}
          size="sm"
          variant={index === 0 ? "default" : "outline"}
          className="cursor-pointer min-h-11 sm:min-h-0"
          data-testid={`inbox-completion-${candidate}`}
          onClick={() =>
            setPending({
              action: candidate,
              target: actionTarget(payloadString(item, "task_id"), criterion),
            })
          }
        >
          {t(actionLabelKey(candidate, criterion.kind))}
        </Button>
      ))}
      {action && (
        <CompletionActionDialog
          open
          onOpenChange={(open) => !open && setPending(null)}
          title={t(DIALOG_KEYS[action].title)}
          description={t(DIALOG_KEYS[action].description)}
          confirmLabel={t(DIALOG_KEYS[action].confirm)}
          reasonRequired={REASON_REQUIRED_ACTIONS.has(action)}
          limit={limitField(action)}
          testId={`completion-dialog-${action}`}
          onConfirm={apply}
        />
      )}
    </div>
  );
}
