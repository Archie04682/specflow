const ZERO_TIME_PREFIX = "0001-01-01";

export function isZeroTime(value: string | undefined | null): boolean {
  return !value || value.startsWith(ZERO_TIME_PREFIX);
}

export function formatClock(value: string | undefined): string {
  if (!value || isZeroTime(value)) return "--:--:--";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "--:--:--";
  return date.toLocaleTimeString(undefined, {
    hour12: false,
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export function formatDateTime(value: string | undefined): string {
  if (!value || isZeroTime(value)) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function formatDuration(
  start: string | undefined,
  end?: string,
  now: number = Date.now(),
): string {
  if (!start || isZeroTime(start)) return "—";
  const startMs = Date.parse(start);
  if (Number.isNaN(startMs)) return "—";
  const endMs = end ? Date.parse(end) : now;
  if (Number.isNaN(endMs)) return "—";
  const totalMs = Math.max(0, endMs - startMs);
  if (totalMs < 1000) return "<1s";

  const totalSeconds = Math.floor(totalMs / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  if (hours > 0) return `${hours}h ${String(minutes).padStart(2, "0")}m`;
  if (minutes > 0) return `${minutes}m ${String(seconds).padStart(2, "0")}s`;
  return `${seconds}s`;
}

export function formatTokens(tokens: number): string {
  if (!Number.isFinite(tokens) || tokens <= 0) return "0";
  if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(1)}M`;
  if (tokens >= 1_000) return `${(tokens / 1_000).toFixed(1)}k`;
  return String(tokens);
}

export function formatCost(costUsd: number): string {
  if (!Number.isFinite(costUsd) || costUsd <= 0) return "—";
  if (costUsd < 0.01) return "<$0.01";
  if (costUsd < 1) return `$${costUsd.toFixed(4)}`;
  return `$${costUsd.toFixed(2)}`;
}

export function basename(path: string): string {
  if (!path) return "";
  const parts = path.split(/[\\/]/).filter(Boolean);
  return parts.length > 0 ? parts[parts.length - 1]! : path;
}

export function shortId(id: string, length = 8): string {
  if (!id) return "—";
  return id.length <= length ? id : id.slice(0, length);
}

export function truncate(text: string, max: number): string {
  if (max <= 0) return "";
  if (text.length <= max) return text;
  return `${text.slice(0, max - 1)}…`;
}

export function formatDeadline(deadline: string | undefined): string {
  if (!deadline) return "no deadline";
  return `deadline ${formatClock(deadline)}`;
}

export function formatObjective(objective: string, max = 90): string {
  const trimmed = objective.trim();
  return trimmed ? truncate(trimmed, max) : "(no objective)";
}

// Catalog rows use a short relative stamp for recent work and fall back to a
// date once "ago" stops being useful.
export function formatRelativeTime(
  value: string | undefined,
  now: number = Date.now(),
): string {
  if (!value || isZeroTime(value)) return "—";
  const then = Date.parse(value);
  if (Number.isNaN(then)) return "—";
  const seconds = Math.max(0, Math.floor((now - then) / 1000));
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days}d ago`;
  return formatDateTime(value);
}

/** Short duration for a tool card or command card. */
export function formatMilliseconds(milliseconds: number | undefined): string | undefined {
  if (milliseconds === undefined) return undefined;
  if (milliseconds < 1000) return `${Math.round(milliseconds)}ms`;
  return `${(milliseconds / 1000).toFixed(1)}s`;
}
