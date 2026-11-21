import { request } from "./client";
import type {
  InitRepositoryResult,
  InstallReport,
  ProjectReadiness,
  ReadinessGit,
  SeedFlowResult,
  SetupResult,
  WorkspaceMode,
} from "../types/onboarding";

export async function getProjectReadiness(
  project: string,
): Promise<ProjectReadiness> {
  const readiness = await request<ProjectReadiness>("/api/project/readiness", {
    project,
  });
  // The server sends arrays for empty collections. A partial or older payload
  // may still carry null, and a null reaching a `.filter` call takes the whole
  // shell down, so normalize at this boundary.
  return {
    ...readiness,
    vcs: readiness.vcs ?? "git",
    vcsExplicit: readiness.vcsExplicit ?? false,
    vcsChangeable: readiness.vcsChangeable ?? false,
    flows: readiness.flows ?? [],
    skills: {
      roots: readiness.skills?.roots ?? [],
      found: readiness.skills?.found ?? [],
    },
    tools: {
      ...(readiness.tools ?? { openspecFound: false, harnesses: [] }),
      harnesses: readiness.tools?.harnesses ?? [],
    },
    uncommitted: readiness.uncommitted ?? [],
    secrets: readiness.secrets ?? [],
    actions: readiness.actions ?? [],
  };
}

export function getProjectRepository(project: string): Promise<ReadinessGit> {
  return request<ReadinessGit>("/api/project/repository", { project });
}

export interface CommitResult {
  commit: string;
  paths: string[];
  authorFallback?: boolean;
}

export interface IgnoreResult {
  ignored: string[];
}

export function commitProject(project: string, message?: string): Promise<CommitResult> {
  return request<CommitResult>("/api/project/commit", {
    method: "POST",
    body: message ? { project, message } : { project },
  });
}

export function ignoreProjectPaths(project: string, paths: string[]): Promise<IgnoreResult> {
  return request<IgnoreResult>("/api/project/ignore", {
    method: "POST",
    body: { project, paths },
  });
}

export function setupProject(
  project: string,
  vcs: WorkspaceMode,
): Promise<SetupResult> {
  return request<SetupResult>("/api/project/setup", { method: "POST", body: { project, vcs } });
}

export function initProjectRepository(
  project: string,
): Promise<InitRepositoryResult> {
  return request<InitRepositoryResult>("/api/project/repository", {
    method: "POST",
    body: { project },
  });
}

export function seedProjectFlow(
  project: string,
  reference?: string,
): Promise<SeedFlowResult> {
  return request<SeedFlowResult>("/api/project/flow", {
    method: "POST",
    body: { project, reference },
  });
}

export function installProjectSkills(
  project: string,
  replace = false,
): Promise<InstallReport> {
  return request<InstallReport>("/api/project/skills", {
    method: "POST",
    body: { project, replace },
  });
}
