import { useCallback, useEffect, useRef, useState } from "react";
import { listRuns } from "../api/runs";
import { describeError } from "../lib/errors";
import type { Run } from "../types/run";

interface RunCatalogState {
  runs: Run[];
  loading: boolean;
  error: string | null;
  reload: () => Promise<void>;
}

// The chat catalog is a read of the run store, never a second stream. It
// refetches when the project changes, when the panel regains focus, and when
// the operator asks; the open run's row is merged from the REST-refreshed
// record instead of polling.
export function useRuns(project: string): RunCatalogState {
  const [runs, setRuns] = useState<Run[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestId = useRef(0);

  const reload = useCallback(async () => {
    const trimmed = project.trim();
    if (!trimmed) {
      setRuns([]);
      setError(null);
      return;
    }

    const id = requestId.current + 1;
    requestId.current = id;
    setLoading(true);
    setError(null);
    try {
      const result = await listRuns(trimmed);
      if (requestId.current === id) setRuns(result);
    } catch (cause) {
      if (requestId.current === id) setError(describeError(cause));
    } finally {
      if (requestId.current === id) setLoading(false);
    }
  }, [project]);

  useEffect(() => {
    void reload();
  }, [reload]);

  // Catalogs are reads, not streams. Focus keeps them honest after work
  // happens in another window without adding a timer.
  useEffect(() => {
    const onFocus = () => void reload();
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [reload]);

  return { runs, loading, error, reload };
}
