import { useMemo } from "react";
import { projectLabels } from "../../lib/projects";
import type { Run } from "../../types/run";
import { Button } from "../common/Button";
import { IconPlus } from "../common/icons";
import { ProjectTreeNode } from "./ProjectTreeNode";

interface ProjectTreeProps {
  projects: string[];
  activeProject: string;
  expanded: ReadonlySet<string>;
  currentRunId?: string;
  currentRunProject?: string;
  currentReference?: string;
  currentFlowProject?: string;
  liveRun: Run | null;
  onToggleProject: (project: string) => void;
  onOpenRun: (project: string, runId: string) => void;
  onNewChat: (project: string) => void;
  onOpenFlow: (project: string, reference: string) => void;
  onNewFlow: (project: string) => void;
  onOpenSettings: () => void;
}

// The panel's whole navigation surface: project nodes that expand into their
// Chats and Flows leaves. There is no separate catalog tab to get out of sync
// with the object the workspace is showing.
export function ProjectTree({
  projects,
  activeProject,
  expanded,
  currentRunId,
  currentRunProject,
  currentReference,
  currentFlowProject,
  liveRun,
  onToggleProject,
  onOpenRun,
  onNewChat,
  onOpenFlow,
  onNewFlow,
  onOpenSettings,
}: ProjectTreeProps) {
  const labels = useMemo(() => projectLabels(projects), [projects]);
  const openRunProject = currentRunProject ?? activeProject;
  const openFlowProject = currentFlowProject ?? activeProject;

  if (projects.length === 0) {
    return (
      <div className="tree-empty">
        <p className="tree-empty__title">No projects yet</p>
        <p className="tree-empty__copy">
          Add a repository to browse its chats and flows here.
        </p>
        <Button variant="primary" size="sm" onClick={onOpenSettings}>
          Add project
        </Button>
      </div>
    );
  }

  return (
    <div className="project-tree" aria-label="Projects">
      {labels.map((label) => (
        <ProjectTreeNode
          key={label.path}
          label={label}
          active={activeProject === label.path}
          expanded={expanded.has(label.path)}
          currentRunId={
            openRunProject === label.path ? currentRunId : undefined
          }
          currentReference={
            openFlowProject === label.path ? currentReference : undefined
          }
          liveRun={liveRun}
          onToggle={() => onToggleProject(label.path)}
          onOpenRun={(runId) => onOpenRun(label.path, runId)}
          onNewChat={() => onNewChat(label.path)}
          onOpenFlow={(reference) => onOpenFlow(label.path, reference)}
          onNewFlow={() => onNewFlow(label.path)}
        />
      ))}
      <button type="button" className="tree-add" onClick={onOpenSettings}>
        <IconPlus />
        <span>Add project</span>
      </button>
    </div>
  );
}
