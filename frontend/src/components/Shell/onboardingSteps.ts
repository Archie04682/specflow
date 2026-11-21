import type { ProjectReadiness } from "../../types/onboarding";

// The setup card is a stepper over the gaps that block a run. The workspace
// choice is deliberately not a step: it is one decision that belongs to the
// project, not to every run, so it collapses to a summary once recorded.

export type SetupStepId = "repository" | "flow" | "skills" | "ready";
export type SetupStepState = "done" | "current" | "pending";

export interface SetupStep {
  id: SetupStepId;
  title: string;
  state: SetupStepState;
  /** One line shown when the step is not the current one. */
  summary?: string;
  /** The line the current step expands into. */
  detail?: string;
  /** A second line: the command to run, or the reason git reported. */
  hint?: string;
  /** Blockers with no action of their own, stated on the ready step. */
  notes?: string[];
}

export interface SetupPlan {
  steps: SetupStep[];
  /** The current step's position, 1-based; the last step when all are done. */
  position: number;
}

interface Entry {
  id: SetupStepId;
  title: string;
  done: boolean;
  summary?: string;
  detail?: string;
  hint?: string;
  notes?: string[];
}

/** The blockers the operator cannot fix with an action in the card. */
export function readyNotes(readiness: ProjectReadiness): string[] {
  const notes: string[] = [];
  if (!readiness.tools.openspecFound) {
    notes.push("The openspec CLI is not on PATH: npm i -g @fission-ai/openspec");
  }
  for (const harness of readiness.tools.harnesses.filter((entry) => !entry.ready)) {
    notes.push(`Harness ${harness.id} is not ready: ${harness.reason ?? "unavailable"}`);
  }
  if (readiness.uncommitted.length > 0) {
    const count = readiness.uncommitted.length;
    notes.push(`${count} uncommitted ${count === 1 ? "path" : "paths"} — a run needs a clean tree`);
  }
  if (readiness.secrets.length > 0) {
    notes.push(`These look like secrets: ${readiness.secrets.join(", ")}`);
  }
  return notes;
}

export function setupPlan(readiness: ProjectReadiness): SetupPlan {
  const folder = readiness.vcs === "folder";
  const entries: Entry[] = [];

  if (!folder) {
    const { repository, branch, hasCommit } = readiness.git;
    const done = repository && Boolean(branch) && hasCommit;
    entries.push({
      id: "repository",
      title: "Repository",
      done,
      summary: done ? `On ${branch} with a commit` : undefined,
      detail: !repository
        ? "Not a git repository"
        : !branch
          ? "No branch checked out"
          : !hasCommit
            ? "No commits yet"
            : undefined,
      hint: !repository
        ? readiness.git.reason
        : !branch
          ? readiness.git.reason
          : !hasCommit
            ? 'git add -A && git commit -m "Initial commit"'
            : undefined,
    });
  }

  const valid = readiness.flows.filter((flow) => flow.valid);
  entries.push({
    id: "flow",
    title: "Flow",
    done: valid.length > 0,
    summary: valid.length > 0 ? valid[0].reference : undefined,
    detail: valid.length === 0 ? "No flow yet" : undefined,
  });

  const missing = Array.from(
    new Set(valid.flatMap((flow) => flow.missingSkills)),
  ).sort();
  entries.push({
    id: "skills",
    title: "Skills",
    done: valid.length > 0 && missing.length === 0,
    summary:
      valid.length === 0
        ? "Waiting for a flow"
        : missing.length === 0
          ? "All bound skills resolve"
          : `${missing.length} ${missing.length === 1 ? "skill" : "skills"} missing`,
    detail: valid.length > 0 && missing.length > 0 ? `Missing: ${missing.join(", ")}` : undefined,
  });

  const notes = readyNotes(readiness);
  // The ready step is the finish line, so it is always the last one: its state
  // says whether the project can start a run, and its notes carry the blockers
  // that have no action of their own.
  entries.push({
    id: "ready",
    title: "Ready",
    done: readiness.ready && notes.length === 0,
    summary: notes.length === 0 && readiness.ready ? "Everything checks out" : undefined,
    detail: notes.length === 0 && !readiness.ready
      ? "The server reports a gap this build cannot name."
      : undefined,
    notes,
  });

  const firstIncomplete = entries.findIndex((entry) => !entry.done);
  const active = firstIncomplete === -1 ? entries.length - 1 : firstIncomplete;
  const steps = entries.map((entry, index): SetupStep => ({
    id: entry.id,
    title: entry.title,
    summary: entry.summary,
    detail: entry.detail,
    hint: entry.hint,
    notes: entry.notes,
    state: entry.done ? "done" : index === active ? "current" : "pending",
  }));

  return { steps, position: active + 1 };
}
