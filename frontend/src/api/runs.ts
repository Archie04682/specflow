import { request } from "./client";
import type { Run } from "../types/run";
import type { RunGraph } from "../types/graph";

export type RetryMode = "forward" | "clean";
export type ApprovalDecision = "approve" | "reject";

export interface AnswerPayload {
  questionId: string;
  value?: string;
  confirmed?: boolean;
  cancelled?: boolean;
}

export interface InjectionResponse {
  id: string;
  status: string;
}

export interface ApplyReport {
  written: string[];
  deleted: string[];
}

export function applyRun(project: string, runId: string): Promise<ApplyReport> {
  return request<ApplyReport>(`/api/runs/${runId}/apply`, { method: "POST", project });
}

export function discardRun(project: string, runId: string): Promise<void> {
  return request<void>(`/api/runs/${runId}`, { method: "DELETE", project });
}

export function listRuns(project: string): Promise<Run[]> {
  return request<Run[]>("/api/runs", { project });
}

export function getRun(project: string, runId: string): Promise<Run> {
  return request<Run>(`/api/runs/${encodeURIComponent(runId)}`, { project });
}

export function getRunGraph(project: string, runId: string): Promise<RunGraph> {
  return request<RunGraph>(`/api/runs/${encodeURIComponent(runId)}/graph`, { project });
}

export function createRun(project: string, workflow: string, objective: string): Promise<Run> {
  return request<Run>("/api/runs", {
    method: "POST",
    body: { project, workflow, objective },
  });
}

export function cancelRun(project: string, runId: string): Promise<void> {
  return request<unknown>(`/api/runs/${encodeURIComponent(runId)}/cancel`, {
    method: "POST",
    project,
  }).then(() => undefined);
}

export function retryRun(
  project: string,
  runId: string,
  nodeId: string,
  mode: RetryMode = "forward",
): Promise<void> {
  return request<unknown>(`/api/runs/${encodeURIComponent(runId)}/retry`, {
    method: "POST",
    project,
    body: { nodeId, mode },
  }).then(() => undefined);
}

export function resumeRun(project: string, runId: string): Promise<void> {
  return request<unknown>(`/api/runs/${encodeURIComponent(runId)}/resume`, {
    method: "POST",
    project,
  }).then(() => undefined);
}

export function answerQuestion(
  project: string,
  runId: string,
  answer: AnswerPayload,
): Promise<void> {
  return request<unknown>(`/api/runs/${encodeURIComponent(runId)}/answers`, {
    method: "POST",
    project,
    body: answer,
  }).then(() => undefined);
}

export function injectClarification(
  project: string,
  runId: string,
  text: string,
): Promise<InjectionResponse> {
  return request<InjectionResponse>(`/api/runs/${encodeURIComponent(runId)}/injections`, {
    method: "POST",
    project,
    body: { text },
  });
}

export function approveGate(
  project: string,
  runId: string,
  nodeId: string,
  attempt: number,
  decision: ApprovalDecision,
): Promise<void> {
  return request<unknown>(`/api/runs/${encodeURIComponent(runId)}/approve`, {
    method: "POST",
    project,
    body: { nodeId, decision, attempt },
  }).then(() => undefined);
}
