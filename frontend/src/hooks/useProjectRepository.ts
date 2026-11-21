import { useCallback, useEffect, useRef, useState } from "react";
import { getProjectRepository } from "../api/onboarding";
import { describeError } from "../lib/errors";
import type { ReadinessGit } from "../types/onboarding";

interface ProjectRepositoryState {
  status: ReadinessGit | null;
  loading: boolean;
  error: string | null;
  reload: () => Promise<void>;
}

/**
 * Whether a project is a repository, which is what the tree needs before it
 * offers chats and flows. It is deliberately cheaper than the readiness report:
 * a nav row does not need tool and harness probes.
 */
export function useProjectRepository(project: string): ProjectRepositoryState {
  const [status, setStatus] = useState<ReadinessGit | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestId = useRef(0);

  const reload = useCallback(async () => {
    const trimmed = project.trim();
    if (!trimmed) {
      setStatus(null);
      setError(null);
      return;
    }

    const id = requestId.current + 1;
    requestId.current = id;
    setLoading(true);
    setError(null);
    try {
      const result = await getProjectRepository(trimmed);
      if (requestId.current === id) setStatus(result);
    } catch (cause) {
      if (requestId.current === id) setError(describeError(cause));
    } finally {
      if (requestId.current === id) setLoading(false);
    }
  }, [project]);

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    const onFocus = () => void reload();
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [reload]);

  return { status, loading, error, reload };
}
