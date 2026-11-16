import type { Run } from "../types/run";

// One HTTP surface for the whole app. Every non-2xx response becomes an
// ApiError carrying the server's message, so components never parse bodies.
export class ApiError extends Error {
  readonly status: number;
  readonly run?: Run;

  constructor(status: number, message: string, run?: Run) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.run = run;
  }
}

export interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "DELETE";
  body?: unknown;
  project?: string;
  signal?: AbortSignal;
}

function withProject(path: string, project: string | undefined): string {
  if (!project) return path;
  const separator = path.includes("?") ? "&" : "?";
  return `${path}${separator}project=${encodeURIComponent(project)}`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function errorMessage(payload: unknown): string | undefined {
  if (!isRecord(payload)) return undefined;
  const message = payload.error;
  return typeof message === "string" && message.trim() ? message : undefined;
}

function payloadRun(payload: unknown): Run | undefined {
  if (!isRecord(payload) || !isRecord(payload.run)) return undefined;
  const candidate = payload.run;
  if (typeof candidate.id !== "string") return undefined;
  return candidate as unknown as Run;
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = "GET", body, project, signal } = options;

  let response: Response;
  try {
    response = await fetch(withProject(path, project), {
      method,
      signal,
      headers: body === undefined ? undefined : { "content-type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") throw error;
    throw new ApiError(0, "Cannot reach the SpecFlow server. Check that it is running.");
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const text = await response.text();
  let payload: unknown;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      if (response.ok) {
        throw new ApiError(response.status, "The server returned an unreadable response.");
      }
    }
  }

  if (!response.ok) {
    const message =
      errorMessage(payload) ?? `Request failed with status ${response.status}.`;
    throw new ApiError(response.status, message, payloadRun(payload));
  }

  return payload as T;
}
