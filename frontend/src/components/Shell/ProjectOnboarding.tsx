import { useState, type ReactNode } from "react";
import {
  commitProject,
  ignoreProjectPaths,
  initProjectRepository,
  installProjectSkills,
  seedProjectFlow,
  setupProject,
} from "../../api/onboarding";
import { describeError } from "../../lib/errors";
import { refreshProjectData } from "../../lib/refresh";
import {
  ACTION_INIT_REPOSITORY,
  ACTION_INSTALL_SKILLS_LOCAL,
  ACTION_INSTALL_SKILLS_ONLINE,
  ACTION_SEED_FLOW,
  type InstallReport,
  type ProjectReadiness,
  type WorkspaceMode,
} from "../../types/onboarding";
import { Button } from "../common/Button";
import { IconCheck } from "../common/icons";
import { setupPlan, type SetupStep, type SetupStepId } from "./onboardingSteps";

interface ProjectOnboardingProps {
  project: string;
  readiness: ProjectReadiness | null;
  loading: boolean;
  /** Called after a provisioning action, so the flow catalog can refetch. */
  onProvisioned: () => void;
}

/**
 * Introduces a project that is not ready as a stepper over the gaps that block
 * a run. It is inline, never a gate: navigation, switching projects, and the
 * editor keep working exactly as they do for a ready project. Once the last gap
 * clears the card is gone and the ordinary chat home is the page again.
 */
