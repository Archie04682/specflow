import { useCallback, useEffect, useRef } from "react";
import { getRun, getRunGraph } from "../api/runs";
import { describeError } from "../lib/errors";
import { useRunRecord, useRunStore } from "../stores/runStore";
import { useRunEvents, type EventSourceFactory } from "./useRunEvents";

const REFRESH_DEBOUNCE_MS = 120;

export interface RunDataController {
  refresh: () => Promise<void>;
  refreshGraph: () => Promise<void>;
  reconnect: () => void;
}

export interface RunDataOptions {
  eventSourceFactory?: EventSourceFactory;
}

// Opens a run: fetch the authoritative record and the pinned graph once, then
// let lifecycle events (debounced) keep the record fresh while the stream
// carries the narrative.
export function useRunData(
  project: string,
  runId: string,
  options: RunDataOptions = {},
): RunDataController {
  const run = useRunRecord();
  const debounceRef = useRef<number | null>(null);

  const refresh = useCallback(async () => {
    try {
      const next = await getRun(project, runId);
      useRunStore.getState().setRun(next);
      useRunStore.getState().setError(null);
    } catch (error) {
      useRunStore.getState().setError(describeError(error));
    }
  }, [project, runId]);

  const refreshGraph = useCallback(async () => {
    try {
      const next = await getRunGraph(project, runId);
      useRunStore.getState().setGraph(next);
    } catch (error) {
      useRunStore.getState().setGraphError(describeError(error));
    }
  }, [project, runId]);

  useEffect(() => {
    let cancelled = false;
    const store = useRunStore.getState();
    store.resetRun();
    store.setProject(project);
    store.setLoading(true);

    void (async () => {
      const [runResult, graphResult] = await Promise.allSettled([
        getRun(project, runId),
        getRunGraph(project, runId),
      ]);
      if (cancelled) return;

      if (runResult.status === "fulfilled") {
        useRunStore.getState().setRun(runResult.value);
        useRunStore.getState().setError(null);
      } else {
        useRunStore.getState().setError(describeError(runResult.reason));
      }

      if (graphResult.status === "fulfilled") {
        useRunStore.getState().setGraph(graphResult.value);
      } else {
        useRunStore.getState().setGraphError(describeError(graphResult.reason));
      }

      useRunStore.getState().setLoading(false);
    })();

    return () => {
      cancelled = true;
    };
  }, [project, runId]);

  const scheduleRefresh = useCallback(() => {
    if (debounceRef.current !== null) {
      window.clearTimeout(debounceRef.current);
    }
    debounceRef.current = window.setTimeout(() => {
      debounceRef.current = null;
      void refresh();
    }, REFRESH_DEBOUNCE_MS);
  }, [refresh]);

  useEffect(() => {
    return () => {
      if (debounceRef.current !== null) {
        window.clearTimeout(debounceRef.current);
        debounceRef.current = null;
      }
    };
  }, []);

  const { reconnect } = useRunEvents({
    project,
    runId,
    enabled: run !== null,
    factory: options.eventSourceFactory,
    onLifecycle: scheduleRefresh,
    onTerminal: () => {
      void refresh();
    },
  });

  return { refresh, refreshGraph, reconnect };
}
