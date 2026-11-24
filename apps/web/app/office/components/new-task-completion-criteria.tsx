"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { CriteriaEditor } from "@/components/completion-gates/criteria-editor";
import { useCompletionGateSettings } from "@/hooks/domains/completion-gates/use-completion-gate-settings";
import {
  EMPTY_SELECTION,
  selectionFromCriteria,
  type CriteriaSelection,
} from "@/lib/completion-gates/criteria-selection";
import type { IssueDraft } from "./new-task-draft";

type Props = {
  workspaceId: string | null;
  draft: IssueDraft;
  onUpdate: (patch: Partial<IssueDraft>) => void;
};

/** Collapsed completion-criteria group, prefilled from the workspace default. */
export function NewTaskCompletionCriteria({ workspaceId, draft, onUpdate }: Props) {
  const { t } = useTranslation();
  const { settings } = useCompletionGateSettings(workspaceId);
  const [open, setOpen] = useState(false);
  const defaults = settings ? selectionFromCriteria(settings.default_criteria) : EMPTY_SELECTION;
  const selection: CriteriaSelection = draft.completionCriteria ?? defaults;
  const summary = [
    selection.ci ? t("office:completionGatesKindCheckGithub") : null,
    selection.review ? t("office:completionGatesKindReview") : null,
    selection.approval ? t("office:completionGatesKindApproval") : null,
  ].filter(Boolean);
  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="rounded-md border border-border"
      data-testid="new-task-completion-criteria"
    >
      <CollapsibleTrigger className="flex min-h-11 w-full cursor-pointer items-center justify-between gap-2 px-3 py-2 text-left text-sm sm:min-h-0">
        <span>
          {t("office:completionGatesTitle")}
          {summary.length > 0 && (
            <span className="text-muted-foreground"> ({summary.join(", ")})</span>
          )}
        </span>
        <IconChevronDown
          className={`h-4 w-4 shrink-0 transition-transform ${open ? "rotate-180" : ""}`}
        />
      </CollapsibleTrigger>
      <CollapsibleContent className="px-3 pb-3">
        <CriteriaEditor
          idPrefix="new-task-criteria"
          value={selection}
          onChange={(completionCriteria) => onUpdate({ completionCriteria })}
        />
      </CollapsibleContent>
    </Collapsible>
  );
}
