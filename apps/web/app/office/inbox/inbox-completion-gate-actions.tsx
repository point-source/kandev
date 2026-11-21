"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  decideProtectedPaths,
  extendCompletionCriterion,
} from "@/lib/api/domains/task-completion-gates-api";
import type { InboxItem } from "@/lib/state/slices/office/types";
import {
  CompletionActionDialog,
  type CompletionActionValues,
} from "@/components/task/simple/completion-action-dialog";
import { DIALOG_KEYS } from "@/lib/completion-gates/copy-keys";

type Action = "allow" | "send_back" | "extend";

const INBOX_ACTION_KEYS: Record<Action, string> = {
  allow: "office:completionGatesInboxAllow",
  send_back: "office:completionGatesSendBack",
  extend: "office:completionGatesAllowMoreAttempts",
};

export const COMPLETION_GATE_INBOX_TYPES = new Set<string>([
  "completion_gate_protected_path",
  "completion_gate_held",
]);

function payloadString(item: InboxItem, key: string): string {
  const value = item.payload?.[key];
  return typeof value === "string" ? value : "";
}

function payloadNumber(item: InboxItem, key: string): number {
  const value = item.payload?.[key];
  return typeof value === "number" ? value : 0;
}

/** Inbox actions for a held completion criterion; same effect as the task page. */
export function InboxCompletionGateActions({
  item,
  onChanged,
}: {
  item: InboxItem;
  onChanged?: () => void;
}) {
  const { t } = useTranslation();
  const [action, setAction] = useState<Action | null>(null);
  const actions: Action[] =
    item.type === "completion_gate_protected_path" ? ["allow", "send_back"] : ["extend"];
  const apply = async (values: CompletionActionValues) => {
    const taskId = payloadString(item, "task_id");
    const criterionId = payloadString(item, "criterion_id");
    const revision = payloadNumber(item, "criterion_revision");
    if (action === "extend") {
      await extendCompletionCriterion(taskId, criterionId, {
        expected_criterion_revision: revision,
        max_rework_rounds: values.rework || undefined,
        max_verification_attempts: values.attempts || undefined,
        reason: values.reason,
      });
    } else if (action) {
      await decideProtectedPaths(taskId, criterionId, {
        action,
        expected_criterion_revision: revision,
        expected_subject_revision: payloadString(item, "subject_revision"),
        reason: values.reason,
      });
    }
    onChanged?.();
  };
  return (
    <div className="flex shrink-0 gap-2">
      {actions.map((candidate) => (
        <Button
          key={candidate}
          size="sm"
          variant={candidate === "send_back" ? "outline" : "default"}
          className="cursor-pointer min-h-11 sm:min-h-0"
          data-testid={`inbox-completion-${candidate}`}
          onClick={() => setAction(candidate)}
        >
          {t(INBOX_ACTION_KEYS[candidate])}
        </Button>
      ))}
      {action && (
        <CompletionActionDialog
          open
          onOpenChange={(open) => !open && setAction(null)}
          title={t(DIALOG_KEYS[action].title)}
          description={t(DIALOG_KEYS[action].description)}
          confirmLabel={t(DIALOG_KEYS[action].confirm)}
          reasonRequired={action === "send_back"}
          withCaps={action === "extend"}
          testId={`completion-dialog-${action}`}
          onConfirm={apply}
        />
      )}
    </div>
  );
}
