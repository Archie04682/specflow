import { NODE_HEIGHT, NODE_WIDTH, layoutGraph } from "../../../graph/layout";
import { truncate } from "../../../lib/format";
import type { GraphEdge, GraphNode } from "../../../types/graph";

interface GraphPreviewProps {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

// A static SVG preview for the start screen. The live graph uses React Flow;
// this exists so a workflow's shape is visible before a run is created.
export function GraphPreview({ nodes, edges }: GraphPreviewProps) {
  if (nodes.length === 0) {
    return <p className="muted">This workflow has no nodes.</p>;
  }

  const positions = layoutGraph(nodes, edges);
  const values = Object.values(positions);
  const minX = Math.min(...values.map((position) => position.x));
  const maxX = Math.max(...values.map((position) => position.x + NODE_WIDTH));
  const minY = Math.min(...values.map((position) => position.y));
  const maxY = Math.max(...values.map((position) => position.y + NODE_HEIGHT));
  const padding = 16;
  const width = Math.max(maxX - minX + padding * 2, 240);
  const height = Math.max(maxY - minY + padding * 2, 96);

  return (
    <div className="graph-preview">
      <svg
        viewBox={`${minX - padding} ${minY - padding} ${width} ${height}`}
        role="img"
        aria-label={`Workflow preview with ${nodes.length} nodes`}
        preserveAspectRatio="xMidYMin meet"
      >
        {edges.map((edge) => {
          const from = positions[edge.from];
          const to = positions[edge.to];
          if (!from || !to) return null;
          return (
            <line
              key={`${edge.from}-${edge.to}`}
              x1={from.x + NODE_WIDTH / 2}
              y1={from.y + NODE_HEIGHT}
              x2={to.x + NODE_WIDTH / 2}
              y2={to.y}
              className="graph-preview__edge"
            />
          );
        })}
        {nodes.map((node) => {
          const position = positions[node.id];
          if (!position) return null;
          return (
            <g key={node.id} transform={`translate(${position.x}, ${position.y})`}>
              <rect width={NODE_WIDTH} height={NODE_HEIGHT} rx={10} className="graph-preview__node" />
              <text x={12} y={30} className="graph-preview__title">
                {truncate(node.title ?? node.id, 24)}
              </text>
              <text x={12} y={52} className="graph-preview__kind">
                {node.kind}
              </text>
            </g>
          );
        })}
      </svg>
    </div>
  );
}
