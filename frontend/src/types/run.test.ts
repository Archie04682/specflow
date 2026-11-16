import { describe, expect, it } from "vitest";
import { isTerminalRun, totalUsage, type Run } from "./run";

describe("run wire types", () => {
  it("supports a record with a zero start time and an empty usage object", () => {
    // The Go record always marshals `startedAt` (time.Time) and `usage`
    // (struct), even when the node never started and reported nothing.
    const run: Run = {
      id: "run-1",
      project: "/repo",
      status: "pending",
      objective: "",
      startedAt: "2026-01-01T00:00:00Z",
      nodes: [
        {
          nodeId: "explore",
          kind: "agent",
          status: "pending",
          usage: {},
          startedAt: "0001-01-01T00:00:00Z",
        },
      ],
    };

    expect(run.nodes[0]!.usage).toEqual({});
    expect(run.nodes[0]!.startedAt.startsWith("0001-01-01")).toBe(true);
    expect(run.pending).toBeUndefined();
    expect(run.finishedAt).toBeUndefined();
    expect(totalUsage(run.nodes)).toEqual({ tokens: 0, cost: 0 });
  });

  it("sums usage across nodes", () => {
    const run: Run = {
      id: "run-1",
      project: "/repo",
      status: "completed",
      objective: "x",
      startedAt: "2026-01-01T00:00:00Z",
      nodes: [
        {
          nodeId: "a",
          kind: "agent",
          status: "completed",
          usage: { totalTokens: 120, costUsd: 0.5 },
          startedAt: "2026-01-01T00:00:00Z",
        },
        {
          nodeId: "b",
          kind: "command",
          status: "completed",
          usage: { totalTokens: 30, costUsd: 0 },
          startedAt: "2026-01-01T00:01:00Z",
        },
      ],
    };

    expect(totalUsage(run.nodes)).toEqual({ tokens: 150, cost: 0.5 });
  });

  it("classifies terminal statuses", () => {
    expect(isTerminalRun("completed")).toBe(true);
    expect(isTerminalRun("failed")).toBe(true);
    expect(isTerminalRun("cancelled")).toBe(true);
    expect(isTerminalRun("running")).toBe(false);
    expect(isTerminalRun("waiting_answer")).toBe(false);
  });
});

describe("command node records", () => {
  it("carries the rendered command and leaves agent nodes without one", () => {
    const run: Run = {
      id: "run-1",
      project: "/repo",
      status: "completed",
      objective: "x",
      startedAt: "2026-01-01T00:00:00Z",
      nodes: [
        {
          nodeId: "verify",
          kind: "command",
          status: "completed",
          usage: {},
          startedAt: "2026-01-01T00:00:00Z",
          command: "make test",
          output: "ok",
          exitCode: 0,
        },
        {
          nodeId: "explore",
          kind: "agent",
          status: "completed",
          usage: {},
          startedAt: "2026-01-01T00:00:00Z",
        },
      ],
    };

    expect(run.nodes[0]!.command).toBe("make test");
    expect(run.nodes[0]!.output).toBe("ok");
    expect(run.nodes[1]!.command).toBeUndefined();
  });
});
