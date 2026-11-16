import { create } from "zustand";
import type { RunGraph } from "../types/graph";
import type { AskShape, RunEvent } from "../types/events";
import type { Run } from "../types/run";
import {
  applyEvent,
  applyEvents,
  createProjection,
  type ProjectionState,
} from "../projection/eventProjection";

export type StreamStatus =
  | "idle"
  | "connecting"
  | "live"
  | "reconnecting"
  | "ended"
  | "failed";

export type ActionName =
  | "start"
  | "answer"
  | "inject"
  | "approve"
  | "cancel"
  | "retry"
  | "resume";

interface RunStore {
  project: string | null;
  runId: string | null;
  run: Run | null;
  graph: RunGraph | null;
  graphError: string | null;
  loading: boolean;
  error: string | null;
  entryError: string | null;
  notice: string | null;
  pendingAction: ActionName | null;
  selectedNodeId: string | null;
  streamStatus: StreamStatus;
  stream: ProjectionState;

  setProject: (project: string | null) => void;
  resetRun: () => void;
  setRun: (run: Run | null) => void;
  setGraph: (graph: RunGraph | null) => void;
  setGraphError: (error: string | null) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  setEntryError: (error: string | null) => void;
  setNotice: (notice: string | null) => void;
  setPendingAction: (action: ActionName | null) => void;
  selectNode: (nodeId: string | null) => void;
  setStreamStatus: (status: StreamStatus) => void;
  ingestEvent: (event: RunEvent) => void;
  ingestEvents: (events: RunEvent[]) => void;
  resetStream: () => void;
}

// One store, two slices: run state comes from REST and changes rarely; the
// stream projection is written by the EventSource callback at token rate.
// Selectors keep the graph and dock out of the stream's render path.
export const useRunStore = create<RunStore>((set, get) => ({
  project: null,
  runId: null,
  run: null,
  graph: null,
  graphError: null,
  loading: false,
  error: null,
  entryError: null,
  notice: null,
  pendingAction: null,
  selectedNodeId: null,
  streamStatus: "idle",
  stream: createProjection(),

  setProject: (project) => set({ project }),
  resetRun: () =>
    set({
      run: null,
      graph: null,
      graphError: null,
      loading: false,
      error: null,
      notice: null,
      selectedNodeId: null,
      streamStatus: "idle",
      stream: createProjection(),
    }),
  setRun: (run) => set({ run }),
  setGraph: (graph) => set({ graph, graphError: graph ? null : get().graphError }),
  setGraphError: (graphError) => set({ graphError }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  setEntryError: (entryError) => set({ entryError }),
  setNotice: (notice) => set({ notice }),
  setPendingAction: (pendingAction) => set({ pendingAction }),
  selectNode: (selectedNodeId) => set({ selectedNodeId }),
  setStreamStatus: (streamStatus) => set({ streamStatus }),
  ingestEvent: (event) => {
    const current = get().stream;
    const next = applyEvent(current, event);
    if (next !== current) set({ stream: next });
  },
  ingestEvents: (events) => {
    if (events.length === 0) return;
    const current = get().stream;
    const next = applyEvents(current, events);
    if (next !== current) set({ stream: next });
  },
  resetStream: () => set({ stream: createProjection(), streamStatus: "idle" }),
}));

export const useRunRecord = (): Run | null => useRunStore((state) => state.run);
export const useRunGraph = (): RunGraph | null => useRunStore((state) => state.graph);
export const useGraphError = (): string | null => useRunStore((state) => state.graphError);
export const useRunLoading = (): boolean => useRunStore((state) => state.loading);
export const useRunError = (): string | null => useRunStore((state) => state.error);
export const useEntryError = (): string | null => useRunStore((state) => state.entryError);
export const useRunNotice = (): string | null => useRunStore((state) => state.notice);
export const usePendingAction = (): ActionName | null =>
  useRunStore((state) => state.pendingAction);
export const useSelectedNodeId = (): string | null =>
  useRunStore((state) => state.selectedNodeId);
export const useStreamStatus = (): StreamStatus =>
  useRunStore((state) => state.streamStatus);
export const useStreamItems = () => useRunStore((state) => state.stream.items);
export const useLastStreamAt = (): string | undefined =>
  useRunStore((state) => state.stream.items[state.stream.items.length - 1]?.at);
export const useAskById = (): Record<string, AskShape> =>
  useRunStore((state) => state.stream.askById);
export const useLastSeq = (): number => useRunStore((state) => state.stream.lastSeq);
