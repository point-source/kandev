"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { toast } from "@/lib/toast/sonner";
import { useAppStore } from "@/components/state-provider";
import { CriteriaEditor } from "@/components/completion-gates/criteria-editor";
import { useCompletionGateSettings } from "@/hooks/domains/completion-gates/use-completion-gate-settings";
import { updateCompletionGateSettings } from "@/lib/api/domains/task-completion-gates-api";
import {
  criteriaFromSelection,
  EMPTY_SELECTION,
  invalidLimits,
  selectionFromCriteria,
  type CriteriaSelection,
} from "@/lib/completion-gates/criteria-selection";

type Limits = {
  defaultAttempts: string;
  defaultRework: string;
  defaultReviewWait: string;
  ceilingAttempts: string;
  ceilingRework: string;
};

const LIMIT_FIELDS: Array<{ key: keyof Limits; labelKey: string; group: "defaults" | "ceilings" }> =
  [
    { key: "defaultRework", labelKey: "office:completionGatesReworkLimit", group: "defaults" },
    {
      key: "defaultAttempts",
      labelKey: "office:completionGatesVerificationLimit",
      group: "defaults",
    },
    {
      key: "defaultReviewWait",
      labelKey: "office:completionGatesReviewWaitLimit",
      group: "defaults",
    },
    { key: "ceilingRework", labelKey: "office:completionGatesReworkLimit", group: "ceilings" },
    {
      key: "ceilingAttempts",
      labelKey: "office:completionGatesVerificationLimit",
      group: "ceilings",
    },
  ];

/** Workspace default criteria, default limits and ceilings for completion gates. */
export function CompletionGatesCard() {
  const { t } = useTranslation();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const { settings, setSettings } = useCompletionGateSettings(workspaceId);
  const [selection, setSelection] = useState<CriteriaSelection>(EMPTY_SELECTION);
  const [limits, setLimits] = useState<Limits>({
    defaultAttempts: "3",
    defaultRework: "3",
    defaultReviewWait: "",
    ceilingAttempts: "10",
    ceilingRework: "10",
  });
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    if (!settings) return;
    setSelection(selectionFromCriteria(settings.default_criteria));
    setLimits({
      defaultAttempts: String(settings.default_verification_limit),
      defaultRework: String(settings.default_rework_limit),
      defaultReviewWait: settings.default_review_wait_limit
        ? String(settings.default_review_wait_limit)
        : "",
      ceilingAttempts: String(settings.verification_limit_ceiling),
      ceilingRework: String(settings.rework_limit_ceiling),
    });
  }, [settings]);
  const payload = {
    default_criteria: criteriaFromSelection({
      ...selection,
      reworkLimit: "",
      verificationLimit: "",
      reviewWaitLimit: "",
    }),
    default_verification_limit: Number(limits.defaultAttempts),
    default_rework_limit: Number(limits.defaultRework),
    verification_limit_ceiling: Number(limits.ceilingAttempts),
    rework_limit_ceiling: Number(limits.ceilingRework),
    default_review_wait_limit: Number(limits.defaultReviewWait) || 0,
  };
  const invalid = invalidLimits(payload) || payload.default_review_wait_limit < 0;
  const save = async () => {
    if (!workspaceId || invalid) return;
    setSaving(true);
    try {
      setSettings(await updateCompletionGateSettings(workspaceId, payload));
      toast.success(t("office:completionGatesSettingsSaved"));
    } catch {
      toast.error(t("office:completionGatesSettingsSaveFailed"));
    } finally {
      setSaving(false);
    }
  };
  return (
    <div className="space-y-4" data-testid="completion-gates-card">
      <p className="text-xs text-muted-foreground">
        {t("office:completionGatesSettingsDescription")}
      </p>
      <div className="space-y-2">
        <p className="text-sm font-medium">{t("office:completionGatesSettingsDefaults")}</p>
        <CriteriaEditor
          idPrefix="settings-criteria"
          value={selection}
          onChange={setSelection}
          showLimits={false}
        />
      </div>
      <LimitGroup
        title={t("office:completionGatesSettingsDefaultLimits")}
        group="defaults"
        limits={limits}
        onChange={setLimits}
      />
      <LimitGroup
        title={t("office:completionGatesSettingsCeilings")}
        hint={t("office:completionGatesSettingsCeilingsHint")}
        group="ceilings"
        limits={limits}
        onChange={setLimits}
      />
      {invalid && (
        <p className="text-xs text-destructive">{t("office:completionGatesSettingsSaveFailed")}</p>
      )}
      <div className="flex justify-end">
        <Button
          className="w-full cursor-pointer min-h-11 sm:w-auto sm:min-h-0"
          data-testid="completion-gates-save"
          disabled={saving || invalid || !settings}
          onClick={save}
        >
          {t("office:completionGatesSettingsSave")}
        </Button>
      </div>
    </div>
  );
}

function LimitGroup(props: {
  title: string;
  hint?: string;
  group: "defaults" | "ceilings";
  limits: Limits;
  onChange: (limits: Limits) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{props.title}</p>
      {props.hint && <p className="text-xs text-muted-foreground">{props.hint}</p>}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        {LIMIT_FIELDS.filter((field) => field.group === props.group).map((field) => (
          <div key={field.key} className="flex flex-col gap-1.5">
            <Label htmlFor={`completion-gates-${field.key}`}>{t(field.labelKey)}</Label>
            <Input
              id={`completion-gates-${field.key}`}
              data-testid={`completion-gates-${field.key}`}
              type="number"
              min={field.key === "defaultReviewWait" ? 0 : 1}
              inputMode="numeric"
              className="min-h-11 sm:min-h-0"
              placeholder={
                field.key === "defaultReviewWait"
                  ? t("office:completionGatesReviewWaitLimitHint")
                  : undefined
              }
              value={props.limits[field.key]}
              onChange={(event) =>
                props.onChange({ ...props.limits, [field.key]: event.target.value })
              }
            />
          </div>
        ))}
      </div>
    </div>
  );
}

/** The settings-page section: header, card and the completion-gate editor. */
export function CompletionGatesSection() {
  const { t } = useTranslation();
  return (
    <div>
      <div className="mb-3 flex items-center gap-3">
        <h2 className="shrink-0 font-mono text-[10px] font-medium uppercase tracking-widest text-muted-foreground/60">
          {t("office:completionGatesSettingsTitle")}
        </h2>
        <div className="h-px flex-1 bg-border" />
      </div>
      <div className="rounded-lg border border-border p-4">
        <CompletionGatesCard />
      </div>
    </div>
  );
}
