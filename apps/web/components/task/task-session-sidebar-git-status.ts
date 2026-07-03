import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";

export function useBulkGitStatusSubscription(primarySessionIds: string[]) {
  const connectionStatus = useAppStore((state) => state.connection.status);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  useEffect(() => {
    if (connectionStatus !== "connected" || primarySessionIds.length === 0) return;
    const client = getWebSocketClient();
    if (!client) return;
    // Skip active session; it is already subscribed and focused by task page hooks.
    const backgroundIds = activeSessionId
      ? primarySessionIds.filter((id) => id !== activeSessionId)
      : primarySessionIds;
    const unsubscribes = backgroundIds.map((id) => client.subscribeSession(id));
    return () => unsubscribes.forEach((u) => u());
  }, [primarySessionIds, connectionStatus, activeSessionId]);
}
