import { request } from "./client";

export interface ProjectPickResult {
  path?: string;
  cancelled?: boolean;
}

// Asks the local server to open the operating system's folder dialog. The
// browser cannot supply an absolute path itself, so this is the only way to
// offer a real explorer for a server-side project root.
export function pickProject(): Promise<ProjectPickResult> {
  return request<ProjectPickResult>("/api/project/pick", { method: "POST" });
}
