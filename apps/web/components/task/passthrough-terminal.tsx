"use client";

import React, { useRef, useCallback, useEffect, useMemo, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { AttachAddon } from "@xterm/addon-attach";
import { WebglAddon } from "@xterm/addon-webgl";
import "@xterm/xterm/css/xterm.css";
import { GridSpinner } from "@/components/grid-spinner";
import { useAppStore } from "@/components/state-provider";
import { useSession } from "@/hooks/domains/session/use-session";
import { useSessionAgentctl } from "@/hooks/domains/session/use-session-agentctl";
import { getBackendConfig } from "@/lib/config";
import { useTerminalLinkHandler } from "@/hooks/use-terminal-link-handler";
import { buildTerminalFontFamily } from "@/lib/terminal/terminal-font";
import {
  MIN_WIDTH,
  MIN_HEIGHT,
  useTerminalInit,
  useWebSocketConnection,
  useSendResize,
  useSendInput,
  useFitAndResize,
} from "./use-passthrough-terminal";
import { useEnvironmentSessionId } from "@/hooks/use-environment-session-id";
import { useTerminalSearch } from "./use-terminal-search";
import { TerminalSearchBar } from "./terminal-search-bar";
import { usePanelSearch } from "@/hooks/use-panel-search";

type BaseProps = {
  autoFocus?: boolean;
  pendingCommand?: string | null;
  onCommandSent?: () => void;
  /** Called once the xterm instance is created. Mobile uses this to register
   * the active key-bar input target. Desktop ignores. */
  onXtermReady?: (xterm: Terminal) => void;
  /** Skip the WebGL renderer addon and use xterm's canvas renderer instead.
   * WebGL miscomputes glyph atlas scaling on mobile (notably after a
   * desktop→mobile responsive switch) and renders text at multiples of the
   * intended font size. Mobile callers should pass true. */
  disableWebgl?: boolean;
  /** When true, the AttachAddon is configured receive-only — incoming PTY
   * data still flows to the terminal display, but the consumer is responsible
   * for forwarding xterm.onData to the WebSocket. Mobile sets this so the
   * key-bar's modifier transforms can run before bytes go on the wire. */
  manualInputRouting?: boolean;
  /** Fires when the dedicated terminal WebSocket reaches the OPEN state.
   * Mobile uses this to register a key-bar sender that writes raw bytes
   * directly to this terminal's socket. */
  onWsReady?: (ws: WebSocket) => void;
};
type AgentTerminalProps = BaseProps & { mode: "agent"; sessionId?: string | null; label?: string };
type ShellTerminalProps = BaseProps & {
  mode: "shell";
  environmentId: string | null | undefined;
  terminalId: string;
  label?: string;
};
type PassthroughTerminalProps = AgentTerminalProps | ShellTerminalProps;

/**
 * PassthroughTerminal provides direct terminal interaction with an agent CLI.
 *
 * Design: Dedicated Binary WebSocket + AttachAddon
 * - Uses dedicated WebSocket routes for session-scoped agent terminals and env-scoped shells
 * - Raw binary frames bypass JSON encoding/decoding latency
 * - AttachAddon (official xterm.js addon) handles the bridging
 * - Unicode11Addon enables proper unicode character support
 * - Resize commands sent via binary protocol: [0x01][JSON {cols, rows}]
 */
function useTerminalRefs() {
  return {
    terminalRef: useRef<HTMLDivElement>(null),
    xtermRef: useRef<Terminal | null>(null),
    fitAddonRef: useRef<FitAddon | null>(null),
    wsRef: useRef<WebSocket | null>(null),
    attachAddonRef: useRef<AttachAddon | null>(null),
    isInitializedRef: useRef(false),
    lastDimensionsRef: useRef({ cols: 0, rows: 0 }),
    resizeTimeoutRef: useRef<ReturnType<typeof setTimeout> | null>(null),
    webglAddonRef: useRef<WebglAddon | null>(null),
  };
}

function useXtermSearchIntegration(
  xtermRef: React.RefObject<Terminal | null>,
  isTerminalReady: boolean,
  containerRef: React.RefObject<HTMLDivElement | null>,
) {
  const search = useTerminalSearch({ xtermRef, isTerminalReady });
  const onFindInPanelRef = useRef<(() => void) | undefined>(search.open);
  useEffect(() => {
    onFindInPanelRef.current = search.open;
  }, [search.open]);
  usePanelSearch({
    containerRef,
    isOpen: search.isOpen,
    onOpen: search.open,
    onClose: search.close,
  });
  return { search, onFindInPanelRef };
}

const WS_BASE_URL_FALLBACK = "ws://localhost:38429";
function useWsBaseUrl() {
  return useMemo(() => {
    try {
      const url = new URL(getBackendConfig().apiBaseUrl);
      return `${url.protocol === "https:" ? "wss:" : "ws:"}//${url.host}`;
    } catch {
      return WS_BASE_URL_FALLBACK;
    }
  }, []);
}

/**
 * Decide whether the WS effect should attempt to open a terminal connection.
 *
 * Agent terminals require agentctl readiness — the agent process must be
 * subscribed before passthrough I/O is meaningful.
 *
 * Shell terminals route by env on the backend; the env handler lazy-creates
 * the execution and waits for remote readiness server-side, so the client
 * only needs the env id. No session involvement.
 */
function computeCanConnect(
  mode: "agent" | "shell",
  connectionID: string | null | undefined,
  sessionId: string | null | undefined,
  isAgentctlReady: boolean,
): boolean {
  if (!connectionID) return false;
  if (mode === "agent") return Boolean(sessionId) && isAgentctlReady;
  return true;
}

// eslint-disable-next-line max-lines-per-function -- wires many hooks + refs; each block is already its own hook
export function PassthroughTerminal(props: PassthroughTerminalProps) {
  const {
    mode,
    label,
    autoFocus,
    pendingCommand,
    onCommandSent,
    onXtermReady,
    disableWebgl,
    manualInputRouting,
    onWsReady,
  } = props;
  const terminalId = mode === "shell" ? props.terminalId : undefined;
  const environmentId = mode === "shell" ? props.environmentId : undefined;
  const refs = useTerminalRefs();
  const { terminalRef, xtermRef, fitAddonRef, wsRef, attachAddonRef } = refs;

  const storeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const environmentSessionId = useEnvironmentSessionId();
  const sessionId = mode === "agent" ? (props.sessionId ?? storeSessionId) : environmentSessionId;

  const { session } = useSession(sessionId);
  const agentctlStatus = useSessionAgentctl(sessionId);
  const taskId = session?.task_id ?? null;
  const connectionID = mode === "agent" ? sessionId : environmentId;
  const canConnect = computeCanConnect(mode, connectionID, sessionId, agentctlStatus.isReady);
  const wsBaseUrl = useWsBaseUrl();

  const [isTerminalReady, setIsTerminalReady] = useState(false);
  const onTerminalReady = useCallback(() => {
    setIsTerminalReady(true);
    if (xtermRef.current) onXtermReady?.(xtermRef.current);
  }, [onXtermReady, xtermRef]);

  // Track which terminal target has an active WebSocket connection. The loading
  // overlay resets on target switches without needing a separate setState effect.
  const [connectedTargetId, setConnectedTargetId] = useState<string | null>(null);
  const isConnected = connectionID != null && connectedTargetId === connectionID;
  const onConnected = useCallback(() => {
    setConnectedTargetId(connectionID ?? null);
    if (autoFocus) refs.xtermRef.current?.textarea?.focus({ preventScroll: true });
  }, [connectionID, autoFocus, refs.xtermRef]);
  const onDisconnected = useCallback(() => {
    const disconnectedTargetId = connectionID ?? null;
    setConnectedTargetId((current) => (current === disconnectedTargetId ? null : current));
  }, [connectionID]);

  const linkHandler = useTerminalLinkHandler();
  const terminalFontFamily = useAppStore((s) => s.userSettings.terminalFontFamily);
  const terminalFontSize = useAppStore((s) => s.userSettings.terminalFontSize);
  const sendResize = useSendResize(wsRef);
  const fitAndResize = useFitAndResize({
    xtermRef: refs.xtermRef,
    fitAddonRef: refs.fitAddonRef,
    terminalRef: refs.terminalRef,
    lastDimensionsRef: refs.lastDimensionsRef,
    sendResize,
  });

  const sendInput = useSendInput(wsRef);
  const toggleBottomTerminal = useAppStore((s) => s.toggleBottomTerminal);
  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const keyboardShortcutsRef = useRef(keyboardShortcuts);
  useEffect(() => {
    keyboardShortcutsRef.current = keyboardShortcuts;
  }, [keyboardShortcuts]);
  const containerRef = useRef<HTMLDivElement>(null);
  const { search, onFindInPanelRef } = useXtermSearchIntegration(
    xtermRef,
    isTerminalReady,
    containerRef,
  );
  useTerminalInit({
    terminalRef: refs.terminalRef,
    xtermRef: refs.xtermRef,
    fitAddonRef: refs.fitAddonRef,
    isInitializedRef: refs.isInitializedRef,
    lastDimensionsRef: refs.lastDimensionsRef,
    resizeTimeoutRef: refs.resizeTimeoutRef,
    webglAddonRef: refs.webglAddonRef,
    fitAndResize,
    onReady: onTerminalReady,
    linkHandler,
    fontFamily: buildTerminalFontFamily(terminalFontFamily),
    fontSize: terminalFontSize ?? undefined,
    disableWebgl,
    onToggleBottomTerminal: toggleBottomTerminal,
    sendInput,
    keyboardShortcutsRef,
    onFindInPanelRef,
  });

  useWebSocketConnection({
    taskId,
    sessionId,
    environmentId,
    canConnect,
    isTerminalReady,
    fitAndResize,
    wsBaseUrl,
    mode,
    terminalId,
    label,
    xtermRef,
    fitAddonRef,
    wsRef,
    attachAddonRef,
    onConnected,
    onDisconnected,
    manualInputRouting,
    onWsReady,
  });

  usePendingCommand(pendingCommand, isConnected, wsRef, onCommandSent);
  useTouchScrollFallback(refs.terminalRef, refs.xtermRef, isTerminalReady);

  return (
    <div
      ref={containerRef}
      tabIndex={-1}
      data-testid={mode === "agent" ? "passthrough-terminal" : undefined}
      data-panel-kind="terminal"
      className="relative h-full w-full overflow-hidden bg-background outline-none"
      style={{ minWidth: MIN_WIDTH, minHeight: MIN_HEIGHT }}
    >
      <div className="h-full w-full p-2 pb-3">
        <div ref={terminalRef} className="h-full w-full" />
      </div>
      <TerminalSearchBar search={search} />
      {!isConnected && (
        <div
          data-testid="passthrough-loading"
          className="absolute inset-0 flex items-start justify-center pt-12 bg-background"
        >
          <div className="flex flex-col items-center gap-3 text-muted-foreground">
            <GridSpinner />
            <span className="text-sm">
              {mode === "agent" ? "Preparing workspace..." : "Connecting terminal..."}
            </span>
          </div>
        </div>
      )}
    </div>
  );
}

/** Sends a pending command to the terminal WS once connected. */
function usePendingCommand(
  pendingCommand: string | null | undefined,
  isConnected: boolean,
  wsRef: React.RefObject<WebSocket | null>,
  onCommandSent?: () => void,
) {
  React.useEffect(() => {
    if (!pendingCommand || !isConnected) return;
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    // Small delay to ensure the shell prompt is ready after WS connect.
    const timer = setTimeout(() => {
      ws.send(new TextEncoder().encode(pendingCommand));
      onCommandSent?.();
    }, 300);
    return () => clearTimeout(timer);
  }, [pendingCommand, isConnected, wsRef, onCommandSent]);
}

/**
 * Touch-drag fallback for mobile. xterm.js renders `.xterm-screen` (canvas)
 * above `.xterm-viewport`, so iOS / Android native scroll on the viewport
 * never sees touches and the terminal is effectively unscrollable on mobile.
 * This hook maps vertical drag to `Terminal.scrollLines` and adds a small
 * inertia decay on release so the gesture feels closer to the native scroll
 * on the Files panel. Gated on `(pointer: coarse)` so desktop xterm
 * behaviour is unchanged.
 */
function useTouchScrollFallback(
  terminalRef: React.RefObject<HTMLDivElement | null>,
  xtermRef: React.RefObject<Terminal | null>,
  isTerminalReady: boolean,
) {
  React.useEffect(() => {
    if (!isTerminalReady) return;
    const el = terminalRef.current;
    const term = xtermRef.current;
    if (!el || !term) return;
    if (typeof window === "undefined" || !window.matchMedia("(pointer: coarse)").matches) return;

    // Take ownership of touch gestures so the page doesn't pull-to-refresh.
    const prevTouchAction = el.style.touchAction;
    el.style.touchAction = "none";
    const prevOverscroll = el.style.overscrollBehavior;
    el.style.overscrollBehavior = "contain";

    // 16px is a safe fallback; xterm's actual cell height is queryable via
    // its internal render service but the path is private API.
    const rowHeightPx = () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const h = (term as any)?._core?._renderService?.dimensions?.css?.cell?.height;
      return typeof h === "number" && h > 0 ? h : 16;
    };

    // Drag state
    let lastY: number | null = null;
    let pendingRows = 0;
    // Last ~80ms of (timestamp, pixel-delta) pairs — used to estimate
    // release velocity. Shorter window than typical (100ms) since long
    // windows include the start of the gesture and over-smooth the speed.
    let samples: Array<{ t: number; dy: number }> = [];
    let inertiaRaf: number | null = null;

    const stopInertia = () => {
      if (inertiaRaf !== null) {
        cancelAnimationFrame(inertiaRaf);
        inertiaRaf = null;
      }
    };

    // Decay velocity to zero. FRICTION 0.94 per 16ms tick gives a feel
    // close to iOS scroll on the Files panel — quick deceleration, no tail.
    const runInertia = (velocityPxPerMs: number) => {
      let v = velocityPxPerMs;
      let prev = performance.now();
      let carryPx = 0;
      const step = () => {
        const now = performance.now();
        const dt = now - prev;
        prev = now;
        const dPx = v * dt + carryPx;
        const rh = rowHeightPx();
        const rows = Math.trunc(dPx / rh);
        if (rows !== 0) {
          term.scrollLines(rows);
          carryPx = dPx - rows * rh;
        } else {
          carryPx = dPx;
        }
        v *= Math.pow(0.94, dt / 16);
        if (Math.abs(v) < 0.01) {
          inertiaRaf = null;
          return;
        }
        inertiaRaf = requestAnimationFrame(step);
      };
      inertiaRaf = requestAnimationFrame(step);
    };

    const onStart = (e: TouchEvent) => {
      stopInertia();
      pendingRows = 0;
      samples = [];
      lastY = e.touches.length === 1 ? e.touches[0].clientY : null;
    };
    const onMove = (e: TouchEvent) => {
      if (lastY === null || e.touches.length !== 1) return;
      e.preventDefault();
      const y = e.touches[0].clientY;
      const dy = lastY - y;
      lastY = y;
      const now = performance.now();
      samples.push({ t: now, dy });
      while (samples.length > 0 && now - samples[0].t > 80) samples.shift();
      pendingRows += dy / rowHeightPx();
      const rows = Math.trunc(pendingRows);
      if (rows !== 0) {
        term.scrollLines(rows);
        pendingRows -= rows;
      }
    };
    const onEnd = () => {
      lastY = null;
      if (samples.length < 2) return;
      const totalDy = samples.reduce((s, v) => s + v.dy, 0);
      const span = samples[samples.length - 1].t - samples[0].t;
      samples = [];
      if (span <= 0) return;
      const v = totalDy / span;
      // Don't fire inertia for tiny / slow releases — those would feel
      // like the screen drifted after a tap.
      if (Math.abs(v) >= 0.3) runInertia(v);
    };

    el.addEventListener("touchstart", onStart, { passive: true });
    el.addEventListener("touchmove", onMove, { passive: false });
    el.addEventListener("touchend", onEnd, { passive: true });
    el.addEventListener("touchcancel", onEnd, { passive: true });

    return () => {
      stopInertia();
      el.removeEventListener("touchstart", onStart);
      el.removeEventListener("touchmove", onMove);
      el.removeEventListener("touchend", onEnd);
      el.removeEventListener("touchcancel", onEnd);
      el.style.touchAction = prevTouchAction;
      el.style.overscrollBehavior = prevOverscroll;
    };
  }, [isTerminalReady, terminalRef, xtermRef]);
}
