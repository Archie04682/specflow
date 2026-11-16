import { useCallback, useEffect, useState } from "react";
import type { Route } from "../../lib/url";
import type { Run } from "../../types/run";
import {
  BrandMark,
  IconPanelLeft,
  IconPlus,
  IconSettings,
} from "../common/icons";
import { ProjectTree } from "./ProjectTree";
import { useShellChrome } from "./ShellChrome";

interface ControlPanelProps {
  projects: string[];
  activeProject: string;
  route: Route;
  liveRun: Run | null;
  navigate: (route: Route, replace?: boolean) => void;
  onActivateProject: (project: string) => void;
  onNewChat: () => void;
  onOpenSettings: () => void;
}

// The panel is the product's navigation tree: brand, one New chat action, the
// project tree with its Chats and Flows leaves, and Settings. It never owns
// workspace state; selecting an entry is a navigation.
export function ControlPanel({
  projects,
  activeProject,
  route,
  liveRun,
  navigate,
  onActivateProject,
  onNewChat,
  onOpenSettings,
}: ControlPanelProps) {
  const { togglePanel } = useShellChrome();
  const [expanded, setExpanded] = useState<Set<string>>(
    () => new Set(activeProject ? [activeProject] : []),
  );

  // The active project is always open; removed projects leave no ghost node.
  useEffect(() => {
    setExpanded((current) => {
      const next = new Set(current);
      let changed = false;
      if (activeProject && !next.has(activeProject)) {
        next.add(activeProject);
        changed = true;
      }
      for (const project of next) {
        if (!projects.includes(project)) {
          next.delete(project);
          changed = true;
        }
      }
      return changed ? next : current;
    });
  }, [activeProject, projects]);

  const toggleProject = useCallback(
    (project: string) => {
      if (project !== activeProject) onActivateProject(project);
      setExpanded((current) => {
        const next = new Set(current);
        if (next.has(project)) next.delete(project);
        else next.add(project);
        return next;
      });
    },
    [activeProject, onActivateProject],
  );

  const currentRunId = route.view === "chat" ? route.runId : undefined;
  const currentRunProject =
    route.view === "chat" ? route.project ?? activeProject : undefined;
  const currentReference = route.view === "flow" ? route.reference : undefined;
  const currentFlowProject =
    route.view === "flow" ? route.project ?? activeProject : undefined;

  return (
    <div className="control-panel">
      <div className="control-panel__top">
        <div className="brand-row">
          <div className="brand">
            <BrandMark className="brand__mark" />
            <span className="brand__name">SpecFlow</span>
          </div>
          <button
            type="button"
            className="icon-button icon-button--sm brand-row__collapse"
            aria-label="Collapse control panel"
            title="Collapse panel"
            onClick={togglePanel}
          >
            <IconPanelLeft />
          </button>
        </div>

        <button
          type="button"
          className="new-chat"
          data-panel-initial-focus
          onClick={onNewChat}
        >
          <IconPlus />
          <span className="new-chat__label">New chat</span>
        </button>
      </div>

      <div className="control-panel__scroll">
        <ProjectTree
          projects={projects}
          activeProject={activeProject}
          expanded={expanded}
          currentRunId={currentRunId}
          currentRunProject={currentRunProject}
          currentReference={currentReference}
          currentFlowProject={currentFlowProject}
          liveRun={liveRun}
          onToggleProject={toggleProject}
          onOpenRun={(project, runId) =>
            navigate({ view: "chat", runId, project })
          }
          onNewChat={(project) => {
            onActivateProject(project);
            navigate({ view: "home" });
          }}
          onOpenFlow={(project, reference) =>
            navigate({ view: "flow", reference, project })
          }
          onNewFlow={(project) => navigate({ view: "flow", project })}
          onOpenSettings={onOpenSettings}
        />
      </div>

      <div className="control-panel__foot">
        <button
          type="button"
          className="settings-trigger"
          onClick={onOpenSettings}
          aria-haspopup="dialog"
        >
          <IconSettings className="settings-trigger__icon" />
          <span>Settings</span>
        </button>
      </div>
    </div>
  );
}
