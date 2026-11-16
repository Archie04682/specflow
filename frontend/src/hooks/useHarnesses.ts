import { useEffect, useState } from "react";
import { listHarnesses } from "../api/harnesses";
import { describeError } from "../lib/errors";
import type { HarnessInfo } from "../types/harness";

export function useHarnesses(): { harnesses: HarnessInfo[]; error: string | null } {
  const [harnesses, setHarnesses] = useState<HarnessInfo[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listHarnesses()
      .then((result) => {
        if (!cancelled) setHarnesses(result);
      })
      .catch((cause) => {
        if (!cancelled) setError(describeError(cause));
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return { harnesses, error };
}
