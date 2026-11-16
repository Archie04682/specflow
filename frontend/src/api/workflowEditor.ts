import { request } from "./client";
import type {
  SaveWorkflowResult,
  WorkflowDraft,
  WorkflowSource,
} from "../types/workflowDraft";

export interface WorkflowSourceRequest {
  reference: string;
  project: string;
}

export function getWorkflowSource(
  project: string,
  reference: string,
): Promise<WorkflowSource> {
  return request<WorkflowSource>(
    `/api/workflows/${encodeURIComponent(reference)}`,
    { project },
  );
}

export function saveWorkflow(
  project: string,
  reference: string,
  draft: WorkflowDraft,
  baseHash: string,
): Promise<SaveWorkflowResult> {
  return request<SaveWorkflowResult>(
    `/api/workflows/${encodeURIComponent(reference)}`,
    {
      method: "PUT",
      project,
      body: { draft, baseHash },
    },
  );
}
