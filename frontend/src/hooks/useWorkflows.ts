import { useCallback, useEffect, useRef, useState } from "react";
import { listWorkflows } from "../api/workflows";
import { describeError } from "../lib/errors";
import type { WorkflowSummary } from "../types/graph";

interface WorkflowCatalogState {
  workflows: WorkflowSummary[];
  loading: boolean;
  error: string | null;
  reload: () => Promise<void>;
}

// The project path is typed, so loading is debounced rather than fired per
// keystroke. The request id drops responses for a project the user left.
export function useWorkflows(project: string): WorkflowCatalogState {
  const [workflows, setWorkflows] = useState<WorkflowSummary[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestId = useRef(0);

  const load = useCallback(async () => {
    const trimmed = project.trim();
    if (!trimmed) {
      setWorkflows([]);
      setError(null);
      return;
    }

    const id = requestId.current + 1;
    requestId.current = id;
    setLoading(true);
    setError(null);
    try {
      const result = await listWorkflows(trimmed);
      if (requestId.current === id) setWorkflows(result);
    } catch (cause) {
      if (requestId.current === id) setError(describeError(cause));
    } finally {
      if (requestId.current === id) setLoading(false);
    }
  }, [project]);

  useEffect(() => {
    if (!project.trim()) {
      setWorkflows([]);
      setError(null);
      return;
    }
    const timer = window.setTimeout(() => {
      void load();
    }, 250);
    return () => window.clearTimeout(timer);
  }, [load, project]);

  useEffect(() => {
    const onFocus = () => void load();
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [load]);

  return { workflows, loading, error, reload: load };
}
