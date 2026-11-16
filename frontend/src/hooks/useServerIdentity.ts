import { useCallback, useEffect, useRef, useState } from "react";
import { getServerIdentity } from "../api/server";
import type { ServerIdentity } from "../types/server";

interface ServerIdentityState {
  identity: ServerIdentity | null;
  // loaded reports that the first read finished, successfully or not, so the
  // shell can tell "still reading" apart from "the server has no identity".
  loaded: boolean;
  reload: () => Promise<void>;
}

// The shell reads the running server's identity on mount and whenever the
// window regains focus, so a restarted server is picked up without a reload.
// Identity is diagnostic: a failed read keeps the last good value instead of
// becoming an error state that could block the workbench.
export function useServerIdentity(): ServerIdentityState {
  const [identity, setIdentity] = useState<ServerIdentity | null>(null);
  const [loaded, setLoaded] = useState(false);
  const requestId = useRef(0);

  const load = useCallback(async () => {
    const id = requestId.current + 1;
    requestId.current = id;
    try {
      const next = await getServerIdentity();
      if (requestId.current === id) setIdentity(next);
    } catch {
      // Best-effort by design: an unreachable identity endpoint is not an error
      // the operator needs to dismiss.
    } finally {
      if (requestId.current === id) setLoaded(true);
    }
  }, []);

  useEffect(() => {
    void load();
    const onFocus = () => void load();
    window.addEventListener("focus", onFocus);
    return () => {
      requestId.current += 1;
      window.removeEventListener("focus", onFocus);
    };
  }, [load]);

  return { identity, loaded, reload: load };
}
