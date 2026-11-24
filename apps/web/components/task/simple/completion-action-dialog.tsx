"use client";

import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { actionErrorKey } from "@/lib/completion-gates/copy-keys";

export type CompletionActionValues = { reason: string; rework: number; attempts: number };

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  confirmLabel: string;
  reasonRequired?: boolean;
  /** The limit an extension raises: rework rounds or verification attempts. */
  limit?: "rework" | "attempts";
  testId: string;
  onConfirm: (values: CompletionActionValues) => Promise<void>;
};

/** One responsive confirmation surface for every completion-gate action. */
export function CompletionActionDialog(props: Props) {
  const { isMobile } = useResponsiveBreakpoint();
  const body = <ActionBody {...props} />;
  if (isMobile) {
    return (
      <Drawer open={props.open} onOpenChange={props.onOpenChange}>
        <DrawerContent data-testid={props.testId}>
          <DrawerHeader>
            <DrawerTitle>{props.title}</DrawerTitle>
            <DrawerDescription>{props.description}</DrawerDescription>
          </DrawerHeader>
          {body}
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent data-testid={props.testId}>
        <DialogHeader>
          <DialogTitle>{props.title}</DialogTitle>
          <DialogDescription>{props.description}</DialogDescription>
        </DialogHeader>
        {body}
      </DialogContent>
    </Dialog>
  );
}

function ActionBody({ reasonRequired, limit, confirmLabel, onConfirm, onOpenChange }: Props) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [reason, setReason] = useState("");
  const [limitValue, setLimitValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState<string | null>(null);
  const raised = Number(limitValue) || 0;
  const limits = {
    rework: limit === "rework" ? raised : 0,
    attempts: limit === "attempts" ? raised : 0,
  };
  const invalid = (reasonRequired && reason.trim() === "") || (limit !== undefined && raised <= 0);

  const submit = async () => {
    setBusy(true);
    setFailed(null);
    try {
      await onConfirm({ reason: reason.trim(), ...limits });
      onOpenChange(false);
    } catch (error) {
      setFailed(actionErrorKey(error));
    } finally {
      setBusy(false);
    }
  };

  const Footer = isMobile ? DrawerFooter : DialogFooter;
  const controlClass = isMobile ? "min-h-11" : undefined;
  return (
    <div className="flex flex-col gap-3 px-4 pb-2 sm:px-0">
      {limit && (
        <LimitField
          id={`completion-limit-${limit}`}
          label={t(
            limit === "rework"
              ? "office:completionGatesReworkLimit"
              : "office:completionGatesVerificationLimit",
          )}
          value={limitValue}
          onChange={setLimitValue}
          className={controlClass}
        />
      )}
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="completion-action-reason">
          {reasonRequired
            ? t("office:completionGatesReasonRequired")
            : t("office:completionGatesReasonOptional")}
        </Label>
        <Textarea
          id="completion-action-reason"
          data-testid="completion-action-reason"
          value={reason}
          onChange={(event) => setReason(event.target.value)}
        />
      </div>
      {failed && (
        <p role="alert" className="text-sm text-destructive">
          {t(failed)}
        </p>
      )}
      <Footer>
        <Button
          className={`cursor-pointer ${controlClass ?? ""}`}
          data-testid="completion-action-confirm"
          disabled={busy || invalid}
          onClick={submit}
        >
          {confirmLabel}
        </Button>
      </Footer>
    </div>
  );
}

function LimitField(props: {
  id: string;
  label: ReactNode;
  value: string;
  onChange: (value: string) => void;
  className?: string;
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
        className={props.className}
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
      />
    </div>
  );
}
