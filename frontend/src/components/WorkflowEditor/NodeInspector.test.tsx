import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { HarnessInfo } from "../../types/harness";
import type { SkillInfo } from "../../types/skill";
import type { WorkflowDraft, WorkflowNodeDraft } from "../../types/workflowDraft";
import { NodeInspector } from "./NodeInspector";

const draft: WorkflowDraft = {
  id: "demo",
  defaults: { harness: "pi" },
  nodes: [],
  edges: [],
};

const node: WorkflowNodeDraft = {
  id: "node-1",
  title: "Explore",
  kind: "agent",
};

function Harness({
  initial,
  onChange,
  onRemove,
  skills,
  skillsError,
  harnesses,
  harnessesError,
}: {
  initial: WorkflowNodeDraft;
  onChange: (patch: Partial<WorkflowNodeDraft>) => void;
  onRemove: () => void;
  skills?: SkillInfo[] | null;
  skillsError?: string | null;
  harnesses?: HarnessInfo[] | null;
  harnessesError?: string | null;
}) {
  const [current, setCurrent] = useState(initial);
  const apply = (patch: Partial<WorkflowNodeDraft>) => {
    setCurrent((previous) => ({ ...previous, ...patch }));
    onChange(patch);
  };

  return (
    <NodeInspector
      draft={draft}
      node={current}
      onChange={apply}
      onRemove={onRemove}
      skills={skills}
      skillsError={skillsError}
      harnesses={harnesses}
      harnessesError={harnessesError}
      onClose={() => {}}
    />
  );
}

function renderInspector(
  initial: WorkflowNodeDraft = node,
  onChange = vi.fn(),
  onRemove = vi.fn(),
  skills?: SkillInfo[] | null,
  skillsError?: string | null,
  harnesses?: HarnessInfo[] | null,
  harnessesError?: string | null,
) {
  render(
    <Harness
      initial={initial}
      onChange={onChange}
      onRemove={onRemove}
      skills={skills}
      skillsError={skillsError}
      harnesses={harnesses}
      harnessesError={harnessesError}
    />,
  );
  return { onChange, onRemove };
}

function catalogHarness(): HarnessInfo {
  return {
    id: "pi",
    name: "Pi",
    version: "1.0.0",
    protocol: 1,
    ready: true,
    capabilities: {
      streaming: true,
      thinking: true,
      toolEvents: true,
      fileEvents: true,
      questions: true,
      injection: true,
      abort: true,
      skills: true,
      modelSelect: true,
      toolPolicy: true,
      plugins: true,
      usage: true,
      sessionResume: false,
    },
    models: [
      {
        id: "deepseek/deepseek-flash",
        name: "DeepSeek Flash",
        provider: "deepseek",
        reasoning: true,
        thinkingLevels: ["off", "low", "high"],
      },
      {
        id: "plain/model",
        name: "Plain model",
        provider: "plain",
        reasoning: false,
        thinkingLevels: ["off"],
      },
    ],
    thinkingLevels: ["off", "minimal", "low", "medium", "high"],
    tools: [
      { name: "read", description: "Read files" },
      { name: "bash", description: "Run commands" },
    ],
  };
}

