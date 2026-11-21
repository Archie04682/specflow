import { useCallback, useEffect, useState } from "react";
import { getProjectReadiness } from "../api/onboarding";
import { describeError } from "../lib/errors";
import type { ProjectReadiness } from "../types/onboarding";

interface ProjectReadinessState {
  readiness: ProjectReadiness | null;
  loading: boolean;
  error: string | null;
  reload: () => void;
}

/**
 * Reads a project's readiness. It refetches when the project changes, when the
 * window regains focus, and when a provisioning action asks it to — never on a
 * timer. A failed read is not an error state: the card simply has nothing to
 * say, and the surfaces that do report errors keep doing so.
 */
export function useProjectReadiness(project: string): ProjectReadinessState {
  const [readiness, setReadiness] = useState<ProjectReadiness | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    const target = project.trim();
    if (!target) {
      setReadiness(null);
      return;
    }

    let cancelled = false;
    setLoading(true);
    void getProjectReadiness(target)
      .then((result) => {
        if (cancelled) return;
        setReadiness(result);
        setError(null);
      })
      .catch((cause) => {
        if (cancelled) return;
        setReadiness(null);
        setError(describeError(cause));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [project, revision]);

  useEffect(() => {
    const onFocus = () => setRevision((value) => value + 1);
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, []);

  const reload = useCallback(() => setRevision((value) => value + 1), []);

  return { readiness, loading, error, reload };
}
