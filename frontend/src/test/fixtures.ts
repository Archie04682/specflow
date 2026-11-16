import type { NodeRun, Run } from "../types/run";
import type { GraphNode, RunGraph, WorkflowSummary } from "../types/graph";
import type { Capabilities, HarnessInfo } from "../types/harness";

export function makeNode(overrides: Partial<NodeRun> = {}): NodeRun {
  return {
    nodeId: "explore",
    title: "Explore",
    kind: "agent",
    status: "pending",
    usage: {},
    startedAt: "0001-01-01T00:00:00Z",
    ...overrides,
  };
}

export function makeRun(overrides: Partial<Run> = {}): Run {
  return {
    id: "run-1",
    project: "/tmp/project",
    workflowId: "standard-feature",
    status: "running",
    objective: "Add an export endpoint",
    nodes: [makeNode()],
    startedAt: "2026-01-01T12:00:00Z",
    ...overrides,
  };
}

export const graphNode = (id: string, overrides: Partial<GraphNode> = {}): GraphNode => ({
  id,
  title: id,
  kind: "agent",
  interactive: false,
  allowInject: true,
  requireApproval: false,
  ...overrides,
});

export function makeGraph(overrides: Partial<RunGraph> = {}): RunGraph {
  return {
    runId: "run-1",
    workflowId: "standard-feature",
    name: "Standard feature",
    nodes: [graphNode("explore"), graphNode("propose", { requireApproval: true })],
    edges: [{ from: "explore", to: "propose" }],
    ...overrides,
  };
}

export function makeWorkflow(overrides: Partial<WorkflowSummary> = {}): WorkflowSummary {
  return {
    reference: "standard",
    path: "/repo/.specflow/workflows/standard.yaml",
    name: "Standard feature",
    description: "Explore, propose, apply, verify.",
    version: 1,
    valid: true,
    problems: [],
    nodes: [graphNode("explore"), graphNode("propose")],
    edges: [{ from: "explore", to: "propose" }],
    ...overrides,
  };
}

export const capabilities: Capabilities = {
  streaming: true,
  thinking: true,
  toolEvents: true,
  fileEvents: true,
  questions: true,
  injection: true,
  abort: true,
  skills: true,
  modelSelect: true,
  toolPolicy: false,
  plugins: false,
  usage: true,
  sessionResume: true,
};

export function makeHarness(overrides: Partial<HarnessInfo> = {}): HarnessInfo {
  return {
    id: "pi",
    name: "pi",
    version: "1.5.2",
    protocol: 1,
    ready: true,
    capabilities,
    ...overrides,
  };
}
