"use client";

import { useTranslation } from "react-i18next";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import AppLink from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";

/**
 * Non-blocking warning shown where people act on completion criteria while
 * authentication is off: any process on the machine can act as the person.
 */
export function GateActionsAuthWarning({ className }: { className?: string }) {
  const { t } = useTranslation();
  const authMode = useAppStore((state) => state.auth.mode);
  if (authMode === "enabled") return null;
  return (
    <Alert
      data-testid="completion-auth-warning"
      className={`border-amber-300 bg-amber-50 text-amber-900 dark:bg-amber-950/40 dark:text-amber-200 ${className ?? ""}`}
    >
      <AlertDescription className="text-xs">
        {t("office:completionGatesAuthWarning")}{" "}
        <AppLink href="/settings/system/feature-toggles" className="underline cursor-pointer">
          {t("office:completionGatesAuthWarningLink")}
        </AppLink>
      </AlertDescription>
    </Alert>
  );
}
