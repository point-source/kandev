import { useEffect, useState } from "react";
import {
  getCompletionGateSettings,
  type CompletionGateSettings,
} from "@/lib/api/domains/task-completion-gates-api";

/** Loads a workspace's completion-gate settings; null until loaded or on error. */
export function useCompletionGateSettings(workspaceId: string | null) {
  const [settings, setSettings] = useState<CompletionGateSettings | null>(null);
  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    getCompletionGateSettings(workspaceId)
      .then((loaded) => {
        if (!cancelled) setSettings(loaded);
      })
      .catch(() => {
        if (!cancelled) setSettings(null);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);
  return { settings, setSettings };
}
