import { request } from "./client";
import type { WorkflowCatalogResponse, WorkflowSummary } from "../types/graph";

export async function listWorkflows(project: string): Promise<WorkflowSummary[]> {
  const response = await request<WorkflowCatalogResponse>("/api/workflows", { project });
  return response.workflows ?? [];
}
