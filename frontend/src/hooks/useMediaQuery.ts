import { useEffect, useState } from "react";

// A tiny matchMedia subscription so the shell can decide between "restore the
// collapsed panel" and "open the drawer" in the same handler. Tests that never
// touch the shell keep the jsdom matchMedia stub.
export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState<boolean>(() => {
    if (typeof window === "undefined") return false;
    return window.matchMedia?.(query).matches ?? false;
  });

  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    const media = window.matchMedia(query);
    const onChange = () => setMatches(media.matches);
    onChange();
    media.addEventListener?.("change", onChange);
    return () => media.removeEventListener?.("change", onChange);
  }, [query]);

  return matches;
}
