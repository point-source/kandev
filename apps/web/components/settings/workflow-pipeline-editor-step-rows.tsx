"use client";

import { useTranslation } from "react-i18next";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import type { WorkflowStep } from "@/lib/types/http";
import { HelpTip, stageTypeOnHoldToggle } from "./workflow-pipeline-editor-helpers";
import { isWorkflowStepValueDirty } from "./workflow-dirty-state";
import { settingsControlClassName } from "./settings-control";

type StepAutoArchiveRowProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
};

export function StepAutoArchiveRow({
  step,
  savedStep,
  onUpdate,
  readOnly,
}: StepAutoArchiveRowProps) {
  const { t } = useTranslation();
  const isDirty = isWorkflowStepValueDirty(
    step,
    savedStep,
    (item) => item.auto_archive_after_hours ?? 0,
  );
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Checkbox
        id={`${step.id}-auto-archive`}
        checked={(step.auto_archive_after_hours ?? 0) > 0}
        onCheckedChange={(checked) => {
          if (readOnly) return;
          onUpdate({ auto_archive_after_hours: checked ? 24 : 0 });
        }}
        disabled={readOnly}
        data-settings-dirty={isDirty}
      />
      <Label htmlFor={`${step.id}-auto-archive`} className="text-sm">
        {t("workflows:autoArchive")}
      </Label>
      <HelpTip text={t("workflows:autoArchiveHelp")} />
      {(step.auto_archive_after_hours ?? 0) > 0 && (
        <>
          <span className="text-sm text-muted-foreground">{t("workflows:autoArchiveAfter")}</span>
          <Input
            id={`${step.id}-auto-archive-hours`}
            type="number"
            min={1}
            className={settingsControlClassName("w-20 text-sm")}
            value={step.auto_archive_after_hours ?? 24}
            onChange={(e) => {
              if (readOnly) return;
              const val = parseInt(e.target.value, 10);
              onUpdate({ auto_archive_after_hours: isNaN(val) || val < 1 ? 1 : val });
            }}
            disabled={readOnly}
            data-settings-dirty={isDirty}
          />
          <span className="text-sm text-muted-foreground">{t("workflows:autoArchiveHours")}</span>
        </>
      )}
    </div>
  );
}

type StepCheckboxRowProps = {
  id: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled: boolean;
  label: string;
  helpText: string;
  isDirty: boolean;
};

export function StepCheckboxRow({
  id,
  checked,
  onCheckedChange,
  disabled,
  label,
  helpText,
  isDirty,
}: StepCheckboxRowProps) {
  return (
    <div className="flex items-center gap-2">
      <Checkbox
        id={id}
        checked={checked}
        onCheckedChange={(v) => onCheckedChange(v === true)}
        disabled={disabled}
        data-settings-dirty={isDirty}
      />
      <Label htmlFor={id} className="text-sm">
        {label}
      </Label>
      <HelpTip text={helpText} />
    </div>
  );
}

type StepHoldRowProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
};

// The step's hold nature, stored as stage_type. Consumers read the stored value
// instead of the step's name, so renaming a hold step keeps it a hold and naming
// a step "hold" does not make it one.
export function StepHoldRow({ step, savedStep, onUpdate, readOnly }: StepHoldRowProps) {
  const { t } = useTranslation();
  const stageType = step.stage_type ?? "custom";
  return (
    <StepCheckboxRow
      id={`${step.id}-hold-step`}
      checked={stageType === "hold"}
      onCheckedChange={(checked) =>
        !readOnly && onUpdate({ stage_type: stageTypeOnHoldToggle(checked, savedStep?.stage_type) })
      }
      disabled={readOnly}
      label={t("workflows:holdStep")}
      helpText={t("workflows:holdStepHelp")}
      isDirty={isWorkflowStepValueDirty(step, savedStep, (item) => item.stage_type ?? "custom")}
    />
  );
}
