"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Textarea } from "@kandev/ui/textarea";
import {
  officeEventsTextMatches,
  officeEventsToYaml,
  parseOfficeEventsYaml,
  type OfficeEventsError,
} from "@/lib/office-workflow/office-events-yaml";
import type { Translate } from "@/lib/office-workflow/problem-messages";
import type { WorkflowStep } from "@/lib/types/http";

type StageType = NonNullable<WorkflowStep["stage_type"]>;
// i18n-exempt: persisted stage_type values, not copy.
const STAGE_TYPES: StageType[] = ["work", "review", "approval", "custom", "hold"];

type OfficeStepControlsProps = {
  step: WorkflowStep;
  steps: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
};

function stageTypeLabel(t: Translate, stage: StageType): string {
  switch (stage) {
    case "work":
      return t("workflows:stageTypeWork");
    case "review":
      return t("workflows:stageTypeReview");
    case "approval":
      return t("workflows:stageTypeApproval");
    case "hold":
      return t("workflows:stageTypeHold");
    default:
      return t("workflows:stageTypeCustom");
  }
}

function StepStageTypeSelect({ step, onUpdate, readOnly }: Omit<OfficeStepControlsProps, "steps">) {
  const { t } = useTranslation();
  const id = `${step.id}-stage-type`;
  return (
    <div className="grid gap-1.5">
      <Label htmlFor={id}>{t("workflows:stageTypeLabel")}</Label>
      <Select
        value={step.stage_type ?? "custom"}
        onValueChange={(value) => onUpdate({ stage_type: value as StageType })}
        disabled={readOnly}
      >
        <SelectTrigger id={id} className="w-full sm:max-w-xs" data-testid="step-stage-type-select">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {STAGE_TYPES.map((stage) => (
            <SelectItem key={stage} value={stage}>
              {stageTypeLabel(t, stage)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">{t("workflows:stageTypeHelp")}</p>
    </div>
  );
}

function describeEventsError(t: Translate, error: OfficeEventsError): string {
  switch (error.code) {
    case "syntax":
      return t("workflows:officeEventsErrorSyntax", { line: error.line, message: error.message });
    case "notMapping":
      return t("workflows:officeEventsErrorNotMapping");
    case "unknownTrigger":
      return t("workflows:officeEventsErrorUnknownTrigger", { trigger: error.trigger });
    case "notList":
      return t("workflows:officeEventsErrorNotList", { trigger: error.trigger });
    case "badAction":
      return t("workflows:officeEventsErrorBadAction", {
        trigger: error.trigger,
        index: error.index + 1,
      });
    case "unknownAction":
      return t("workflows:officeEventsErrorUnknownAction", {
        trigger: error.trigger,
        action: error.action,
      });
    case "unknownStep":
      return t("workflows:officeEventsErrorUnknownStep", {
        trigger: error.trigger,
        step: error.step,
      });
    default:
      return t("workflows:officeEventsErrorDuplicateStepNames", { step: error.step });
  }
}

/**
 * Text for a step's Office events editor. Valid text is applied to the step
 * at once; text that does not parse is kept on the step as a draft, which
 * blocks saving, until it is fixed. Shown text follows the stored events
 * when a rename or discard makes the last valid text stale.
 */
function useOfficeEventsText(
  step: WorkflowStep,
  steps: WorkflowStep[],
  onUpdate: OfficeStepControlsProps["onUpdate"],
) {
  const [validText, setValidText] = useState(() => officeEventsToYaml(step.events, steps));
  const draft = step.office_events_draft;
  const draftResult = draft === undefined ? null : parseOfficeEventsYaml(draft, steps);
  useEffect(() => {
    // A draft can become valid without an edit, for example after a rename.
    if (draft !== undefined && draftResult?.ok) {
      setValidText(draft);
      onUpdate({ events: draftResult.events, office_events_draft: undefined });
    }
  }, [draft, draftResult?.ok]); // eslint-disable-line react-hooks/exhaustive-deps
  let text = draft ?? validText;
  if (draft === undefined && !officeEventsTextMatches(validText, steps, step.events)) {
    text = officeEventsToYaml(step.events, steps);
  }
  const error = draftResult && !draftResult.ok ? draftResult.error : null;
  const change = (next: string) => {
    const parsed = parseOfficeEventsYaml(next, steps);
    if (!parsed.ok) {
      onUpdate({ office_events_draft: next });
      return;
    }
    setValidText(next);
    onUpdate({ events: parsed.events, office_events_draft: undefined });
  };
  return { text, error, change };
}

function StepOfficeEventsEditor({ step, steps, onUpdate, readOnly }: OfficeStepControlsProps) {
  const { t } = useTranslation();
  const { text, error, change } = useOfficeEventsText(step, steps, onUpdate);
  const id = `${step.id}-office-events`;
  return (
    <details className="rounded-md border p-3" data-testid="office-events-editor">
      <summary className="cursor-pointer text-sm font-medium">
        {t("workflows:officeEventsTitle")}
      </summary>
      <div className="mt-2 grid gap-1.5">
        <Label htmlFor={id} className="sr-only">
          {t("workflows:officeEventsTitle")}
        </Label>
        <p className="text-xs text-muted-foreground">{t("workflows:officeEventsHelp")}</p>
        <Textarea
          id={id}
          value={text}
          onChange={(event) => change(event.target.value)}
          disabled={readOnly}
          spellCheck={false}
          rows={Math.min(24, Math.max(6, text.split("\n").length + 1))}
          className="min-w-0 whitespace-pre font-mono text-xs"
          aria-invalid={error !== null}
          data-testid="office-events-textarea"
        />
        {error && (
          <p
            className="break-words text-xs text-destructive"
            role="alert"
            data-testid="office-events-error"
          >
            {describeEventsError(t, error)}
          </p>
        )}
      </div>
    </details>
  );
}

/** Stage type and Office events editing for a step of an Office workflow. */
export function OfficeStepControls(props: OfficeStepControlsProps) {
  return (
    <div className="space-y-3" data-testid="office-step-controls">
      <StepStageTypeSelect step={props.step} onUpdate={props.onUpdate} readOnly={props.readOnly} />
      <StepOfficeEventsEditor {...props} />
    </div>
  );
}
