"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconPlayerPause } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import type {
  TaskCompletionCriterion,
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
  applyCriterionAction,
  limitField,
  type ActionValues,
} from "@/lib/completion-gates/person-actions";
import { timeAgo } from "@/lib/utils/time";

export const COMPLETION_GATE_INBOX_TYPES = new Set<string>(["completion_gate_hold"]);

function payloadString(item: InboxItem, key: string): string {
  const value = item.payload?.[key];
  return typeof value === "string" ? value : "";
}

function payloadNumber(item: InboxItem, key: string): number {
  const value = item.payload?.[key];
  return typeof value === "number" ? value : 0;
}

/** The held criterion an Inbox item describes. */
function heldCriterionFromItem(item: InboxItem): TaskCompletionCriterion {
  const detail = item.payload?.detail;
  return {
    id: payloadString(item, "criterion_id"),
    description: "",
    evidence_subject: { kind: "github_pr_heads", id: payloadString(item, "task_id") },
    criterion_revision: payloadNumber(item, "criterion_revision"),
    kind: payloadString(item, "criterion_kind") === "review" ? "review" : "check",
    hold_kind: payloadString(item, "hold_kind") as TaskCompletionHoldKind,
    held_at: item.createdAt,
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
  return (
    <div
      className="flex flex-col gap-2 px-4 py-2.5 hover:bg-accent/50 transition-colors sm:flex-row sm:items-center sm:gap-3"
      data-testid={`inbox-item-${item.type}`}
    >
      <div className="flex min-w-0 flex-1 items-center gap-3">
        <div className="h-8 w-8 rounded-full bg-muted flex items-center justify-center shrink-0">
          <IconPlayerPause className="h-4 w-4 text-muted-foreground" />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium" data-testid="inbox-completion-hold">
            {detail.text ? t(detail.text.key, detail.text.values) : item.title}
          </p>
          <Link
            href={`/office/tasks/${taskId}`}
            className="text-xs text-muted-foreground underline-offset-2 hover:underline cursor-pointer truncate block"
          >
            {item.title}
          </Link>
          <p className="text-xs text-muted-foreground">{timeAgo(item.createdAt)}</p>
        </div>
      </div>
      <InboxCompletionGateActions item={item} criterion={criterion} onChanged={onChanged} />
    </div>
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
  const [action, setAction] = useState<CriterionAction | null>(null);
  const apply = async (values: ActionValues) => {
    if (!action) return;
    await applyCriterionAction(
      {
        taskId: payloadString(item, "task_id"),
        criterionId: criterion.id,
        criterionRevision: criterion.criterion_revision,
        subjectRevision: criterion.result_subject_revision ?? "",
      },
      action,
      values,
    );
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
          onClick={() => setAction(candidate)}
        >
          {t(actionLabelKey(candidate, criterion.kind))}
        </Button>
      ))}
      {action && (
        <CompletionActionDialog
          open
          onOpenChange={(open) => !open && setAction(null)}
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
