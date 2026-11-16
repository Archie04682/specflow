import { useEffect, useMemo, useRef, useState } from "react";
import {
  Background,
  Controls,
  MarkerType,
  ReactFlow,
  useNodesState,
  type Connection,
  type Edge,
  type EdgeTypes,
  type Node,
  type NodeTypes,
  type ReactFlowInstance,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { edgeKey, editorGraphNodes } from "../../editor/model";
import {
  findFreePosition,
  type EditorPosition,
} from "../../editor/placement";
import { layoutGraph, NODE_HEIGHT, NODE_WIDTH } from "../../graph/layout";
import type { GraphNode } from "../../types/graph";
import type { WorkflowDraft } from "../../types/workflowDraft";
import { EditorEdge } from "./EditorEdge";
import { EditorNodeCard, type EditorNodeData } from "./EditorNodeCard";

const nodeTypes = { editor: EditorNodeCard } as unknown as NodeTypes;
const edgeTypes = { editor: EditorEdge } as unknown as EdgeTypes;

interface HandleSides {
  sourceHandle: string;
  targetHandle: string;
}

interface EditorGraphProps {
  draft: WorkflowDraft;
  selectedNodeId: string | null;
  selectedEdgeKey: string | null;
  onSelectNode: (nodeId: string | null) => void;
  onSelectEdge: (edgeKey: string | null) => void;
  onConnect: (from: string, to: string) => void;
}

export function EditorGraph({
  draft,
  selectedNodeId,
  selectedEdgeKey,
  onSelectNode,
  onSelectEdge,
  onConnect,
}: EditorGraphProps) {
  const graphNodes = useMemo(() => editorGraphNodes(draft), [draft]);
  const layoutPositions = useMemo(
    () => layoutGraph(graphNodes, draft.edges),
    [graphNodes, draft.edges],
  );

  const [instance, setInstance] = useState<ReactFlowInstance<
    Node<EditorNodeData>,
    Edge
  > | null>(null);

  const makeNode = (
    node: GraphNode,
    position: EditorPosition,
    selected: boolean,
  ): Node<EditorNodeData> => ({
    id: node.id,
    type: "editor",
    position,
    data: {
      title: node.title ?? "Untitled node",
      kind: node.kind,
      selected,
      onSelect: (nodeId: string) => onSelectNode(nodeId),
    },
    draggable: true,
    connectable: true,
    selected,
  });

  // React Flow owns drag positions in this array; the draft only owns topology,
  // so moving a node never reaches the saved workflow. The initial render uses
  // the deterministic layout; later additions get a free spot in the viewport.
  const initialNodesRef = useRef<Node<EditorNodeData>[] | null>(null);
  if (initialNodesRef.current === null) {
    initialNodesRef.current = graphNodes.map((node) =>
      makeNode(
        node,
        layoutPositions[node.id] ?? { x: 0, y: 0 },
        node.id === selectedNodeId,
      ),
    );
  }
  const [nodes, setNodes, onNodesChange] = useNodesState<Node<EditorNodeData>>(
    initialNodesRef.current ?? [],
  );
  const canvasRef = useRef<HTMLDivElement>(null);

  const preferredPosition = (nodeId: string): EditorPosition => {
    const fallback = layoutPositions[nodeId] ?? { x: 0, y: 0 };
    const element = canvasRef.current;
    if (!instance || !element) return fallback;
    const rect = element.getBoundingClientRect();
    if (rect.width === 0 || rect.height === 0) return fallback;
    const center = instance.screenToFlowPosition({
      x: rect.left + rect.width / 2,
      y: rect.top + rect.height / 2,
    });
    return {
      x: center.x - NODE_WIDTH / 2,
      y: center.y - NODE_HEIGHT / 2,
    };
  };

  useEffect(() => {
    setNodes((current) => {
      const previous = new Map(current.map((node) => [node.id, node]));
      const occupied = current.map((node) => node.position);
      return graphNodes.map((node) => {
        const prior = previous.get(node.id);
        if (prior) {
          return makeNode(node, prior.position, node.id === selectedNodeId);
        }
        const position = findFreePosition(
          preferredPosition(node.id),
          occupied,
        );
        occupied.push(position);
        return makeNode(node, position, node.id === selectedNodeId);
      });
    });
  }, [graphNodes, layoutPositions, selectedNodeId, onSelectNode, setNodes, instance]);

  // The handles a connection actually used are visual-only: the saved draft
  // stores endpoints, never handle sides. They let a fresh connection render
  // from the side the operator dragged from instead of being re-routed.
  const [handleSides, setHandleSides] = useState<Record<string, HandleSides>>({});

  const edges = useMemo<Edge[]>(
    () =>
      draft.edges.map((edge) => {
        const key = edgeKey(edge);
        const selected = key === selectedEdgeKey;
        const routed = handleSides[key];
        return {
          id: key,
          source: edge.from,
          target: edge.to,
          // A handle-routed edge keeps the sides the operator used. Edges from
          // a saved file have no handle metadata, so the custom edge routes
          // them between the sides that face each other.
          ...(routed
            ? {
                type: "smoothstep",
                sourceHandle: routed.sourceHandle,
                targetHandle: routed.targetHandle,
              }
            : { type: "editor" }),
          selected,
          markerEnd: {
            type: MarkerType.ArrowClosed,
            width: 14,
            height: 14,
            color: selected ? "#a3adff" : "#4a5566",
          },
        };
      }),
    [draft.edges, selectedEdgeKey, handleSides],
  );

  const topology = useMemo(
    () =>
      `${graphNodes.map((node) => node.id).join("|")}#${draft.edges
        .map((edge) => edgeKey(edge))
        .join("|")}`,
    [graphNodes, draft.edges],
  );

  useEffect(() => {
    if (!instance) return;
    const frame = window.requestAnimationFrame(() => {
      void instance.fitView({ padding: 0.25, duration: 0 });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [instance, topology, nodes.length]);

  const onFlowConnect = (connection: Connection) => {
    const { source, target, sourceHandle, targetHandle } = connection;
    if (!source || !target) return;
    const exists = draft.edges.some(
      (edge) => edge.from === source && edge.to === target,
    );
    if (!exists && sourceHandle && targetHandle) {
      setHandleSides((current) => ({
        ...current,
        [edgeKey({ from: source, to: target })]: {
          sourceHandle,
          targetHandle,
        },
      }));
    }
    onConnect(source, target);
  };

  return (
    <div className="editor-canvas" data-testid="editor-graph" ref={canvasRef}>
      <ReactFlow<Node<EditorNodeData>, Edge>
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        onInit={setInstance}
        onNodesChange={onNodesChange}
        onNodeClick={(_event, node) => onSelectNode(node.id)}
        onEdgeClick={(_event, edge) => onSelectEdge(edge.id)}
        onPaneClick={() => {
          onSelectNode(null);
          onSelectEdge(null);
        }}
        onConnect={onFlowConnect}
        nodesConnectable
        nodesFocusable={false}
        edgesFocusable={false}
        deleteKeyCode={null}
        panOnDrag
        selectionOnDrag={false}
        selectionKeyCode={null}
        multiSelectionKeyCode={null}
        fitView
        minZoom={0.2}
        maxZoom={1.5}
        proOptions={{ hideAttribution: false }}
      >
        <Background gap={24} size={1} color="rgba(255,255,255,0.08)" />
        <Controls showInteractive={false} />
      </ReactFlow>
      {draft.nodes.length === 0 ? (
        <div className="editor-canvas__empty">
          <p>Add a node to start building the flow.</p>
        </div>
      ) : null}
    </div>
  );
}
