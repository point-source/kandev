"use client";

import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import type {
  EffectiveCompletionCaps,
  TaskCompletionCriterion,
} from "@/lib/api/domains/task-completion-gates-api";
import {
  criterionDisplayStatus,
  isCapHold,
  isProtectedPathHold,
  type CriterionDisplayStatus,
} from "@/lib/completion-gates/criteria-display";
import { criterionDetail, type DecisionLike } from "@/lib/completion-gates/criterion-detail";
import { KIND_KEYS, STATUS_KEYS } from "@/lib/completion-gates/copy-keys";
import { CriterionDetailLine } from "./completion-criterion-detail";

const STATUS_CLASS: Record<CriterionDisplayStatus, string> = {
  pass: "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300",
  fail: "bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-300",
  held: "bg-amber-100 text-amber-800 dark:bg-amber-900/50 dark:text-amber-300",
  stale: "bg-amber-100 text-amber-800 dark:bg-amber-900/50 dark:text-amber-300",
  cannot_evaluate: "bg-muted text-muted-foreground",
  pending: "bg-muted text-muted-foreground",
};

export type CriterionAction = "allow" | "send_back" | "extend";

type Props = {
  criterion: TaskCompletionCriterion;
  blocker: string | undefined;
  caps?: EffectiveCompletionCaps;
  decisions: DecisionLike[];
  onAction: (action: CriterionAction, criterion: TaskCompletionCriterion) => void;
};

/** One criterion: name, status, what decided it at which commits, counts and actions. */
export function CompletionCriterionRow({ criterion, blocker, caps, decisions, onAction }: Props) {
  const { t } = useTranslation();
  const status = criterionDisplayStatus(criterion, blocker);
  return (
    <li
      className="flex flex-col gap-1.5 px-4 py-3"
      data-testid={`completion-criterion-${criterion.id}`}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium">
          {t(KIND_KEYS[criterion.kind ?? "attested"] ?? KIND_KEYS.attested)}
        </span>
        <Badge className={STATUS_CLASS[status]} data-testid="completion-criterion-status">
          {t(STATUS_KEYS[status])}
        </Badge>
      </div>
      <CriterionDetailLine detail={criterionDetail(criterion, blocker, decisions)} />
      <CriterionCounts criterion={criterion} caps={caps} />
      <CriterionActions criterion={criterion} onAction={onAction} />
    </li>
  );
}

function CriterionCounts({
  criterion,
  caps,
}: {
  criterion: TaskCompletionCriterion;
  caps?: EffectiveCompletionCaps;
}) {
  const { t } = useTranslation();
  return (
    <p className="text-xs text-muted-foreground" data-testid="completion-criterion-counts">
      {caps
        ? t("office:completionGatesCountsOfCaps", {
            rework: criterion.rework_count ?? 0,
            maxRework: caps.max_rework_rounds,
            attempts: criterion.attempt_count ?? 0,
            maxAttempts: caps.max_verification_attempts,
          })
        : t("office:completionGatesCounts", {
            rework: criterion.rework_count ?? 0,
            attempts: criterion.attempt_count ?? 0,
          })}
    </p>
  );
}

function CriterionActions({ criterion, onAction }: Pick<Props, "criterion" | "onAction">) {
  const { t } = useTranslation();
  if (isProtectedPathHold(criterion)) {
    return (
      <div className="flex flex-col gap-2 sm:flex-row">
        <Button
          size="sm"
          className="cursor-pointer min-h-11 sm:min-h-0"
          data-testid="completion-allow"
          onClick={() => onAction("allow", criterion)}
        >
          {t("office:completionGatesAllowForCommit")}
        </Button>
        <Button
          size="sm"
          variant="outline"
          className="cursor-pointer min-h-11 sm:min-h-0"
          data-testid="completion-send-back"
          onClick={() => onAction("send_back", criterion)}
        >
          {t("office:completionGatesSendBack")}
        </Button>
      </div>
    );
  }
  if (isCapHold(criterion)) {
    return (
      <div className="flex flex-col gap-2 sm:flex-row">
        <Button
          size="sm"
          className="cursor-pointer min-h-11 sm:min-h-0"
          data-testid="completion-extend"
          onClick={() => onAction("extend", criterion)}
        >
          {t("office:completionGatesAllowMoreAttempts")}
        </Button>
      </div>
    );
  }
  return null;
}
