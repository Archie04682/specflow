import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { WorkflowEditor } from "./WorkflowEditor";
import {
  callBody,
  callMethod,
  callUrl,
  fakeResponse,
  stubFetch,
} from "../../test/http";
import type { WorkflowDraft, WorkflowSource } from "../../types/workflowDraft";

vi.mock("./EditorGraph", () => ({
  EditorGraph: ({
    draft,
    selectedNodeId,
    onSelectNode,
  }: {
    draft: { nodes: Array<{ id: string; title?: string }> };
    selectedNodeId: string | null;
    onSelectNode: (nodeId: string | null) => void;
  }) => (
    <div data-testid="editor-graph">
      {draft.nodes.map((node) => (
        <button
          key={node.id}
          type="button"
          aria-pressed={selectedNodeId === node.id}
          onClick={() => onSelectNode(node.id)}
        >
          {node.title ?? node.id}
        </button>
      ))}
    </div>
  ),
}));

const draft: WorkflowDraft = {
  id: "demo",
  name: "Demo",
  version: 1,
  defaults: { harness: "pi" },
  nodes: [{ id: "node-1", title: "Explore", kind: "agent" }],
  edges: [],
};

const source: WorkflowSource = {
  reference: "demo",
  path: "/repo/.specflow/workflows/demo.yaml",
  source: "id: demo\n",
  hash: "hash-1",
  isTemplate: false,
  draft,
};

function renderEditor(reference: string | undefined, onSaved = vi.fn()) {
  const onBack = vi.fn();
  render(
    <WorkflowEditor
      project="/repo"
      reference={reference}
      onBack={onBack}
      onSaved={onSaved}
    />,
  );
  return { onSaved, onBack };
}

