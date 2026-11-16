import { addProject, normalizeProjects } from "./projects";
import { MAX_RUN_SIDE, MIN_RUN_SIDE } from "./split";

const PROJECT_KEY = "specflow.project";
const PROJECTS_KEY = "specflow.projects";

export interface StoredProjectState {
  projects: string[];
  activeProject: string;
}

// The list is new; the active path keeps its original key so an existing
// browser upgrades in place instead of losing the project it already had.
export function readStoredProjectState(): StoredProjectState {
  let projects: string[] = [];
  try {
    const raw = localStorage.getItem(PROJECTS_KEY);
    if (raw) {
      const parsed: unknown = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        projects = normalizeProjects(
          parsed.filter((entry): entry is string => typeof entry === "string"),
        );
      }
    }
  } catch {
    projects = [];
  }

  let activeProject = "";
  try {
    activeProject = localStorage.getItem(PROJECT_KEY) ?? "";
  } catch {
    activeProject = "";
  }
  if (activeProject && !projects.includes(activeProject)) {
    projects = addProject(projects, activeProject);
  }
  if (!activeProject) activeProject = projects[0] ?? "";

  return { projects, activeProject };
}

export function writeStoredProjects(projects: readonly string[]): void {
  try {
    const normalized = normalizeProjects(projects);
    if (normalized.length > 0) {
      localStorage.setItem(PROJECTS_KEY, JSON.stringify(normalized));
    } else {
      localStorage.removeItem(PROJECTS_KEY);
    }
  } catch {
    // Storage can be unavailable in private modes; the session still works.
  }
}

export function writeStoredActiveProject(project: string): void {
  try {
    if (project.trim()) {
      localStorage.setItem(PROJECT_KEY, project.trim());
    } else {
      localStorage.removeItem(PROJECT_KEY);
    }
  } catch {
    // Storage can be unavailable in private modes; the session still works.
  }
}

const PANEL_COLLAPSED_KEY = "specflow.panelCollapsed";

export function readStoredPanelCollapsed(): boolean {
  try {
    return localStorage.getItem(PANEL_COLLAPSED_KEY) === "1";
  } catch {
    return false;
  }
}

export function writeStoredPanelCollapsed(collapsed: boolean): void {
  try {
    if (collapsed) {
      localStorage.setItem(PANEL_COLLAPSED_KEY, "1");
    } else {
      localStorage.removeItem(PANEL_COLLAPSED_KEY);
    }
  } catch {
    // Storage can be unavailable in private modes; the panel still works.
  }
}

const RUN_SPLIT_KEY = "specflow.runSplit";

// The run view's activity width is a view preference rather than shell state;
// it lives here so every browser-stored preference has one home.
export function readStoredRunSplit(): number | null {
  try {
    const raw = localStorage.getItem(RUN_SPLIT_KEY);
    if (!raw) return null;
    const value = Number.parseInt(raw, 10);
    if (!Number.isFinite(value)) return null;
    return Math.min(Math.max(value, MIN_RUN_SIDE), MAX_RUN_SIDE);
  } catch {
    return null;
  }
}

export function writeStoredRunSplit(width: number): void {
  try {
    if (!Number.isFinite(width)) return;
    const clamped = Math.min(Math.max(Math.round(width), MIN_RUN_SIDE), MAX_RUN_SIDE);
    localStorage.setItem(RUN_SPLIT_KEY, String(clamped));
  } catch {
    // Storage can be unavailable in private modes; the split still works.
  }
}
