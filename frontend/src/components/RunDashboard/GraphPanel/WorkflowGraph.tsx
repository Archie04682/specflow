import { useEffect, useMemo, useState } from "react";
import {
  Background,
  Controls,
  ReactFlow,
  type Edge,
  type Node,
  type NodeTypes,
  type ReactFlowInstance,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { deriveGraphEdges } from "../../../graph/edgeState";
import { deriveNodeState } from "../../../graph/nodeState";
import { layoutGraph } from "../../../graph/layout";
import type { RunGraph } from "../../../types/graph";
import type { Run } from "../../../types/run";
import { NodeCard, type WorkflowNodeData } from "./NodeCard";

const nodeTypes = { workflow: NodeCard } as unknown as NodeTypes;

interface WorkflowGraphProps {
  graph: RunGraph;
  run: Run;
  selectedNodeId: string | null;
  onSelectNode: (nodeId: string | null) => void;
}

export function WorkflowGraph({
  graph,
  run,
  selectedNodeId,
  onSelectNode,
}: WorkflowGraphProps) {
  const positions = useMemo(
    () => layoutGraph(graph.nodes, graph.edges),
    [graph.nodes, graph.edges],
  );
  const runNodes = useMemo(
    () => new Map(run.nodes.map((node) => [node.nodeId, node])),
    [run.nodes],
  );

  const nodes = useMemo<Node<WorkflowNodeData>[]>(
    () =>
      graph.nodes.map((node) => ({
        id: node.id,
        type: "workflow",
        position: {
          x: positions[node.id]?.x ?? 0,
          y: positions[node.id]?.y ?? 0,
        },
        data: {
          title: node.title ?? node.id,
          kind: node.kind,
          state: deriveNodeState(runNodes.get(node.id), run),
          onSelect: (nodeId: string) =>
            onSelectNode(nodeId === selectedNodeId ? null : nodeId),
        },
        draggable: false,
        connectable: false,
        selected: node.id === selectedNodeId,
      })),
    [graph.nodes, positions, runNodes, run, selectedNodeId, onSelectNode],
  );

  const edges = useMemo<Edge[]>(
    () =>
      deriveGraphEdges(graph, run).map((edge) => ({
        id: edge.id,
        source: edge.source,
        target: edge.target,
        type: "smoothstep",
        // The condition is the edge's label; the state is its class, so a
        // branch the run did not take reads as passed over.
        ...(edge.label ? { label: edge.label } : {}),
        animated: edge.animated,
        className: `run-edge run-edge--${edge.state}`,
        data: { state: edge.state },
      })),
    [graph, run],
  );

  const [instance, setInstance] = useState<ReactFlowInstance | null>(null);
  const topology = useMemo(
    () =>
      `${graph.nodes.map((node) => node.id).join("|")}#${graph.edges
        .map((edge) => `${edge.from}>${edge.to}`)
        .join("|")}`,
    [graph.nodes, graph.edges],
  );

  // Fit once per topology, never on a status change: a run that updates must
  // not make the graph jump under the operator's cursor.
  useEffect(() => {
    if (!instance) return;
    const frame = window.requestAnimationFrame(() => {
      void instance.fitView({ padding: 0.2, duration: 0 });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [instance, topology]);

  return (
    <div className="graph-canvas" data-testid="workflow-graph">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        onInit={setInstance}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
        nodesFocusable={false}
        fitView
        minZoom={0.2}
        maxZoom={1.4}
        proOptions={{ hideAttribution: false }}
      >
        <Background gap={24} size={1} color="rgba(255,255,255,0.08)" />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