describe("WorkflowEditor", () => {
  it("loads a workflow, edits a node, and saves with the base hash", async () => {
    const user = userEvent.setup();
    const fetchMock = stubFetch((_url, init) => {
      if (init?.method === "PUT") {
        return fakeResponse(200, {
          path: "/repo/.specflow/workflows/demo.yaml",
          hash: "hash-2",
        });
      }
      return fakeResponse(200, source);
    });
    const { onSaved } = renderEditor("demo");

    const graph = await screen.findByTestId("editor-graph");
    await within(graph).findByRole("button", { name: "Explore" });
    expect(screen.getByLabelText("Title")).toHaveValue("Explore");

    await user.clear(screen.getByLabelText("Title"));
    await user.type(screen.getByLabelText("Title"), "Explore the repo");
    await user.click(screen.getByRole("button", { name: "Save workflow" }));

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(
          (call) => (call[1] as RequestInit | undefined)?.method === "PUT",
        ),
      ).toBe(true);
    });

    const putIndex = fetchMock.mock.calls.findIndex(
      (call) => (call[1] as RequestInit | undefined)?.method === "PUT",
    );
    expect(callUrl(fetchMock, putIndex)).toContain("/api/workflows/demo");
    const body = callBody(fetchMock, putIndex) as {
      draft: WorkflowDraft;
      baseHash: string;
    };
    expect(body.baseHash).toBe("hash-1");
    expect(body.draft.nodes[0]!.title).toBe("Explore the repo");
    expect(await screen.findByText("Saved.")).toBeInTheDocument();
    expect(onSaved).not.toHaveBeenCalled();
  });

  it("derives the file name from the workflow name and saves a new flow", async () => {
    const user = userEvent.setup();
    const fetchMock = stubFetch((_url, init) => {
      if (init?.method === "PUT") {
        return fakeResponse(200, {
          path: "/repo/.specflow/workflows/some-new-flow.yaml",
          hash: "hash-new",
        });
      }
      return fakeResponse(404, { error: "not found" });
    });
    const { onSaved } = renderEditor(undefined);

    expect(screen.getByRole("heading", { name: "New flow" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Workflow" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByRole("button", { name: "Save workflow" })).toBeDisabled();
    expect(
      screen.getByText("Name the workflow to create a file"),
    ).toBeInTheDocument();

    await user.type(screen.getByLabelText("Name"), "!!!");
    expect(screen.getByRole("button", { name: "Save workflow" })).toBeDisabled();
    expect(
      screen.getByText(/ASCII letters or digits/),
    ).toBeInTheDocument();

    await user.clear(screen.getByLabelText("Name"));
    await user.type(screen.getByLabelText("Name"), "Some new flow");
    expect(screen.getByText("some-new-flow.yaml")).toBeInTheDocument();
    expect(screen.getByText("File name: some-new-flow.yaml")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save workflow" })).toBeEnabled();

    await user.click(screen.getByRole("button", { name: "Add node" }));
    expect(within(screen.getByTestId("editor-graph")).getByText("New node")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Save workflow" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith("some-new-flow"));

    const putIndex = fetchMock.mock.calls.findIndex(
      (call) => (call[1] as RequestInit | undefined)?.method === "PUT",
    );
    expect(callUrl(fetchMock, putIndex)).toContain(
      "/api/workflows/some-new-flow",
    );
    expect(callMethod(fetchMock, putIndex)).toBe("PUT");
    expect(callBody(fetchMock, putIndex)).toMatchObject({
      baseHash: "",
      draft: {
        id: "some-new-flow",
        name: "Some new flow",
        nodes: [{ id: "node-1", title: "New node" }],
      },
    });
  });

  it("orders the inspector Workflow, Node, Edges and hides internal ids", async () => {
    const user = userEvent.setup();
    stubFetch(() => fakeResponse(200, source));
    renderEditor("demo");

    await screen.findByTestId("editor-graph");
    const tabs = screen.getAllByRole("tab");
    expect(tabs[0]).toHaveTextContent("Workflow");
    expect(tabs[1]).toHaveTextContent("Node");
    expect(tabs[2]).toHaveTextContent("Edges");

    expect(screen.getByLabelText("Title")).toHaveValue("Explore");
    expect(screen.queryByText("Identifier")).toBeNull();
    expect(screen.queryByLabelText("Version")).toBeNull();
    expect(screen.queryByLabelText("ID")).toBeNull();
    expect(screen.getByText("rev hash-1")).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: "Workflow" }));
    expect(screen.getByLabelText("Name")).toHaveValue("Demo");
    expect(screen.queryByLabelText("Title")).toBeNull();
    expect(screen.queryByLabelText("Version")).toBeNull();
    expect(screen.queryByLabelText("ID")).toBeNull();

    await user.click(screen.getByRole("tab", { name: /Edges/ }));
    expect(screen.getByText(/Drag from a node's/)).toBeInTheDocument();
  });

  it("moves between tabs with the arrow keys", async () => {
    const user = userEvent.setup();
    stubFetch(() => fakeResponse(200, source));
    renderEditor("demo");

    await screen.findByTestId("editor-graph");
    await user.click(screen.getByRole("tab", { name: "Workflow" }));
    const workflowTab = screen.getByRole("tab", { name: "Workflow" });
    workflowTab.focus();
    await user.keyboard("{ArrowRight}");

    expect(screen.getByRole("tab", { name: "Node" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByLabelText("Title")).toBeInTheDocument();
  });

  it("shows edge endpoints by title, never by internal id", async () => {
    const user = userEvent.setup();
    const connected: WorkflowSource = {
      ...source,
      draft: {
        ...draft,
        nodes: [
          { id: "node-1", title: "Explore", kind: "agent" },
          { id: "node-2", title: "Apply", kind: "agent" },
        ],
        edges: [{ from: "node-1", to: "node-2" }],
      },
    };
    stubFetch(() => fakeResponse(200, connected));
    renderEditor("demo");

    await screen.findByTestId("editor-graph");
    await user.click(screen.getByRole("tab", { name: /Edges/ }));

    expect(screen.getByText("Explore → Apply")).toBeInTheDocument();
    expect(screen.queryByText(/node-1|node-2/)).toBeNull();
    await user.click(
      screen.getByRole("button", { name: "Remove edge from Explore to Apply" }),
    );
    expect(screen.getByText(/Drag from a node's/)).toBeInTheDocument();
  });

  it("surfaces a conflict and offers a reload", async () => {
    const user = userEvent.setup();
    stubFetch((_url, init) => {
      if (init?.method === "PUT") {
        return fakeResponse(409, {
          error: "workflow changed on disk",
          hash: "hash-2",
        });
      }
      return fakeResponse(200, source);
    });
    renderEditor("demo");

    await screen.findByTestId("editor-graph");
    await user.clear(screen.getByLabelText("Title"));
    await user.type(screen.getByLabelText("Title"), "Changed");
    await user.click(screen.getByRole("button", { name: "Save workflow" }));

    expect(
      await screen.findByText(/changed on disk/),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reload" })).toBeInTheDocument();
  });

  it("keeps the draft editable when the server reports structural problems", async () => {
    const user = userEvent.setup();
    stubFetch((_url, init) => {
      if (init?.method === "PUT") {
        return fakeResponse(400, {
          error: "workflow is invalid: workflow has no nodes",
          problems: ["workflow has no nodes"],
        });
      }
      return fakeResponse(200, source);
    });
    renderEditor("demo");

    await screen.findByTestId("editor-graph");
    await user.clear(screen.getByLabelText("Title"));
    await user.type(screen.getByLabelText("Title"), "Changed");
    await user.click(screen.getByRole("button", { name: "Save workflow" }));

    expect(
      await screen.findByText(/workflow is invalid: workflow has no nodes/),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Title")).toHaveValue("Changed");
  });

  it("shows a load failure with a way back", async () => {
    stubFetch(() => fakeResponse(404, { error: "workflow not found" }));
    const { onBack } = renderEditor("missing");

    expect(await screen.findByText(/workflow not found/)).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "Back to flows" }));
    expect(onBack).toHaveBeenCalledTimes(1);
  });
});

describe("WorkflowEditor edge editing", () => {
  const multi: WorkflowSource = {
    ...source,
    draft: {
      ...draft,
      nodes: [
        { id: "node-1", title: "Explore", kind: "agent" },
        { id: "node-2", title: "Apply", kind: "agent" },
        { id: "node-3", title: "Review", kind: "agent" },
      ],
      edges: [{ from: "node-1", to: "node-2" }],
    },
  };

  it("selects an edge from the list and rewires its endpoint", async () => {
    const user = userEvent.setup();
    stubFetch(() => fakeResponse(200, multi));
    renderEditor("demo");

    await screen.findByTestId("editor-graph");
    await user.click(screen.getByRole("tab", { name: /Edges/ }));
    await user.click(screen.getByRole("button", { name: "Explore → Apply" }));

    expect(screen.getByLabelText("From")).toHaveValue("node-1");
    expect(screen.getByLabelText("To")).toHaveValue("node-2");
    await user.selectOptions(screen.getByLabelText("To"), "node-3");

    expect(screen.getByLabelText("To")).toHaveValue("node-3");
    expect(screen.getByRole("button", { name: "Explore → Review" })).toBeInTheDocument();
  });

  it("refuses a duplicate edge from the form", async () => {
    const user = userEvent.setup();
    const twoEdges: WorkflowSource = {
      ...multi,
      draft: {
        ...multi.draft,
        edges: [
          { from: "node-1", to: "node-2" },
          { from: "node-1", to: "node-3" },
        ],
      },
    };
    stubFetch(() => fakeResponse(200, twoEdges));
    renderEditor("demo");

    await screen.findByTestId("editor-graph");
    await user.click(screen.getByRole("tab", { name: /Edges/ }));
    await user.click(screen.getByRole("button", { name: "Explore → Review" }));
    await user.selectOptions(screen.getByLabelText("To"), "node-2");

    expect(screen.getByRole("alert")).toHaveTextContent(/already exists/);
    expect(screen.getByRole("button", { name: "Explore → Review" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });

  it("deletes the selected edge with Delete", async () => {
    const user = userEvent.setup();
    stubFetch(() => fakeResponse(200, multi));
    renderEditor("demo");

    await screen.findByTestId("editor-graph");
    await user.click(screen.getByRole("tab", { name: /Edges/ }));
    await user.click(screen.getByRole("button", { name: "Explore → Apply" }));
    await user.keyboard("{Delete}");

    expect(screen.queryByRole("button", { name: "Explore → Apply" })).toBeNull();
    expect(screen.getByText(/Drag from a node's/)).toBeInTheDocument();
  });

  it("deletes the selected node with Delete after confirmation", async () => {
    const user = userEvent.setup();
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    stubFetch(() => fakeResponse(200, source));
    renderEditor("demo");

    await screen.findByTestId("editor-graph");
    await user.keyboard("{Delete}");

    expect(confirm).toHaveBeenCalledWith(
      'Remove node "Explore" and its edges?',
    );
    expect(screen.queryByText("Explore", { selector: "button" })).toBeNull();
    confirm.mockRestore();
  });

  it("does not delete while typing in a form field", async () => {
    const user = userEvent.setup();
    stubFetch(() => fakeResponse(200, source));
    renderEditor("demo");

    await screen.findByTestId("editor-graph");
    await user.click(screen.getByRole("tab", { name: "Workflow" }));
    const name = screen.getByLabelText("Name");
    await user.click(name);
    await user.keyboard("Explore");
    await user.keyboard("{Backspace}");
    expect(name).toHaveValue("DemoExplor");
  });
});

describe("WorkflowEditor selection", () => {
  it("switches the node inspector when another node is selected", async () => {
    const user = userEvent.setup();
    const twoNodes: WorkflowSource = {
      ...source,
      draft: {
        ...draft,
        nodes: [
          { id: "node-1", title: "Explore", kind: "agent" },
          { id: "node-2", title: "Apply", kind: "agent" },
        ],
      },
    };
    stubFetch(() => fakeResponse(200, twoNodes));
    renderEditor("demo");

    await screen.findByTestId("editor-graph");
    expect(screen.getByLabelText("Title")).toHaveValue("Explore");

    await user.click(
      within(screen.getByTestId("editor-graph")).getByRole("button", {
        name: "Apply",
      }),
    );
    expect(screen.getByLabelText("Title")).toHaveValue("Apply");
  });
});

describe("WorkflowEditor edge direction", () => {
  it("reverses the selected edge in one step", async () => {
    const user = userEvent.setup();
    const connected: WorkflowSource = {
      ...source,
      draft: {
        ...draft,
        nodes: [
          { id: "node-1", title: "Explore", kind: "agent" },
          { id: "node-2", title: "Apply", kind: "agent" },
        ],
        edges: [{ from: "node-1", to: "node-2" }],
      },
    };
    stubFetch(() => fakeResponse(200, connected));
    renderEditor("demo");

    await screen.findByTestId("editor-graph");
    await user.click(screen.getByRole("tab", { name: /Edges/ }));
    await user.click(screen.getByRole("button", { name: "Explore → Apply" }));
    await user.click(screen.getByRole("button", { name: "Reverse" }));

    expect(screen.getByRole("button", { name: "Apply → Explore" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });
});

describe("WorkflowEditor skill catalog", () => {
  it("loads installed skills into the multi-select", async () => {
    const user = userEvent.setup();
    stubFetch((url, init) => {
      if (url.includes("/api/skills")) {
        return fakeResponse(200, {
          skills: [
            {
              name: "alpha",
              description: "Alpha skill",
              source: ".specflow/skills",
            },
            {
              name: "ui-ux-pro-max",
              description: "UI/UX design intelligence",
              source: ".dsh/skills",
            },
          ],
        });
      }
      if (init?.method === "PUT") {
        return fakeResponse(200, {
          path: "/repo/.specflow/workflows/demo.yaml",
          hash: "hash-new",
        });
      }
      return fakeResponse(200, source);
    });
    renderEditor("demo");

    await screen.findByTestId("editor-graph");
    await waitFor(() =>
      expect(screen.getByLabelText("Skills").tagName).toBe("BUTTON"),
    );

    const trigger = screen.getByLabelText("Skills");
    await user.click(trigger);
    const panel = screen.getByRole("group", { name: "Skills options" });
    expect(within(panel).getByText(".specflow/skills")).toBeInTheDocument();
    await user.click(
      within(panel).getByRole("checkbox", { name: /ui-ux-pro-max/ }),
    );
    expect(within(trigger).getByText("ui-ux-pro-max")).toBeInTheDocument();
  });
});