export function ProjectOnboarding({
  project,
  readiness,
  loading,
  onProvisioned,
}: ProjectOnboardingProps) {
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);
  const [report, setReport] = useState<InstallReport | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  // The stepper is navigable: the state of each step comes from readiness, but
  // which step is expanded is the operator's choice.
  const [selected, setSelected] = useState<SetupStepId | null>(null);

  if (!readiness || readiness.ready) return null;

  const action = (name: string) =>
    readiness.actions.find((candidate) => candidate.name === name);

  const run = async (work: () => Promise<void>) => {
    setBusy(true);
    setProblem(null);
    setNotice(null);
    try {
      await work();
    } catch (cause) {
      setProblem(describeError(cause));
    } finally {
      setBusy(false);
    }
  };

  const chooseMode = (mode: WorkspaceMode) =>
    run(async () => {
      await setupProject(project, mode);
      onProvisioned();
      refreshProjectData();
    });

  const initRepository = () =>
    run(async () => {
      await initProjectRepository(project);
      onProvisioned();
      refreshProjectData();
    });

  const seedFlow = () =>
    run(async () => {
      await seedProjectFlow(project);
      onProvisioned();
      refreshProjectData();
    });

  const commitChanges = () =>
    run(async () => {
      const result = await commitProject(project);
      setNotice(
        `Committed ${result.paths.length} ${
          result.paths.length === 1 ? "path" : "paths"
        }${result.authorFallback ? " as SpecFlow (no git identity configured)" : ""}.`,
      );
      onProvisioned();
      refreshProjectData();
    });

  const ignoreSecrets = () =>
    run(async () => {
      const result = await ignoreProjectPaths(project, readiness.secrets);
      setNotice(`Ignored ${result.ignored.join(", ")}. Commit the changes when you are ready.`);
      onProvisioned();
      refreshProjectData();
    });

  const installSkills = (replace: boolean) =>
    run(async () => {
      const result = await installProjectSkills(project, replace);
      setReport(result);
      onProvisioned();
      refreshProjectData();
    });

  const init = action(ACTION_INIT_REPOSITORY);
  const seed = action(ACTION_SEED_FLOW);
  const local = action(ACTION_INSTALL_SKILLS_LOCAL);
  const online = action(ACTION_INSTALL_SKILLS_ONLINE);
  const folder = readiness.vcs === "folder";
  const plan = setupPlan(readiness);
  const current = plan.steps.find((step) => step.state === "current");
  const activeId =
    plan.steps.find((step) => step.id === selected)?.id ??
    current?.id ??
    plan.steps[plan.steps.length - 1].id;
  const activeIndex = plan.steps.findIndex((step) => step.id === activeId);
  const position = activeIndex + 1;
  // The choice is one project decision, not a permanent control: once a mode is
  // recorded it collapses to a summary line.
  const offerMode = readiness.vcsChangeable && !readiness.vcsExplicit;

  // Committing is an explicit action, never a side effect of setup. Paths
  // that look like secrets are ignored first, so a first commit cannot publish
  // an environment file by accident.
  const commitControls = (initial: boolean): ReactNode => {
    if (readiness.secrets.length > 0) {
      return (
        <>
          <Button
            size="sm"
            variant="secondary"
            onClick={() => void ignoreSecrets()}
            disabled={busy}
          >
            Add{" "}
            {readiness.secrets.length === 1
              ? readiness.secrets[0]
              : `${readiness.secrets.length} paths`}{" "}
            to .gitignore
          </Button>
          <span className="onboarding__reason">
            {readiness.secrets.length === 1 ? "This path looks" : "These paths look"} like
            secrets, so a commit would publish {readiness.secrets.length === 1 ? "it" : "them"}.
          </span>
        </>
      );
    }
    return (
      <Button size="sm" onClick={() => void commitChanges()} disabled={busy}>
        {initial ? "Create initial commit" : "Commit changes"}
      </Button>
    );
  };

  const stepAction = (step: SetupStep): ReactNode => {
    switch (step.id) {
      case "repository":
        if (init?.available) {
          return (
            <Button size="sm" onClick={() => void initRepository()} disabled={busy}>
              Initialize repository
            </Button>
          );
        }
        if (readiness.git.repository && !readiness.git.hasCommit) {
          return commitControls(true);
        }
        return init?.reason;
      case "flow":
        return seed?.available ? (
          <Button size="sm" onClick={() => void seedFlow()} disabled={busy}>
            Add the standard flow
          </Button>
        ) : (
          seed?.reason
        );
      case "skills":
        return local?.available ? (
          <Button size="sm" onClick={() => void installSkills(false)} disabled={busy}>
            Install skills from source
          </Button>
        ) : online?.available ? (
          <Button size="sm" onClick={() => void installSkills(false)} disabled={busy}>
            Install skills
          </Button>
        ) : (
          <span className="onboarding__reason">
            {local?.reason ?? online?.reason ?? "No skills source is available."}
          </span>
        );
      case "ready":
        return readiness.uncommitted.length > 0 ? commitControls(false) : null;
      default:
        return null;
    }
  };

  return (
    <section className="onboarding" aria-label="Project setup" data-loading={loading}>
      <header className="onboarding__head">
        <div className="onboarding__progress">
          <h2 className="onboarding__title">Finish setting up this project</h2>
          <span className="onboarding__count">
            Step {position} of {plan.steps.length}
          </span>
        </div>
        <p className="onboarding__lead">
          {folder
            ? "A run works on a copy of this folder and applies its changes when you approve them."
            : "A run works in a git worktree, so it needs a repository, a flow, and the skills it binds."}
        </p>
      </header>

      <div className={`onboarding__mode${offerMode ? "" : " onboarding__mode--chosen"}`}>
        {offerMode ? (
          <>
            <p className="onboarding__mode-lead">Where should a run work?</p>
            <div className="onboarding__mode-actions">
              <Button
                size="sm"
                variant={readiness.vcs === "git" ? "primary" : "secondary"}
                disabled={busy}
                onClick={() => void chooseMode("git")}
              >
                Use git
              </Button>
              <Button
                size="sm"
                variant={readiness.vcs === "folder" ? "primary" : "secondary"}
                disabled={busy}
                onClick={() => void chooseMode("folder")}
              >
                Just a folder
              </Button>
            </div>
          </>
        ) : (
          <p className="onboarding__mode-note">
            Workspace: {folder ? "a copy of this folder" : "a git worktree"}
          </p>
        )}
      </div>

      <ol className="onboarding__steps">
        {plan.steps.map((step, index) => {
          const active = step.id === activeId;
          const next = plan.steps[index + 1];
          const action = active ? stepAction(step) : null;
          return (
            <li
              key={step.id}
              className={`onboarding__step onboarding__step--${step.state}${
                active ? " onboarding__step--active" : ""
              }`}
            >
              <button
                type="button"
                className="onboarding__step-toggle"
                aria-expanded={active}
                aria-current={active ? "step" : undefined}
                onClick={() => setSelected(step.id)}
              >
                <span className="onboarding__marker" aria-hidden="true">
                  {step.state === "done" ? <IconCheck /> : index + 1}
                </span>
                <span className="onboarding__step-head">
                  <span className="onboarding__step-title">{step.title}</span>
                  {step.summary ? (
                    <span className="onboarding__step-summary">{step.summary}</span>
                  ) : null}
                </span>
              </button>
              {/* Blockers with no action stay visible even when the step is
                  collapsed: they are the reason the project cannot run. */}
              {step.notes && step.notes.length > 0 ? (
                <ul className="onboarding__notes">
                  {step.notes.map((note) => (
                    <li key={note}>{note}</li>
                  ))}
                </ul>
              ) : null}
              {active ? (
                <div className="onboarding__step-panel">
                  {step.detail ? <p className="onboarding__step-detail">{step.detail}</p> : null}
                  {step.hint ? (
                    <p className="onboarding__step-hint">
                      <code>{step.hint}</code>
                    </p>
                  ) : null}
                  {action ? <div className="onboarding__step-action">{action}</div> : null}
                  <div className="onboarding__step-nav">
                    {index > 0 ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setSelected(plan.steps[index - 1].id)}
                      >
                        Back
                      </Button>
                    ) : null}
                    {next ? (
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => setSelected(next.id)}
                      >
                        Next: {next.title}
                      </Button>
                    ) : null}
                  </div>
                </div>
              ) : null}
            </li>
          );
        })}
      </ol>

      <div className="onboarding__footer">
        <span className="onboarding__footer-hint">
          Changed something outside SpecFlow?
        </span>
        <Button variant="ghost" size="sm" onClick={() => refreshProjectData()}>
          Re-check
        </Button>
      </div>

      {notice ? (
        <p className="onboarding__notice" role="status">
          {notice}
        </p>
      ) : null}

      {problem ? (
        <p className="onboarding__error" role="alert">
          {problem}
        </p>
      ) : null}

      {report ? (
        <div className="onboarding__report" role="status">
          <InstallSummary report={report} />
          {report.conflicts.length > 0 ? (
            <Button
              size="sm"
              variant="secondary"
              onClick={() => void installSkills(true)}
              disabled={busy}
            >
              Replace {report.conflicts.length} existing skill
              {report.conflicts.length === 1 ? "" : "s"}
            </Button>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

function InstallSummary({ report }: { report: InstallReport }) {
  const lines: string[] = [];
  if (report.installed.length > 0) lines.push(`Installed: ${report.installed.join(", ")}`);
  if (report.unchanged.length > 0) lines.push(`Already present: ${report.unchanged.join(", ")}`);
  if (report.conflicts.length > 0) lines.push(`Different content: ${report.conflicts.join(", ")}`);
  if (report.missing.length > 0) lines.push(`Still missing: ${report.missing.join(", ")}`);
  if (report.exitCode !== undefined && report.exitCode !== 0) {
    lines.push(`The install command exited with ${report.exitCode}.`);
  }
  if (lines.length === 0) lines.push("The source provided no skills.");

  return (
    <>
      <ul className="onboarding__summary">
        {lines.map((line) => (
          <li key={line}>{line}</li>
        ))}
      </ul>
      {report.output ? <pre className="onboarding__output">{report.output.trim()}</pre> : null}
    </>
  );
}
