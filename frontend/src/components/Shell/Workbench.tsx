import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useMediaQuery } from "../../hooks/useMediaQuery";
import { useServerIdentity } from "../../hooks/useServerIdentity";
import { useWorkflows } from "../../hooks/useWorkflows";
import { shortId } from "../../lib/format";
import {
  readStoredPanelCollapsed,
  writeStoredPanelCollapsed,
} from "../../lib/storage";
import { routeProject, routeToSearch, type Route } from "../../lib/url";
import { useRunRecord, useRunStore } from "../../stores/runStore";
import { Button } from "../common/Button";
import { RunView } from "../RunDashboard/RunView";
import { WorkflowEditor } from "../WorkflowEditor/WorkflowEditor";
import { AppShell } from "./AppShell";
import { ChatHome } from "./ChatHome";
import { ControlPanel } from "./ControlPanel";
import { SettingsDialog } from "./SettingsDialog";
import { ProjectRequired } from "./ProjectRequired";
import { ServerBuildWarning } from "./ServerBuildWarning";
import { ShellChromeProvider, type ShellChrome } from "./ShellChrome";
import { ViewHeader } from "./ViewHeader";

interface WorkbenchProps {
  route: Route;
  projects: string[];
  activeProject: string;
  onActivateProject: (project: string) => void;
  onRemoveProject: (project: string) => void;
  navigate: (route: Route, replace?: boolean) => void;
}

const NARROW_QUERY = "(max-width: 899px)";

