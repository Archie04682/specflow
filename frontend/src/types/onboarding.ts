// The readiness report and provisioning results the server returns. A project
// is not "first opened" anywhere durable — the registry lives in localStorage —
// so what is missing is the signal, not a first-run flag.

/** How a project isolates a run: a git worktree, or a copy of the folder. */
export type WorkspaceMode = "git" | "folder";

export interface ReadinessGit {
  repository: boolean;
  branch?: string;
  /** Whether the repository has a commit a run's worktree can branch from. */
  hasCommit: boolean;
  reason?: string;
  /** The project's workspace mode, so a missing repository is not always a gap. */
  vcs?: WorkspaceMode;
}

export interface ReadinessFlow {
  reference: string;
  path: string;
  valid: boolean;
  problems: string[];
  requiredSkills: string[];
  missingSkills: string[];
}

export interface ReadinessSkill {
  name: string;
  description: string;
  source: string;
}

export interface ReadinessSkills {
  roots: string[];
  found: ReadinessSkill[];
}

export interface ReadinessHarness {
  id: string;
  ready: boolean;
  reason?: string;
}

export interface ReadinessTools {
  openspecFound: boolean;
  openspecVersion?: string;
  harnesses: ReadinessHarness[];
}

export interface ReadinessAction {
  name: string;
  available: boolean;
  reason?: string;
}

export interface ProjectReadiness {
  project: string;
  vcs: WorkspaceMode;
  /** Whether the mode was recorded, rather than defaulting to git. */
  vcsExplicit: boolean;
  /** Whether the mode may still change: it is locked once the project has a run. */
  vcsChangeable: boolean;
  git: ReadinessGit;
  layout: boolean;
  flows: ReadinessFlow[];
  skills: ReadinessSkills;
  tools: ReadinessTools;
  uncommitted: string[];
  /** Uncommitted paths that look like secrets and must not be committed. */
  secrets: string[];
  offline: boolean;
  skillsSource?: string;
  actions: ReadinessAction[];
  ready: boolean;
}

export interface SetupResult {
  vcs: WorkspaceMode;
  repository: boolean;
  branch?: string;
  hasCommit: boolean;
}

export interface InitRepositoryResult {
  path: string;
  branch: string;
}

export interface SeedFlowResult {
  reference: string;
  path: string;
}

export interface InstallReport {
  installed: string[];
  unchanged: string[];
  conflicts: string[];
  missing: string[];
  output?: string;
  exitCode?: number;
}

/** The action names the server reports. */
export const ACTION_INIT_REPOSITORY = "init_repository";
export const ACTION_SEED_FLOW = "seed_flow";
export const ACTION_INSTALL_SKILLS_LOCAL = "install_skills_local";
export const ACTION_INSTALL_SKILLS_ONLINE = "install_skills_online";
