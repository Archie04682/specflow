import { useCallback, useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { ApiError } from "../../api/client";
import {
  getWorkflowSource,
  saveWorkflow,
} from "../../api/workflowEditor";
import { listSkills } from "../../api/skills";
import { useHarnesses } from "../../hooks/useHarnesses";
import { describeError } from "../../lib/errors";
import {
  addEdge,
  addNode,
  draftSignature,
  edgeKey,
  emptyDraft,
  isValidReference,
  nodeTitle,
  referenceFromName,
  removeEdge,
  removeNode,
  replaceEdge,
  routeLabels,
  updateNode,
} from "../../editor/model";
import type { SkillInfo } from "../../types/skill";
import type { WorkflowDraft, WorkflowNodeDraft } from "../../types/workflowDraft";
import { ViewHeader } from "../Shell/ViewHeader";
import { Banner } from "../common/Banner";
import { Button } from "../common/Button";
import { CanvasErrorBoundary } from "../common/CanvasErrorBoundary";
import { EmptyState } from "../common/EmptyState";
import { Spinner } from "../common/Spinner";
import { EditorGraph } from "./EditorGraph";
import { NodeInspector } from "./NodeInspector";
import { WorkflowMetaPanel } from "./WorkflowMetaPanel";

type EditorTab = "node" | "workflow" | "edges";

const EDITOR_TABS: EditorTab[] = ["workflow", "node", "edges"];


interface WorkflowEditorProps {
  project: string;
  reference?: string;
  onBack: () => void;
  onSaved: (reference: string) => void;
}

export function WorkflowEditor({
  project,
  reference,
  onBack,
  onSaved,
}: WorkflowEditorProps) {
  const isNew = reference === undefined;
  const [draft, setDraft] = useState<WorkflowDraft | null>(null);
  const [baseHash, setBaseHash] = useState("");
  const [sourcePath, setSourcePath] = useState<string | null>(null);
  const [isTemplate, setIsTemplate] = useState(false);
  const [savedSignature, setSavedSignature] = useState("");
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [selectedEdgeKey, setSelectedEdgeKey] = useState<string | null>(null);
  const [edgeError, setEdgeError] = useState<string | null>(null);
  const [tab, setTab] = useState<EditorTab>("workflow");
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [conflict, setConflict] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const { harnesses, error: harnessesError } = useHarnesses();
  const [skills, setSkills] = useState<SkillInfo[] | null>(null);
  const [skillsError, setSkillsError] = useState<string | null>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let active = true;
    setSkills(null);
    setSkillsError(null);
    void listSkills(project)
      .then((catalog) => {
        if (active) setSkills(catalog);
      })
      .catch((cause) => {
        if (active) setSkillsError(describeError(cause));
      });
    return () => {
      active = false;
    };
  }, [project]);

  const load = useCallback(async () => {
    if (!reference) {
      const blank = emptyDraft();
      setDraft(blank);
      setSavedSignature(draftSignature(blank));
      setBaseHash("");
      setSourcePath(null);
      setIsTemplate(false);
      setSelectedNodeId(null);
      setSelectedEdgeKey(null);
      setEdgeError(null);
      setTab("workflow");
      setLoading(false);
      return;
    }

    setLoading(true);
    setLoadError(null);
    setSaveError(null);
    setConflict(false);
    setNotice(null);
    try {
      const source = await getWorkflowSource(project, reference);
      setDraft(source.draft);
      setBaseHash(source.hash);
      setSourcePath(source.path);
      setIsTemplate(source.isTemplate);
      setSavedSignature(draftSignature(source.draft));
      setSelectedNodeId(source.draft.nodes[0]?.id ?? null);
      setSelectedEdgeKey(null);
      setEdgeError(null);
      setTab(source.draft.nodes.length > 0 ? "node" : "workflow");
    } catch (cause) {
      setLoadError(describeError(cause));
    } finally {
      setLoading(false);
    }
  }, [project, reference]);

  useEffect(() => {
    void load();
  }, [load]);

  const dirty = draft !== null && draftSignature(draft) !== savedSignature;
  const targetReference = isNew
    ? referenceFromName(draft?.name ?? "")
    : reference ?? "";
  const referenceValid = isValidReference(targetReference);
  const derivedFileName = targetReference
    ? `${targetReference}.yaml`
    : "Name the workflow to create a file";
  const selectedNode =
    draft?.nodes.find((node) => node.id === selectedNodeId) ?? null;
  const selectedEdge =
    draft?.edges.find((edge) => edgeKey(edge) === selectedEdgeKey) ?? null;
  // The labels the selected edge may choose from: its source declares them.
  const selectedEdgeConditions =
    draft && selectedEdge ? routeLabels(draft, selectedEdge.from) : [];

  useEffect(() => {
    if (!dirty) return;
    const beforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", beforeUnload);
    return () => window.removeEventListener("beforeunload", beforeUnload);
  }, [dirty]);

  useEffect(() => {
    if (panelRef.current) panelRef.current.scrollTop = 0;
  }, [tab, selectedNodeId]);

  const back = () => {
    if (dirty && !window.confirm("Discard unsaved changes?")) return;
    onBack();
  };

  const save = async () => {
    if (!draft) return;
    if (!referenceValid) {
      setSaveError(
        isNew
          ? "Give the workflow a name with at least one ASCII letter or digit; the file name is derived from it."
          : "This workflow's file reference is invalid.",
      );
      return;
    }

    setSaving(true);
    setSaveError(null);
    setConflict(false);
    setNotice(null);
    try {
      const currentId = draft.id.trim();
      const draftToSave: WorkflowDraft = {
        ...draft,
        // The id is not user-facing. New workflows take it from the reference;
        // a path-like id parsed from a hand-written file is replaced by it too.
        id:
          currentId && !currentId.includes("/") && !currentId.includes("\\")
            ? currentId
            : targetReference,
      };
      const result = await saveWorkflow(
        project,
        targetReference,
        draftToSave,
        baseHash,
      );
      setDraft(draftToSave);
      setBaseHash(result.hash);
      setSourcePath(result.path);
      setIsTemplate(false);
      setSavedSignature(draftSignature(draftToSave));
      setNotice("Saved.");
      if (isNew) onSaved(targetReference);
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 409) {
        setConflict(true);
        setSaveError(
          isNew
            ? `A workflow file named ${targetReference}.yaml already exists. Change the workflow name or open the existing workflow from the start screen.`
            : "This workflow changed on disk. Reload it before saving again.",
        );
      } else {
        setSaveError(describeError(cause));
      }
    } finally {
      setSaving(false);
    }
  };

  const addNewNode = () => {
    if (!draft) return;
    const result = addNode(draft);
    setDraft(result.draft);
    setSelectedNodeId(result.nodeId);
    setSelectedEdgeKey(null);
    setEdgeError(null);
    setTab("node");
  };

  const selectNode = (nodeId: string | null) => {
    setSelectedNodeId(nodeId);
    if (nodeId) {
      setSelectedEdgeKey(null);
      setEdgeError(null);
      setTab("node");
    }
  };

  const selectEdge = (key: string | null) => {
    setSelectedEdgeKey(key);
    setEdgeError(null);
    if (key) {
      setSelectedNodeId(null);
      setTab("edges");
    }
  };

  const changeSelectedEdge = (patch: {
    from?: string;
    to?: string;
    when?: string;
  }) => {
    if (!draft || !selectedEdge) return;
    const next = {
      from: patch.from ?? selectedEdge.from,
      to: patch.to ?? selectedEdge.to,
      when: patch.when ?? selectedEdge.when,
    };
    if (
      next.from === selectedEdge.from &&
      next.to === selectedEdge.to &&
      next.when === selectedEdge.when
    ) {
      return;
    }
    const updated = replaceEdge(draft, selectedEdge, next);
    if (!updated) {
      setEdgeError(
        "That edge already exists, or a node cannot connect to itself.",
      );
      return;
    }
    setDraft(updated);
    setSelectedEdgeKey(edgeKey(next));
    setEdgeError(null);
  };

  const removeSelectedEdge = () => {
    if (!draft || !selectedEdge) return;
    setDraft(removeEdge(draft, selectedEdge.from, selectedEdge.to));
    setSelectedEdgeKey(null);
    setEdgeError(null);
  };

  const onTabKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "ArrowRight" && event.key !== "ArrowLeft") return;
    event.preventDefault();
    const index = EDITOR_TABS.indexOf(tab);
    const direction = event.key === "ArrowRight" ? 1 : -1;
    const next = EDITOR_TABS[(index + direction + EDITOR_TABS.length) % EDITOR_TABS.length]!;
    setTab(next);
    document.getElementById(`editor-tab-${next}`)?.focus();
  };

  const connect = (from: string, to: string) => {
    if (!draft) return;
    const next = addEdge(draft, from, to);
    if (next === draft) return;
    setDraft(next);
    setSelectedNodeId(null);
    setSelectedEdgeKey(edgeKey({ from, to }));
    setEdgeError(null);
    setTab("edges");
  };

  const changeNode = (patch: Partial<WorkflowNodeDraft>) => {
    if (!draft || !selectedNode) return;
    setDraft(updateNode(draft, selectedNode.id, patch));
  };

  const removeSelectedNode = () => {
    if (!draft || !selectedNode) return;
    if (
      !window.confirm(
        `Remove node "${nodeTitle(draft, selectedNode.id)}" and its edges?`,
      )
    ) {
      return;
    }
    setDraft(removeNode(draft, selectedNode.id));
    setSelectedNodeId(null);
    setSelectedEdgeKey(null);
    setEdgeError(null);
  };

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Delete" && event.key !== "Backspace") return;
      const target = event.target;
      if (
        target instanceof HTMLElement &&
        target.closest("input, textarea, select, [contenteditable='true']")
      ) {
        return;
      }
      if (selectedEdge) {
        event.preventDefault();
        setDraft((current) =>
          current
            ? removeEdge(current, selectedEdge.from, selectedEdge.to)
            : current,
        );
        setSelectedEdgeKey(null);
        setEdgeError(null);
        return;
      }
      if (selectedNode) {
        event.preventDefault();
        removeSelectedNode();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [selectedEdge, selectedNode, removeSelectedNode]);

  const canSave =
    draft !== null &&
    !saving &&
    referenceValid &&
    (dirty || isNew);

  return (
    <div className="workspace-view flow-editor">
      <ViewHeader
        title={draft?.name?.trim() || (isNew ? "New flow" : targetReference)}
        subtitle={
          <span className="view-header__path mono">
            {isNew ? derivedFileName : sourcePath ?? derivedFileName}
          </span>
        }
        badge={
          <span className={`chip ${dirty ? "chip--warning" : "chip--muted"}`}>
            {dirty ? "Unsaved" : isNew ? "New" : "Saved"}
          </span>
        }
        actions={
          <>
            <Button variant="ghost" size="sm" onClick={back}>
              Close
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() => void save()}
              loading={saving}
              disabled={!canSave}
            >
              Save workflow
            </Button>
          </>
        }
      />

      <main className="editor-main">
        {loading ? (
          <Spinner label="Loading workflow" />
        ) : loadError ? (
          <div className="centered-state">
            <Banner tone="error">{loadError}</Banner>
            <Button variant="secondary" onClick={onBack}>
              Back to flows
            </Button>
          </div>
        ) : draft ? (
          <>
            {isTemplate ? (
              <Banner tone="info">
                This is a template under <span className="mono">workflows/</span>.
                Saving creates{" "}
                <span className="mono">
                  .specflow/workflows/{targetReference}.yaml
                </span>{" "}
                and leaves the template unchanged.
              </Banner>
            ) : null}
            {saveError ? (
              <Banner
                tone="error"
                onDismiss={() => setSaveError(null)}
                action={
                  conflict && !isNew ? (
                    <Button size="sm" onClick={() => void load()}>
                      Reload
                    </Button>
                  ) : undefined
                }
              >
                {saveError}
              </Banner>
            ) : null}
            {notice ? (
              <Banner tone="success" onDismiss={() => setNotice(null)}>
                {notice}
              </Banner>
            ) : null}

            <div className="editor-layout">
              <section className="panel editor-canvas-panel" aria-label="Flow graph">
                <header className="panel__header">
                  <h2 className="panel__title">Graph</h2>
                  <div className="panel__actions">
                    <span className="muted small">
                      {draft.nodes.length} nodes · {draft.edges.length} edges
                    </span>
                    <Button variant="secondary" size="sm" onClick={addNewNode}>
                      Add node
                    </Button>
                  </div>
                </header>
                <div className="panel__body editor-canvas-body">
                  <CanvasErrorBoundary resetKey={reference ?? "new"}>
                    <EditorGraph
                      key={reference ?? "new"}
                      draft={draft}
                      selectedNodeId={selectedNodeId}
                      selectedEdgeKey={selectedEdgeKey}
                      onSelectNode={selectNode}
                      onSelectEdge={selectEdge}
                      onConnect={connect}
                    />
                  </CanvasErrorBoundary>
                </div>
              </section>

              <aside className="editor-side" aria-label="Workflow inspector">
                <div className="editor-meta">
                  <span
                    className="editor-meta__rev mono"
                    title="Document revision (content hash)"
                  >
                    {baseHash ? `rev ${baseHash.slice(0, 7)}` : "unsaved"}
                  </span>
                </div>

                <div
                  className="editor-tabs"
                  role="tablist"
                  aria-label="Workflow inspector"
                  onKeyDown={onTabKeyDown}
                >
                  {EDITOR_TABS.map((value) => (
                    <button
                      key={value}
                      id={`editor-tab-${value}`}
                      type="button"
                      role="tab"
                      aria-selected={tab === value}
                      aria-controls="editor-panel"
                      tabIndex={tab === value ? 0 : -1}
                      className="editor-tab"
                      onClick={() => setTab(value)}
                    >
                      {value === "node"
                        ? "Node"
                        : value === "workflow"
                          ? "Workflow"
                          : "Edges"}
                      {value === "edges" ? (
                        <span className="editor-tab__count">
                          {draft.edges.length}
                        </span>
                      ) : null}
                    </button>
                  ))}
                </div>

                <div
                  className="editor-tabpanel"
                  id="editor-panel"
                  role="tabpanel"
                  aria-labelledby={`editor-tab-${tab}`}
                  ref={panelRef}
                >
                  {tab === "workflow" ? (
                    <WorkflowMetaPanel
                      draft={draft}
                      onChange={setDraft}
                      harnesses={harnesses}
                      harnessesError={harnessesError}
                      nameHint={
                        isNew
                          ? targetReference
                            ? `File name: ${derivedFileName}`
                            : "Use ASCII letters or digits in the name; the file name is derived from it."
                          : "The file name stays fixed after creation."
                      }
                    />
                  ) : null}

                  {tab === "node" ? (
                    selectedNode ? (
                      <NodeInspector
                        key={selectedNode.id}
                        draft={draft}
                        node={selectedNode}
                        skills={skills}
                        skillsError={skillsError}
                        harnesses={harnesses}
                        harnessesError={harnessesError}
                        onChange={changeNode}
                        onRemove={removeSelectedNode}
                        onClose={() => setSelectedNodeId(null)}
                      />
                    ) : (
                      <section className="panel editor-section">
                        <div className="panel__body">
                          <p className="muted small">
                            Select a node on the canvas to edit its settings, or
                            add a node to start.
                          </p>
                        </div>
                      </section>
                    )
                  ) : null}

                  {tab === "edges" ? (
                    <>
                      {selectedEdge ? (
                        <section
                          className="panel editor-section"
                          aria-label="Edge settings"
                        >
                          <header className="panel__header">
                            <h2 className="panel__title">Edge</h2>
                          </header>
                          <div className="panel__body editor-form">
                            <div className="field-grid">
                              <div className="field">
                                <label className="field__label" htmlFor="edge-from">
                                  From
                                </label>
                                <select
                                  id="edge-from"
                                  className="input"
                                  value={selectedEdge.from}
                                  onChange={(event) =>
                                    changeSelectedEdge({
                                      from: event.target.value,
                                    })
                                  }
                                >
                                  {draft.nodes.map((node) => (
                                    <option key={node.id} value={node.id}>
                                      {nodeTitle(draft, node.id)}
                                    </option>
                                  ))}
                                </select>
                              </div>
                              <div className="field">
                                <label className="field__label" htmlFor="edge-to">
                                  To
                                </label>
                                <select
                                  id="edge-to"
                                  className="input"
                                  value={selectedEdge.to}
                                  onChange={(event) =>
                                    changeSelectedEdge({
                                      to: event.target.value,
                                    })
                                  }
                                >
                                  {draft.nodes.map((node) => (
                                    <option key={node.id} value={node.id}>
                                      {nodeTitle(draft, node.id)}
                                    </option>
                                  ))}
                                </select>
                              </div>
                            </div>
                            {selectedEdgeConditions.length > 0 ? (
                              <div className="field">
                                <label
                                  className="field__label"
                                  htmlFor="edge-when"
                                >
                                  Condition
                                </label>
                                <select
                                  id="edge-when"
                                  className="input"
                                  value={selectedEdge.when ?? ""}
                                  onChange={(event) =>
                                    changeSelectedEdge({
                                      when: event.target.value,
                                    })
                                  }
                                >
                                  {selectedEdge.when ? null : (
                                    // A routing node has no always-taken edge:
                                    // every outgoing edge names a route.
                                    <option value="" disabled>
                                      Choose a route
                                    </option>
                                  )}
                                  {selectedEdgeConditions.map((label) => (
                                    <option key={label} value={label}>
                                      {label}
                                    </option>
                                  ))}
                                </select>
                              </div>
                            ) : null}
                            {edgeError ? (
                              <p className="field__error" role="alert">
                                {edgeError}
                              </p>
                            ) : null}
                            <div className="editor-actions">
                              <Button
                                variant="secondary"
                                size="sm"
                                onClick={() =>
                                  changeSelectedEdge({
                                    from: selectedEdge.to,
                                    to: selectedEdge.from,
                                  })
                                }
                              >
                                Reverse
                              </Button>
                              <Button
                                variant="danger"
                                size="sm"
                                onClick={removeSelectedEdge}
                              >
                                Delete edge
                              </Button>
                            </div>
                          </div>
                        </section>
                      ) : null}

                      <section className="panel editor-section">
                        <header className="panel__header">
                          <h2 className="panel__title">Edges</h2>
                        </header>
                        <div className="panel__body">
                          {draft.edges.length === 0 ? (
                            <p className="muted small">
                              Drag from a node's bottom or right handle to
                              another node's top or left handle to connect
                              them.
                            </p>
                          ) : (
                            <>
                              {!selectedEdge ? (
                                <p className="muted small edge-list__hint">
                                  Select an edge on the canvas or below to
                                  change its endpoints.
                                </p>
                              ) : null}
                              <ul className="edge-list">
                                {draft.edges.map((edge) => {
                                  const key = edgeKey(edge);
                                  const active = key === selectedEdgeKey;
                                  return (
                                    <li key={key}>
                                      <button
                                        type="button"
                                        className={`edge-list__select${
                                          active
                                            ? " edge-list__select--active"
                                            : ""
                                        }`}
                                        aria-pressed={active}
                                        onClick={() => selectEdge(key)}
                                      >
                                        {nodeTitle(draft, edge.from)} →{" "}
                                        {nodeTitle(draft, edge.to)}
                                        {edge.when ? ` (${edge.when})` : ""}
                                      </button>
                                      <Button
                                        variant="ghost"
                                        size="sm"
                                        aria-label={`Remove edge from ${nodeTitle(
                                          draft,
                                          edge.from,
                                        )} to ${nodeTitle(draft, edge.to)}`}
                                        onClick={() => {
                                          setDraft(
                                            removeEdge(
                                              draft,
                                              edge.from,
                                              edge.to,
                                            ),
                                          );
                                          if (active) setSelectedEdgeKey(null);
                                          setEdgeError(null);
                                        }}
                                      >
                                        Remove
                                      </Button>
                                    </li>
                                  );
                                })}
                              </ul>
                            </>
                          )}
                        </div>
                      </section>
                    </>
                  ) : null}
                </div>
              </aside>
            </div>
          </>
        ) : (
          <EmptyState title="No workflow loaded" />
        )}
      </main>
    </div>
  );
}