// The workbench owns the shell chrome and the project dialog. Catalog reads
// live inside each project group; the active project's workflow read feeds the
// composer. Views receive the object's project and a navigate callback.
export function Workbench({
  route,
  projects,
  activeProject,
  onActivateProject,
  onRemoveProject,
  navigate,
}: WorkbenchProps) {
  const narrow = useMediaQuery(NARROW_QUERY);
  const [panelCollapsed, setPanelCollapsed] = useState(readStoredPanelCollapsed);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);

  const activeWorkflows = useWorkflows(activeProject);
  const serverIdentity = useServerIdentity();
  const runRecord = useRunRecord();
  const setEntryError = useRunStore((state) => state.setEntryError);
  const setError = useRunStore((state) => state.setError);

  const declaredProject = routeProject(route);
  const chatProject = route.view === "chat" ? declaredProject ?? activeProject : "";
  const flowProject = route.view === "flow" ? declaredProject ?? activeProject : "";

  const liveRun =
    route.view === "chat" && runRecord?.id === route.runId ? runRecord : null;
  const panelHidden = narrow ? !drawerOpen : panelCollapsed;

  const closeDrawer = useCallback(() => setDrawerOpen(false), []);

  const openPanel = useCallback(() => {
    if (narrow) {
      setDrawerOpen(true);
      return;
    }
    writeStoredPanelCollapsed(false);
    setPanelCollapsed(false);
  }, [narrow]);

  const togglePanel = useCallback(() => {
    if (narrow) {
      setDrawerOpen(false);
      return;
    }
    setPanelCollapsed((current) => {
      const next = !current;
      writeStoredPanelCollapsed(next);
      return next;
    });
  }, [narrow]);

  const chrome = useMemo<ShellChrome>(
    () => ({ panelCollapsed, panelHidden, openPanel, togglePanel }),
    [panelCollapsed, panelHidden, openPanel, togglePanel],
  );

  // A deep link names its project. Adopt it once per route: after that, the
  // operator may switch the active project in the footer without the open
  // object dragging the panel back.
  const adoptedRoute = useRef<string | null>(null);
  useEffect(() => {
    const key = routeToSearch(route);
    if (adoptedRoute.current === key) return;
    adoptedRoute.current = key;
    if (declaredProject && declaredProject !== activeProject) {
      onActivateProject(declaredProject);
    }
  }, [route, declaredProject, activeProject, onActivateProject]);

  // A navigation selects a different object; the overlay drawer has done its
  // job. The desktop track is unaffected.
  useEffect(() => {
    setDrawerOpen(false);
  }, [route, activeProject]);

  // A route change starts a new surface; stale entry errors from the previous
  // one would otherwise float into it.
  useEffect(() => {
    setEntryError(null);
    setError(null);
  }, [route, setEntryError, setError]);

  const addProject = (next: string) => {
    const trimmed = next.trim();
    if (!trimmed) return;
    setSettingsOpen(false);
    onActivateProject(trimmed);
  };

  const onStarted = (runId: string) => {
    navigate({ view: "chat", runId, project: activeProject });
  };

  const onFlowSaved = (reference: string) => {
    navigate({ view: "flow", reference, project: flowProject }, true);
  };

  const onNewChat = () => {
    if (activeProject.trim()) {
      navigate({ view: "home" });
    } else {
      setSettingsOpen(true);
    }
  };

  // A removed project cannot stay the owner of the open object. Unknown deep
  // link projects are adopted, not treated as removals.
  const previousProjects = useRef(projects);
  useEffect(() => {
    const removed = previousProjects.current.filter(
      (project) => !projects.includes(project),
    );
    previousProjects.current = projects;
    const owningProject = routeProject(route);
    if (owningProject && removed.includes(owningProject)) {
      navigate({ view: "home" });
    }
  }, [projects, route, navigate]);

  const panel = (
    <ControlPanel
      projects={projects}
      activeProject={activeProject}
      route={route}
      liveRun={liveRun}
      navigate={navigate}
      onActivateProject={onActivateProject}
      onNewChat={onNewChat}
      onOpenSettings={() => setSettingsOpen(true)}
    />
  );

  return (
    <ShellChromeProvider value={chrome}>
      <AppShell
        panel={panel}
        collapsed={panelCollapsed && !narrow}
        panelHidden={panelHidden}
        drawerOpen={drawerOpen}
        onCloseDrawer={closeDrawer}
      >
        <ServerBuildWarning identity={serverIdentity.identity} />

        {route.view === "home" ? (
          <ChatHome
            project={activeProject}
            workflows={activeWorkflows.workflows}
            workflowsLoading={activeWorkflows.loading}
            workflowsError={activeWorkflows.error}
            onReloadWorkflows={() => void activeWorkflows.reload()}
            onStarted={onStarted}
            onNewFlow={(project) => navigate({ view: "flow", project })}
            onOpenProject={() => setSettingsOpen(true)}
            onProjectChange={addProject}
          />
        ) : null}

        {route.view === "chat" ? (
          chatProject.trim() ? (
            <RunView
              project={chatProject}
              runId={route.runId}
              onNewRun={() => navigate({ view: "home" })}
            />
          ) : (
            <div className="workspace-view">
              <ViewHeader
                title="Open run"
                subtitle={<span className="mono">{shortId(route.runId)}</span>}
              />
              <ProjectRequired
                title="Which project holds this run?"
                description={
                  <>
                    Run <span className="mono">{shortId(route.runId)}</span> is
                    stored inside a project's{" "}
                    <span className="mono">.specflow</span> directory. Add that
                    repository once; it is remembered in this browser.
                  </>
                }
                confirmLabel="Open run"
                secondaryAction={
                  <Button variant="ghost" onClick={() => navigate({ view: "home" })}>
                    Start a new chat instead
                  </Button>
                }
                onApply={addProject}
              />
            </div>
          )
        ) : null}

        {route.view === "flow" ? (
          flowProject.trim() ? (
            <WorkflowEditor
              project={flowProject}
              reference={route.reference}
              onBack={() => navigate({ view: "home" })}
              onSaved={onFlowSaved}
            />
          ) : (
            <div className="workspace-view">
              <ViewHeader title="Flows" subtitle="No project selected" />
              <ProjectRequired
                title="Choose a project to edit flows"
                description="Flow files are read and written inside the project's .specflow/workflows directory."
                onApply={addProject}
              />
            </div>
          )
        ) : null}
      </AppShell>

      <SettingsDialog
        open={settingsOpen}
        projects={projects}
        activeProject={activeProject}
        identity={serverIdentity.identity}
        identityLoading={!serverIdentity.loaded}
        onAdd={addProject}
        onSelect={(project) => {
          setSettingsOpen(false);
          onActivateProject(project);
        }}
        onRemove={onRemoveProject}
        onClose={() => setSettingsOpen(false)}
      />
    </ShellChromeProvider>
  );
}
