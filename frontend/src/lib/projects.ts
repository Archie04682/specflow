import { basename } from "./format";

// Project paths are just strings; the registry keeps the order the operator
// added them. Normalization is the only place duplicates are removed.
export interface ProjectLabel {
  path: string;
  name: string;
  /** Present only when another project shares the same basename. */
  suffix?: string;
}

export function normalizeProjects(paths: readonly string[]): string[] {
  const seen = new Set<string>();
  const result: string[] = [];
  for (const raw of paths) {
    const path = raw.trim();
    if (!path || seen.has(path)) continue;
    seen.add(path);
    result.push(path);
  }
  return result;
}

export function addProject(projects: readonly string[], path: string): string[] {
  return normalizeProjects([...projects, path]);
}

export function hasProject(projects: readonly string[], path: string): boolean {
  return projects.includes(path.trim());
}

export function removeProject(
  projects: readonly string[],
  path: string,
): string[] {
  return projects.filter((project) => project !== path.trim());
}

function segments(path: string): string[] {
  return path.split(/[\\/]+/).filter(Boolean);
}

// Two checkouts named "specflow" are indistinguishable by title. The suffix
// shows the fewest parent segments that tell the duplicates apart.
function distinguishingSuffix(path: string, duplicates: string[]): string | undefined {
  const parts = segments(path);
  for (let take = 1; take < parts.length; take += 1) {
    const suffix = parts.slice(parts.length - 1 - take, parts.length - 1).join("/");
    const matches = duplicates.filter((other) => {
      const otherParts = segments(other);
      return (
        otherParts.slice(otherParts.length - 1 - take, otherParts.length - 1).join("/") ===
        suffix
      );
    });
    if (matches.length === 1) return suffix;
  }
  return undefined;
}

export function projectLabels(paths: readonly string[]): ProjectLabel[] {
  const byName = new Map<string, string[]>();
  for (const path of paths) {
    const name = basename(path);
    const list = byName.get(name);
    if (list) list.push(path);
    else byName.set(name, [path]);
  }

  return paths.map((path) => {
    const name = basename(path);
    const duplicates = byName.get(name) ?? [];
    return {
      path,
      name,
      suffix: duplicates.length > 1 ? distinguishingSuffix(path, duplicates) : undefined,
    };
  });
}
