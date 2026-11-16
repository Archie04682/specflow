import { useCallback, useState } from "react";
import { Workbench } from "./components/Shell/Workbench";
import { addProject, removeProject } from "./lib/projects";
import {
  readStoredProjectState,
  writeStoredActiveProject,
  writeStoredProjects,
} from "./lib/storage";
import { useRoute } from "./lib/url";

export default function App() {
  const { route, navigate } = useRoute();
  const [projectState, setProjectState] = useState(readStoredProjectState);

  // Adding and activating are the same gesture: a path that is not in the
  // registry is appended, and becomes the project the workspace points at.
  const onActivateProject = useCallback((next: string) => {
    const trimmed = next.trim();
    if (!trimmed) return;
    setProjectState((current) => {
      const projects = addProject(current.projects, trimmed);
      writeStoredProjects(projects);
      writeStoredActiveProject(trimmed);
      return { projects, activeProject: trimmed };
    });
  }, []);

  const onRemoveProject = useCallback((next: string) => {
    const trimmed = next.trim();
    if (!trimmed) return;
    setProjectState((current) => {
      const projects = removeProject(current.projects, trimmed);
      const activeProject =
        current.activeProject === trimmed
          ? (projects[0] ?? "")
          : current.activeProject;
      writeStoredProjects(projects);
      writeStoredActiveProject(activeProject);
      return { projects, activeProject };
    });
  }, []);

  return (
    <Workbench
      route={route}
      projects={projectState.projects}
      activeProject={projectState.activeProject}
      onActivateProject={onActivateProject}
      onRemoveProject={onRemoveProject}
      navigate={navigate}
    />
  );
}
