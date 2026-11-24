"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type {
  TaskCompletionCriterion,
  TaskCompletionGateSnapshot,
} from "@/lib/api/domains/task-completion-gates-api";
import {
  completionGateRefreshKey,
  criterionActions,
  criterionAtShownCommit,
  criterionBlocker,
  overriddenBy,
  type CriterionAction,
} from "@/lib/completion-gates/criteria-display";
import {
  DIALOG_KEYS,
  REASON_REQUIRED_ACTIONS,
  type CompletionDialogAction,
} from "@/lib/completion-gates/copy-keys";
import {
  actionTarget,
  applyCriterionAction,
  limitField,
} from "@/lib/completion-gates/person-actions";
import { GateActionsAuthWarning } from "@/components/completion-gates/gate-actions-auth-warning";
import type { DecisionLike } from "@/lib/completion-gates/criterion-detail";
import {
  overrideTaskCompletion,
  useTaskCompletionGate,
} from "@/hooks/domains/completion-gates/use-task-completion-gate";
import type { TaskStatusSummaryCompletionGate } from "@/lib/types/task-status-summary";
import { CompletionActionDialog, type CompletionActionValues } from "./completion-action-dialog";
import { CompletionCriterionRow } from "./completion-criterion-row";

type SectionTask = {
  id: string;
  status: string;
  updatedAt: string;
  statusSummary?: { completion_gate?: TaskStatusSummaryCompletionGate | null } | null;
  decisions?: DecisionLike[];
};

type PendingAction = { action: CompletionDialogAction; criterion?: TaskCompletionCriterion };

/** Completion criteria of a gated Office task, with human recovery actions. */
export function CompletionCriteriaSection({ task }: { task: SectionTask }) {
  const summary = task.statusSummary?.completion_gate;
  const enabled = (summary?.criteria_count ?? 0) > 0;
  const refreshKey = completionGateRefreshKey(task.status, summary);
  const { gate, history, limits, reload } = useTaskCompletionGate(task.id, enabled, refreshKey);
  const [pending, setPending] = useState<PendingAction | null>(null);
  if (!enabled || !gate || gate.criteria.length === 0) return null;
  const done = task.status === "done";
  const personActions =
    gate.blocked ||
    gate.criteria.some(
      (item) =>
        criterionActions(criterionAtShownCommit(item, gate.heads_revision), "task").length > 0,
    );
  return (
    <section
      className="mt-4 rounded-lg border border-border"
      data-testid="completion-criteria-section"
    >
      <SectionHeader done={done} overriddenBy={done ? overriddenBy(history) : undefined} />
      {personActions && !done && <GateActionsAuthWarning className="mx-4 mt-3" />}
      <ul className="divide-y divide-border">
        {gate.criteria.map((criterion) => (
          <CompletionCriterionRow
            key={criterion.id}
            criterion={criterionAtShownCommit(criterion, gate.heads_revision)}
            blocker={criterionBlocker(gate, criterion.id)}
            limits={limits.find((entry) => entry.criterion_id === criterion.id)}
            decisions={task.decisions ?? []}
            onAction={(action, target) => setPending({ action, criterion: target })}
          />
        ))}
      </ul>
      {gate.blocked && !done && (
        <OverrideButton onClick={() => setPending({ action: "override" })} />
      )}
      <PendingActionDialog
        taskId={task.id}
        gate={gate}
        pending={pending}
        onClose={() => setPending(null)}
        onDone={reload}
      />
    </section>
  );
}

function SectionHeader(props: { done: boolean; overriddenBy?: string }) {
  const { t } = useTranslation();
  let summary = "";
  if (props.done) {
    summary = props.overriddenBy
      ? t("office:completionGatesOverridden", { actor: props.overriddenBy })
      : t("office:completionGatesDone");
  }
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-2.5">
      <h2 className="text-sm font-semibold">{t("office:completionGatesTitle")}</h2>
      {summary && (
        <span className="text-xs text-muted-foreground" data-testid="completion-criteria-summary">
          {summary}
        </span>
      )}
    </div>
  );
}

function OverrideButton({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex justify-end border-t border-border px-4 py-2.5">
      <Button
        size="sm"
        variant="outline"
        className="w-full cursor-pointer min-h-11 sm:w-auto sm:min-h-0"
        data-testid="completion-override"
        onClick={onClick}
      >
        {t("office:completionGatesOverrideCompletion")}
      </Button>
    </div>
  );
}

type DialogProps = {
  taskId: string;
  gate: TaskCompletionGateSnapshot;
  pending: PendingAction | null;
  onClose: () => void;
  onDone: () => Promise<void>;
};

/** Opens the confirmation for the chosen action and applies it. */
export function PendingActionDialog({ taskId, gate, pending, onClose, onDone }: DialogProps) {
  const { t } = useTranslation();
  if (!pending) return null;
  const action = pending.action;
  const apply = async (values: CompletionActionValues) => {
    await applyCompletionAction(taskId, gate, pending, values);
    await onDone();
  };
  return (
    <CompletionActionDialog
      open
      onOpenChange={(open) => !open && onClose()}
      title={t(DIALOG_KEYS[action].title)}
      description={t(DIALOG_KEYS[action].description)}
      confirmLabel={t(DIALOG_KEYS[action].confirm)}
      reasonRequired={REASON_REQUIRED_ACTIONS.has(action)}
      limit={limitField(action)}
      testId={`completion-dialog-${action}`}
      onConfirm={apply}
    />
  );
}

async function applyCompletionAction(
  taskId: string,
  gate: TaskCompletionGateSnapshot,
  pending: PendingAction,
  values: CompletionActionValues,
) {
  const criterion = pending.criterion;
  if (pending.action === "override") {
    await overrideTaskCompletion(taskId, gate.revision, values.reason);
    return;
  }
  if (!criterion) return;
  await applyCriterionAction(
    actionTarget(taskId, criterion),
    pending.action as CriterionAction,
    values,
  );
}
