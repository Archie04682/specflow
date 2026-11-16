import { describe, expect, it } from "vitest";
import { describeTool } from "./toolArguments";

describe("describeTool", () => {
  it("renders a shell command from a command tool", () => {
    expect(describeTool("bash", { command: "make test", timeout: 30 })).toEqual({
      kind: "command",
      command: "make test",
      timeout: 30,
    });
  });

  it("renders a read range", () => {
    expect(describeTool("read", { path: "src/a.ts", offset: 10, limit: 10 })).toEqual({
      kind: "file",
      path: "src/a.ts",
      detail: "lines 10–19",
    });
  });

  it("renders a write size instead of its content", () => {
    expect(describeTool("write", { path: "a.ts", content: "hello" })).toEqual({
      kind: "file",
      path: "a.ts",
      detail: "5 chars",
    });
  });

  it("renders an edit replacement size", () => {
    expect(
      describeTool("edit", { file_path: "a.ts", oldString: "one", newString: "three" }),
    ).toEqual({ kind: "file", path: "a.ts", detail: "3 → 5 chars" });
  });

  it("renders a search pattern and path", () => {
    expect(describeTool("grep", { pattern: "TODO", path: "src" })).toEqual({
      kind: "search",
      pattern: "TODO",
      path: "src",
    });
    expect(describeTool("find", { pattern: "*.go" })).toEqual({
      kind: "search",
      pattern: "*.go",
      path: undefined,
    });
  });

  it("renders a list path", () => {
    expect(describeTool("ls", { path: "backend" })).toEqual({
      kind: "file",
      path: "backend",
      detail: undefined,
    });
    expect(describeTool("ls", undefined)).toEqual({
      kind: "file",
      path: ".",
      detail: undefined,
    });
  });

  it("renders the ask tool as a question", () => {
    expect(
      describeTool("specflow_ask", {
        title: "Pick one",
        question: "Which one?",
        options: ["a", "b"],
      }),
    ).toEqual({
      kind: "question",
      title: "Pick one",
      description: "Which one?",
      options: ["a", "b"],
    });
  });

  it("falls back to the argument shape for an unknown tool", () => {
    expect(describeTool("run_shell", { command: "ls" })).toEqual({
      kind: "command",
      command: "ls",
      timeout: undefined,
    });
    expect(describeTool("open_file", { path: "a.ts" })).toEqual({
      kind: "file",
      path: "a.ts",
    });
  });

  it("lists unknown arguments as labelled rows", () => {
    expect(describeTool("mystery", { alpha: "one", beta: 2, gamma: true })).toEqual({
      kind: "generic",
      entries: [
        { label: "alpha", value: "one" },
        { label: "beta", value: "2" },
        { label: "gamma", value: "true" },
      ],
    });
  });

  it("survives a tool with no arguments", () => {
    expect(describeTool("mystery", undefined)).toEqual({ kind: "generic", entries: [] });
  });
});
