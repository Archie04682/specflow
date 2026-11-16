import type { StreamStatus } from "../../stores/runStore";

export const CONNECTION_LABEL: Record<StreamStatus, string> = {
  idle: "Not connected",
  connecting: "Connecting",
  live: "Live",
  reconnecting: "Reconnecting",
  ended: "Ended",
  failed: "Connection lost",
};

export type ConnectionTone = "live" | "pending" | "ended" | "failed";

export function connectionTone(status: StreamStatus): ConnectionTone {
  if (status === "live") return "live";
  if (status === "failed") return "failed";
  if (status === "ended") return "ended";
  return "pending";
}
