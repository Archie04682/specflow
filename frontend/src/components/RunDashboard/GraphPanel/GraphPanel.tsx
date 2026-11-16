import type { RunActions } from "../../../hooks/useRunActions";
import {
  usePendingAction,
  useRunStore,
  useSelectedNodeId,
} from "../../../stores/runStore";
import type { RunGraph } from "../../../types/graph";
import type { Run } from "../../../types/run";
import { EmptyState } from "../../common/EmptyState";
import { Spinner } from "../../common/Spinner";
import { NodeDetails } from "./NodeDetails";
import { WorkflowGraph } from "./WorkflowGraph";

interface GraphPanelProps {
  graph: RunGraph | null;
  graphError: string | null;
  run: Run;
  actions: RunActions;
}

export function GraphPanel({ graph, graphError, run, actions }: GraphPanelProps) {
  const selectedNodeId = useSelectedNodeId();
  const pendingAction = usePendingAction();
  const selectNode = useRunStore((state) => state.selectNode);

  const selectedRunNode = selectedNodeId
    ? run.nodes.find((node) => node.nodeId === selectedNodeId)
    : undefined;
  const selectedGraphNode = selectedNodeId
    ? graph?.nodes.find((node) => node.id === selectedNodeId)
    : undefined;

  return (
    <section className="panel graph-panel" aria-label="Workflow graph">
      <header className="panel__header">
        <h2 className="panel__title">Workflow</h2>
        {graph ? <span className="muted small">{graph.nodes.length} nodes</span> : null}
      </header>
      <div className="panel__body graph-panel__body">
        {graphError ? (
          <EmptyState title="Graph unavailable" description={graphError} />
        ) : !graph ? (
          <Spinner label="Loading graph" />
        ) : (
          <WorkflowGraph
            graph={graph}
            run={run}
            selectedNodeId={selectedNodeId}
            onSelectNode={selectNode}
          />
        )}

        {selectedRunNode && selectedGraphNode ? (
          <NodeDetails
            node={selectedRunNode}
            graphNode={selectedGraphNode}
            run={run}
            busy={pendingAction === "retry"}
            onClose={() => selectNode(null)}
            onRetry={(mode) => void actions.retry(selectedRunNode.nodeId, mode)}
          />
        ) : null}
      </div>
    </section>
  );
}