describe("NodeInspector", () => {
  it("stacks kind, title, harness, and skills in that order", () => {
    renderInspector(node, vi.fn(), vi.fn(), [
      { name: "alpha", source: ".specflow/skills" },
    ]);

    const kind = screen.getByLabelText("Kind");
    const title = screen.getByLabelText("Title");
    const harness = screen.getByLabelText("Harness");
    const skills = screen.getByLabelText("Skills");
    const follows = (first: HTMLElement, second: HTMLElement) =>
      Boolean(
        first.compareDocumentPosition(second) &
          Node.DOCUMENT_POSITION_FOLLOWING,
      );

    expect(follows(kind, title)).toBe(true);
    expect(follows(title, harness)).toBe(true);
    expect(follows(harness, skills)).toBe(true);
  });

  it("edits the title and kind", async () => {
    const user = userEvent.setup();
    const { onChange } = renderInspector();

    await user.clear(screen.getByLabelText("Title"));
    await user.type(screen.getByLabelText("Title"), "Explore repo");
    expect(screen.getByLabelText("Title")).toHaveValue("Explore repo");
    expect(onChange).toHaveBeenLastCalledWith({ title: "Explore repo" });

    await user.selectOptions(screen.getByLabelText("Kind"), "command");
    expect(onChange).toHaveBeenLastCalledWith({ kind: "command" });
    expect(screen.getByLabelText("Command")).toBeInTheDocument();
  });

  it("commits tools on blur, not on every keystroke", async () => {
    const user = userEvent.setup();
    const { onChange } = renderInspector();

    const tools = screen.getByLabelText("Tools");
    await user.type(tools, "read, write");
    expect(onChange).not.toHaveBeenCalledWith({ tools: ["read", "write"] });

    await user.tab();
    expect(onChange).toHaveBeenCalledWith({ tools: ["read", "write"] });
  });

  it("adds, changes, and removes a postcondition", async () => {
    const user = userEvent.setup();
    const { onChange } = renderInspector();

    await user.click(screen.getByRole("button", { name: "Add postcondition" }));
    expect(onChange).toHaveBeenCalledWith({ postcondition: { type: "none" } });
  });

  it("shows the failure domain fields for the selected postcondition", async () => {
    const user = userEvent.setup();
    const withPostcondition: WorkflowNodeDraft = {
      ...node,
      postcondition: { type: "file_exists", path: "README.md" },
    };
    const { onChange } = renderInspector(withPostcondition);

    expect(screen.getByLabelText("Path")).toHaveValue("README.md");
    await user.selectOptions(screen.getByLabelText("Type"), "new_change");
    expect(onChange).toHaveBeenCalledWith({
      postcondition: { type: "new_change", path: "README.md" },
    });

    await user.click(screen.getByRole("button", { name: "Remove postcondition" }));
    expect(onChange).toHaveBeenCalledWith({ postcondition: undefined });
  });

  it("removes the node on request", async () => {
    const user = userEvent.setup();
    const { onRemove } = renderInspector();
    await user.click(screen.getByRole("button", { name: "Remove node" }));
    expect(onRemove).toHaveBeenCalledTimes(1);
  });
});

describe("NodeInspector skill catalog", () => {
  it("selects multiple installed skills from one multi-select", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderInspector(node, onChange, vi.fn(), [
      { name: "alpha", source: ".specflow/skills", description: "Alpha skill" },
      {
        name: "ui-ux-pro-max",
        source: ".dsh/skills",
        description: "UI/UX design intelligence",
      },
    ]);

    const trigger = screen.getByLabelText("Skills");
    expect(trigger.tagName).toBe("BUTTON");
    await user.click(trigger);

    const panel = screen.getByRole("group", { name: "Skills options" });
    expect(within(panel).getByText(".specflow/skills")).toBeInTheDocument();
    expect(within(panel).getByText(".dsh/skills")).toBeInTheDocument();

    await user.click(within(panel).getByRole("checkbox", { name: /ui-ux-pro-max/ }));
    expect(onChange).toHaveBeenLastCalledWith({
      skills: ["ui-ux-pro-max"],
      skill: undefined,
    });

    await user.click(within(panel).getByRole("checkbox", { name: /alpha/ }));
    expect(onChange).toHaveBeenLastCalledWith({
      skills: ["ui-ux-pro-max", "alpha"],
      skill: undefined,
    });
    expect(
      screen.queryByText(/Their instructions are combined in the node prompt/),
    ).toBeNull();
  });

  it("keeps an uninstalled skill visible and marks it", async () => {
    const user = userEvent.setup();
    renderInspector({ ...node, skill: "legacy-skill" }, vi.fn(), vi.fn(), [
      { name: "alpha", source: ".specflow/skills" },
    ]);

    await user.click(screen.getByLabelText("Skills"));
    const panel = screen.getByRole("group", { name: "Skills options" });
    expect(within(panel).getByText("Not installed")).toBeInTheDocument();
    expect(
      within(panel).getByRole("checkbox", { name: /legacy-skill/ }),
    ).toBeChecked();
    expect(screen.getByText(/Not installed: legacy-skill/)).toBeInTheDocument();
  });

  it("falls back to a comma-separated input when the catalog is unavailable", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderInspector(node, onChange, vi.fn(), null, "network down");

    const input = screen.getByLabelText("Skills");
    expect(input.tagName).toBe("INPUT");
    expect(input).toHaveAttribute("aria-describedby", "node-skill-hint");
    expect(
      screen.getByText(/Could not list installed skills/),
    ).toBeInTheDocument();

    await user.type(input, "alpha, beta");
    await user.tab();
    expect(onChange).toHaveBeenCalledWith({
      skills: ["alpha", "beta"],
      skill: undefined,
    });
  });
});


