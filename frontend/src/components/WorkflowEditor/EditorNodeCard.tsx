import { Handle, Position, type Node, type NodeProps } from "@xyflow/react";
import type { KeyboardEvent } from "react";
import type { NodeKind } from "../../types/run";

export interface EditorNodeData extends Record<string, unknown> {
  title: string;
  kind: NodeKind;
  selected: boolean;
  onSelect: (nodeId: string) => void;
}

interface EditorNodeCardViewProps {
  title: string;
  kind: NodeKind;
  selected: boolean;
  onSelect: () => void;
}

export function EditorNodeCardView({
  title,
  kind,
  selected,
  onSelect,
}: EditorNodeCardViewProps) {
  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    onSelect();
  };

  return (
    <div
      role="button"
      tabIndex={0}
      className={`editor-node${selected ? " editor-node--selected" : ""}`}
      onClick={onSelect}
      onKeyDown={handleKeyDown}
      aria-pressed={selected}
      aria-label={`${title}, ${kind}`}
    >
      <span className="editor-node__title">{title}</span>
      <span className="editor-node__kind">{kind}</span>
    </div>
  );
}

export function EditorNodeCard({ id, data }: NodeProps<Node<EditorNodeData>>) {
  return (
    <div className="editor-node-wrap">
      <Handle
        id="target-top"
        type="target"
        position={Position.Top}
        className="editor-node__handle"
      />
      <Handle
        id="target-left"
        type="target"
        position={Position.Left}
        className="editor-node__handle"
      />
      <EditorNodeCardView
        title={data.title}
        kind={data.kind}
        selected={data.selected}
        onSelect={() => data.onSelect(id)}
      />
      <Handle
        id="source-bottom"
        type="source"
        position={Position.Bottom}
        className="editor-node__handle"
      />
      <Handle
        id="source-right"
        type="source"
        position={Position.Right}
        className="editor-node__handle"
      />
    </div>
  );
}
