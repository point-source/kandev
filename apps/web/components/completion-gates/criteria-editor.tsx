"use client";

import { useTranslation } from "react-i18next";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import type { CriteriaSelection } from "@/lib/completion-gates/criteria-selection";

type Props = {
  value: CriteriaSelection;
  onChange: (value: CriteriaSelection) => void;
  idPrefix: string;
  showLimits?: boolean;
};

/** Check and review criteria with protected paths and limits. */
export function CriteriaEditor({ value, onChange, idPrefix, showLimits = true }: Props) {
  const { t } = useTranslation();
  const set = (patch: Partial<CriteriaSelection>) => onChange({ ...value, ...patch });
  return (
    <div className="flex flex-col gap-3" data-testid={`${idPrefix}-criteria-editor`}>
      <CheckRow
        id={`${idPrefix}-ci`}
        checked={value.ci}
        onChange={(ci) => set({ ci })}
        label={t("office:completionGatesKindCheckGithub")}
      />
      {value.ci && (
        <div className="flex flex-col gap-1.5 pl-7">
          <Label htmlFor={`${idPrefix}-protected`}>
            {t("office:completionGatesProtectedPaths")}
          </Label>
          <Input
            id={`${idPrefix}-protected`}
            data-testid={`${idPrefix}-protected`}
            className="min-h-11 sm:min-h-0"
            value={value.protectedPaths}
            placeholder={t("office:completionGatesProtectedPathsHint")}
            onChange={(event) => set({ protectedPaths: event.target.value })}
          />
        </div>
      )}
      <CheckRow
        id={`${idPrefix}-review`}
        checked={value.review}
        onChange={(review) => set({ review })}
        label={t("office:completionGatesKindReview")}
      />
      {showLimits && value.review && (
        <div className="flex flex-col gap-1.5 pl-7">
          <NumberField
            id={`${idPrefix}-review-wait`}
            label={t("office:completionGatesReviewWaitLimit")}
            value={value.reviewWaitLimit}
            placeholder={t("office:completionGatesReviewWaitLimitHint")}
            onChange={(reviewWaitLimit) => set({ reviewWaitLimit })}
          />
        </div>
      )}
      {showLimits && (value.ci || value.review) && (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <NumberField
            id={`${idPrefix}-rework-limit`}
            label={t("office:completionGatesReworkLimit")}
            value={value.reworkLimit}
            onChange={(reworkLimit) => set({ reworkLimit })}
          />
          <NumberField
            id={`${idPrefix}-verification-limit`}
            label={t("office:completionGatesVerificationLimit")}
            value={value.verificationLimit}
            onChange={(verificationLimit) => set({ verificationLimit })}
          />
        </div>
      )}
    </div>
  );
}

function CheckRow(props: {
  id: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: string;
}) {
  return (
    <div className="flex min-h-11 items-center gap-3 sm:min-h-0">
      <Checkbox
        id={props.id}
        data-testid={props.id}
        checked={props.checked}
        onCheckedChange={(checked) => props.onChange(checked === true)}
        className="cursor-pointer"
      />
      <Label htmlFor={props.id} className="cursor-pointer text-sm">
        {props.label}
      </Label>
    </div>
  );
}

function NumberField(props: {
  id: string;
  label: string;
  value: string;
  placeholder?: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={props.id}>{props.label}</Label>
      <Input
        id={props.id}
        data-testid={props.id}
        type="number"
        min={1}
        inputMode="numeric"
        className="min-h-11 sm:min-h-0"
        value={props.value}
        placeholder={props.placeholder}
        onChange={(event) => props.onChange(event.target.value)}
      />
    </div>
  );
}