describe("NodeInspector harness catalog", () => {
  it("renders model and thinking as selects driven by the catalog", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderInspector(node, onChange, vi.fn(), null, null, [catalogHarness()]);

    const model = screen.getByLabelText("Model");
    expect(model.tagName).toBe("SELECT");
    expect(
      screen.getByRole("option", { name: /DeepSeek Flash/ }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("option", { name: /Plain model/ }),
    ).toBeInTheDocument();

    await user.selectOptions(model, "deepseek/deepseek-flash");
    expect(onChange).toHaveBeenCalledWith({
      model: { id: "deepseek/deepseek-flash" },
    });

    const thinking = screen.getByLabelText("Thinking");
    expect(thinking.tagName).toBe("SELECT");
    await user.selectOptions(thinking, "high");
    expect(onChange).toHaveBeenLastCalledWith({
      model: { id: "deepseek/deepseek-flash", thinking: "high" },
    });

    expect(screen.getByText(/Supported: off, low, high/)).toBeInTheDocument();
  });

  it("drops an unsupported thinking level when the model changes", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderInspector(
      {
        ...node,
        model: { id: "deepseek/deepseek-flash", thinking: "high" },
      },
      onChange,
      vi.fn(),
      null,
      null,
      [catalogHarness()],
    );

    expect(screen.getByLabelText("Thinking")).toHaveValue("high");
    await user.selectOptions(screen.getByLabelText("Model"), "plain/model");
    expect(onChange).toHaveBeenCalledWith({
      model: { id: "plain/model" },
    });
    expect(screen.getByText(/does not report reasoning/)).toBeInTheDocument();
    expect(screen.getByLabelText("Thinking")).toBeDisabled();
  });

  it("selects tools in a multi-select and accepts a custom tool", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderInspector(node, onChange, vi.fn(), null, null, [catalogHarness()]);

    await user.click(screen.getByLabelText("Tools"));
    const panel = screen.getByRole("group", { name: "Tools options" });
    await user.click(within(panel).getByRole("checkbox", { name: /read/ }));
    expect(onChange).toHaveBeenCalledWith({ tools: ["read"] });

    await user.type(
      within(panel).getByLabelText("Add a custom tool"),
      "custom_tool",
    );
    await user.click(within(panel).getByRole("button", { name: "Add" }));
    expect(onChange).toHaveBeenLastCalledWith({
      tools: ["read", "custom_tool"],
    });

    const currentPanel = screen.getByRole("group", { name: "Tools options" });
    const customCheckbox = within(currentPanel).getByRole("checkbox", {
      name: /custom_tool/,
    });
    expect(customCheckbox).toBeChecked();
    await user.click(customCheckbox);
    expect(onChange).toHaveBeenLastCalledWith({ tools: ["read"] });
  });

  it("clears the tool allowlist", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderInspector(
      { ...node, tools: ["read"] },
      onChange,
      vi.fn(),
      null,
      null,
      [catalogHarness()],
    );

    await user.click(screen.getByLabelText("Tools"));
    const panel = screen.getByRole("group", { name: "Tools options" });
    await user.click(
      within(panel).getByRole("button", { name: "Clear Tools" }),
    );
    expect(onChange).toHaveBeenCalledWith({ tools: undefined });
  });

  it("inserts a prompt variable at the caret", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderInspector(
      { ...node, prompt: "Explore this. " },
      onChange,
      vi.fn(),
      null,
      null,
      [catalogHarness()],
    );

    const prompt = screen.getByLabelText("Prompt");
    (prompt as HTMLTextAreaElement).setSelectionRange(
      (prompt as HTMLTextAreaElement).value.length,
      (prompt as HTMLTextAreaElement).value.length,
    );
    await user.click(
      screen.getByRole("button", { name: "Insert {{objective}}" }),
    );
    expect(onChange).toHaveBeenCalledWith({
      prompt: "Explore this. {{objective}}",
    });
    expect(
      screen.getByRole("button", { name: "Insert {{change}}" }),
    ).toBeInTheDocument();
  });
});
