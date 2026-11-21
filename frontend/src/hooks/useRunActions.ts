import { useCallback } from "react";
import { ApiError } from "../api/client";
import {
  answerQuestion,
  applyRun,
  approveGate,
  cancelRun,
  createRun,
  discardRun,
  injectClarification,
  resumeRun,
  retryRun,
  type AnswerPayload,
  type ApplyReport,
  type ApprovalDecision,
  type RetryMode,
} from "../api/runs";
import { describeError } from "../lib/errors";
import { useRunStore, type ActionName } from "../stores/runStore";
import type { Run } from "../types/run";

interface RunActionsOptions {
  project: string;
  runId: string;
  refresh: () => Promise<void>;
  reconnect: () => void;
}

export interface RunActions {
  answer: (payload: AnswerPayload) => Promise<boolean>;
  inject: (text: string) => Promise<boolean>;
  approve: (
    nodeId: string,
    attempt: number,
    decision: ApprovalDecision,
  ) => Promise<boolean>;
  cancel: () => Promise<boolean>;
  retry: (nodeId: string, mode: RetryMode) => Promise<boolean>;
  resume: () => Promise<boolean>;
  apply: () => Promise<boolean>;
  discard: () => Promise<boolean>;
}

// One sentence naming what applying did, for the notice that outlives the
// refetch clearing the run's pending apply.
function applySummary(report: ApplyReport): string {
  const written = report.written.length;
  const deleted = report.deleted.length;
  const parts = [`${written} ${written === 1 ? "file" : "files"} written`];
  if (deleted > 0) parts.push(`${deleted} deleted`);
  return `Applied to the project: ${parts.join(", ")}.`;
}

export function useRunActions({
  project,
  runId,
  refresh,
  reconnect,
}: RunActionsOptions): RunActions {
  const withAction = useCallback(
    async (name: ActionName, action: () => Promise<void>): Promise<boolean> => {
      const store = useRunStore.getState();
      store.setPendingAction(name);
      store.setError(null);
      store.setNotice(null);
      try {
        await action();
        return true;
      } catch (error) {
        const after = useRunStore.getState();
        if (error instanceof ApiError) {
          if (error.status === 410) {
            // The interaction resolved between render and submit. A notice
            // outlives the refetch that clears the pending interaction; an
            // error banner would be cleared by that same refetch.
            after.setNotice("This interaction already resolved.");
            await refresh();
          } else if (error.status === 409) {
            after.setNotice(error.message);
            await refresh();
          } else {
            after.setError(error.message);
          }
        } else {
          after.setError(describeError(error));
        }
        return false;
      } finally {
        useRunStore.getState().setPendingAction(null);
      }
    },
    [refresh],
  );

  const answer = useCallback(
    (payload: AnswerPayload) =>
      withAction("answer", async () => {
        await answerQuestion(project, runId, payload);
        await refresh();
      }),
    [project, runId, refresh, withAction],
  );

  const inject = useCallback(
    (text: string) =>
      withAction("inject", async () => {
        const response = await injectClarification(project, runId, text);
        useRunStore.getState().setNotice(`Clarification queued (${response.id}).`);
        await refresh();
      }),
    [project, runId, refresh, withAction],
  );

  const approve = useCallback(
    (nodeId: string, attempt: number, decision: ApprovalDecision) =>
      withAction("approve", async () => {
        await approveGate(project, runId, nodeId, attempt, decision);
        await refresh();
      }),
    [project, runId, refresh, withAction],
  );

  const cancel = useCallback(
    () =>
      withAction("cancel", async () => {
        await cancelRun(project, runId);
        useRunStore.getState().setNotice("Cancellation requested.");
        await refresh();
      }),
    [project, runId, refresh, withAction],
  );

  const retry = useCallback(
    (nodeId: string, mode: RetryMode) =>
      withAction("retry", async () => {
        await retryRun(project, runId, nodeId, mode);
        await refresh();
        reconnect();
      }),
    [project, runId, refresh, reconnect, withAction],
  );

  const resume = useCallback(
    () =>
      withAction("resume", async () => {
        await resumeRun(project, runId);
        await refresh();
        reconnect();
      }),
    [project, runId, refresh, reconnect, withAction],
  );

  const apply = useCallback(
    () =>
      withAction("apply", async () => {
        const report = await applyRun(project, runId);
        useRunStore.getState().setNotice(applySummary(report));
        await refresh();
      }),
    [project, runId, refresh, withAction],
  );

  const discard = useCallback(
    () =>
      withAction("discard", async () => {
        await discardRun(project, runId);
        useRunStore.getState().setNotice("The run's changes were discarded.");
        await refresh();
      }),
    [project, runId, refresh, withAction],
  );

  return { answer, inject, approve, cancel, retry, resume, apply, discard };
}

export type StartResult =
  | { ok: true; run: Run }
  | { ok: false; run?: Run };

export function useStartRun(): (project: string, workflow: string, objective: string) => Promise<StartResult> {
  return useCallback(async (project, workflow, objective) => {
    const store = useRunStore.getState();
    store.setPendingAction("start");
    store.setError(null);
    store.setEntryError(null);
    try {
      const run = await createRun(project, workflow, objective);
      return { ok: true, run };
    } catch (error) {
      const message = describeError(error);
      const storeAfter = useRunStore.getState();
      storeAfter.setError(message);
      if (error instanceof ApiError) {
        storeAfter.setEntryError(message);
        if (error.run) return { ok: false, run: error.run };
      }
      return { ok: false };
    } finally {
      useRunStore.getState().setPendingAction(null);
    }
  }, []);
}
