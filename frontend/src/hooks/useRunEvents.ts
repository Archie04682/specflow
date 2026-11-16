import { useCallback, useEffect, useRef, useState } from "react";
import {
  EVENT_TYPES,
  isLifecycleEvent,
  isTerminalEvent,
  parseRunEvent,
  runEventsUrl,
} from "../api/sse";
import { useRunStore } from "../stores/runStore";
import { isTerminalRun } from "../types/run";
import type { RunEvent } from "../types/events";

export type EventSourceFactory = (url: string) => EventSource;

const defaultFactory: EventSourceFactory = (url) => new EventSource(url);

// A burst of retained events must not turn into one React render per event.
// Events are queued and folded into the projection once per frame, capped so a
// long replay yields between batches instead of blocking the page.
const FLUSH_BATCH_SIZE = 400;

function scheduleFlush(callback: () => void): number {
  if (typeof requestAnimationFrame === "function") {
    return requestAnimationFrame(callback);
  }
  return window.setTimeout(callback, 0);
}

function cancelFlush(handle: number): void {
  if (typeof cancelAnimationFrame === "function") {
    cancelAnimationFrame(handle);
    return;
  }
  window.clearTimeout(handle);
}

interface UseRunEventsOptions {
  project: string | null;
  runId: string | null;
  enabled?: boolean;
  factory?: EventSourceFactory;
  onLifecycle?: (event: RunEvent) => void;
  onTerminal?: (event: RunEvent) => void;
}

// The browser's EventSource owns reconnection and Last-Event-ID. This hook
// only decides when a source exists, feeds the projection, and closes on the
// terminal event. A reconnect resumes after the last applied sequence, so an
// explicit reopen after retry/resume cannot duplicate the history.
export function useRunEvents({
  project,
  runId,
  enabled = true,
  factory = defaultFactory,
  onLifecycle,
  onTerminal,
}: UseRunEventsOptions): { reconnect: () => void } {
  const [generation, setGeneration] = useState(0);
  const lifecycle = useRef(onLifecycle);
  const terminal = useRef(onTerminal);
  lifecycle.current = onLifecycle;
  terminal.current = onTerminal;

  const ingestEvents = useRunStore((state) => state.ingestEvents);
  const setStatus = useRunStore((state) => state.setStreamStatus);

  useEffect(() => {
    if (!enabled || !project || !runId) return;

    const lastSeq = useRunStore.getState().stream.lastSeq;
    setStatus("connecting");
    const source = factory(runEventsUrl(project, runId, lastSeq));

    let terminalSeen = false;
    let closedByUs = false;
    const queue: RunEvent[] = [];
    let flushHandle: number | null = null;

    const flush = () => {
      flushHandle = null;
      if (queue.length === 0) return;
      const batch = queue.splice(0, FLUSH_BATCH_SIZE);
      ingestEvents(batch);
      if (queue.length > 0) flushHandle = scheduleFlush(flush);
    };

    const enqueue = (event: RunEvent) => {
      queue.push(event);
      if (flushHandle === null) flushHandle = scheduleFlush(flush);
    };

    const handle = (message: MessageEvent) => {
      const event = parseRunEvent(message);
      if (!event) return;

      enqueue(event);
      if (isLifecycleEvent(event.type)) lifecycle.current?.(event);

      if (isTerminalEvent(event.type)) {
        terminalSeen = true;
        closedByUs = true;
        setStatus("ended");
        source.close();
        // The terminal event is already queued; the scheduled flush applies it.
        terminal.current?.(event);
      }
    };

    const listeners = EVENT_TYPES.map((type) => {
      const listener = handle as EventListener;
      source.addEventListener(type, listener);
      return { type, listener };
    });

    source.onopen = () => {
      if (!closedByUs) setStatus("live");
    };
    source.onerror = () => {
      if (closedByUs) return;
      if (source.readyState === 0) {
        setStatus("reconnecting");
        return;
      }
      const run = useRunStore.getState().run;
      if (terminalSeen || (run && isTerminalRun(run.status))) {
        setStatus("ended");
      } else {
        setStatus("failed");
      }
    };

    return () => {
      closedByUs = true;
      if (flushHandle !== null) {
        cancelFlush(flushHandle);
        flushHandle = null;
      }
      queue.length = 0;
      for (const { type, listener } of listeners) {
        source.removeEventListener(type, listener);
      }
      source.close();
    };
  }, [project, runId, enabled, generation, factory, ingestEvents, setStatus]);

  const reconnect = useCallback(() => {
    setGeneration((value) => value + 1);
  }, []);

  return { reconnect };
}
