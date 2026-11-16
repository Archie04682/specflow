import { useCallback, useEffect, useState } from "react";

// A route names one object in the workspace. The control panel's project tree
// derives its current state from it, so a deep link and the panel cannot
// disagree.
export type Route =
  | { view: "home" }
  | { view: "chat"; runId: string; project?: string }
  | { view: "flow"; reference?: string; project?: string };

// A run or flow belongs to a project; home and the flows index use the active
// project instead. Kept separate from the route so a deep link can carry the
// project it was opened from.
export function routeProject(route: Route): string | undefined {
  if (route.view === "chat" || route.view === "flow") return route.project;
  return undefined;
}

export function parseRoute(search: string): Route {
  const params = new URLSearchParams(search);
  const project = params.get("project") ?? undefined;
  const runId = params.get("run");
  if (runId) return project ? { view: "chat", runId, project } : { view: "chat", runId };

  const view = params.get("view");
  if (view === "editor") {
    const reference = params.get("edit");
    if (reference) {
      return project ? { view: "flow", reference, project } : { view: "flow", reference };
    }
    return project ? { view: "flow", project } : { view: "flow" };
  }
  // `view=history` and `view=flows` are old spellings of surfaces that no
  // longer exist as pages; the tree in the control panel now owns them.
  // Parsing both as the chat home keeps old bookmarks working.
  return { view: "home" };
}

export function routeToSearch(route: Route): string {
  switch (route.view) {
    case "chat":
      return `?run=${encodeURIComponent(route.runId)}${route.project ? `&project=${encodeURIComponent(route.project)}` : ""}`;
    case "flow": {
      const base = route.reference
        ? `?view=editor&edit=${encodeURIComponent(route.reference)}`
        : "?view=editor";
      return `${base}${route.project ? `&project=${encodeURIComponent(route.project)}` : ""}`;
    }
    case "home":
    default:
      return "";
  }
}

export interface RouteController {
  route: Route;
  navigate: (route: Route, replace?: boolean) => void;
}

// Two views do not justify a router dependency. The SPA fallback serves the
// entry document for any path, so search parameters survive a reload.
export function useRoute(): RouteController {
  const [route, setRoute] = useState<Route>(() => parseRoute(window.location.search));

  useEffect(() => {
    const onPopState = () => setRoute(parseRoute(window.location.search));
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  const navigate = useCallback((next: Route, replace = false) => {
    const search = routeToSearch(next);
    const target = `${window.location.pathname}${search}`;
    if (replace) {
      window.history.replaceState(null, "", target);
    } else {
      window.history.pushState(null, "", target);
    }
    setRoute(next);
  }, []);

  return { route, navigate };
}
