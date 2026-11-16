import {
  BaseEdge,
  getSmoothStepPath,
  useInternalNode,
  type EdgeProps,
} from "@xyflow/react";
import {
  edgeAnchors,
  FALLBACK_NODE_HEIGHT,
  FALLBACK_NODE_WIDTH,
} from "./edgeGeometry";

/**
 * Edges route from the sides that face each other, so a connection is
 * horizontal when the nodes sit side by side and vertical when they are
 * stacked. Handles only start a connection; the layout decides its shape.
 */
export function EditorEdge({
  id,
  source,
  target,
  selected,
  markerEnd,
}: EdgeProps) {
  const sourceNode = useInternalNode(source);
  const targetNode = useInternalNode(target);
  const sourcePosition = sourceNode?.internals?.positionAbsolute;
  const targetPosition = targetNode?.internals?.positionAbsolute;

  if (!sourcePosition || !targetPosition) return null;

  const anchors = edgeAnchors(
    {
      x: sourcePosition.x,
      y: sourcePosition.y,
      width: sourceNode.measured?.width ?? FALLBACK_NODE_WIDTH,
      height: sourceNode.measured?.height ?? FALLBACK_NODE_HEIGHT,
    },
    {
      x: targetPosition.x,
      y: targetPosition.y,
      width: targetNode.measured?.width ?? FALLBACK_NODE_WIDTH,
      height: targetNode.measured?.height ?? FALLBACK_NODE_HEIGHT,
    },
  );

  const numbers = [
    anchors.sourceX,
    anchors.sourceY,
    anchors.targetX,
    anchors.targetY,
  ];
  if (!numbers.every(Number.isFinite)) return null;

  const [path] = getSmoothStepPath({
    ...anchors,
    borderRadius: 18,
    offset: 22,
  });

  return (
    <BaseEdge
      id={id}
      path={path}
      markerEnd={markerEnd}
      interactionWidth={24}
      style={
        selected ? { stroke: "var(--accent)", strokeWidth: 2 } : undefined
      }
      className={selected ? "editor-edge editor-edge--selected" : "editor-edge"}
    />
  );
}
