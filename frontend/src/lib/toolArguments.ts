// Tool calls are rendered by what the tool does, not by dumping its arguments.
// This module is the pure part: it turns a tool name and its arguments into a
// description a component can render. It is deliberately harness-agnostic:
// known names come first, then the argument shape decides, then labelled rows.

export type ToolCallDescription =
  | { kind: "command"; command: string; timeout?: number }
  | { kind: "file"; path: string; detail?: string }
  | { kind: "search"; pattern: string; path?: string }
  | { kind: "question"; title?: string; description?: string; options: string[] }
  | { kind: "generic"; entries: { label: string; value: string }[] };

const COMMAND_TOOLS = new Set(["bash", "shell", "sh", "powershell", "zsh"]);
const FILE_TOOLS = new Set(["read", "write", "edit"]);
const SEARCH_TOOLS = new Set(["grep", "find"]);
const LIST_TOOLS = new Set(["ls"]);
const ASK_TOOL = "specflow_ask";

const MAX_ARG_VALUE = 2000;

function asText(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function asNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function asStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((entry): entry is string => typeof entry === "string");
}

function pathOf(args: Record<string, unknown> | undefined): string | undefined {
  if (!args) return undefined;
  for (const key of ["path", "file_path", "filePath"]) {
    const value = asText(args[key]);
    if (value) return value;
  }
  return undefined;
}

function rangeDetail(args: Record<string, unknown> | undefined): string | undefined {
  if (!args) return undefined;
  const offset = asNumber(args.offset);
  const limit = asNumber(args.limit);
  if (limit !== undefined && offset !== undefined) {
    return `lines ${offset}–${offset + limit - 1}`;
  }
  if (limit !== undefined) return `${limit} lines`;
  if (offset !== undefined) return `from line ${offset}`;
  return undefined;
}

function fileDetail(name: string, args: Record<string, unknown> | undefined): string | undefined {
  if (!args) return undefined;
  if (name === "write") {
    const content = asText(args.content);
    return content ? `${content.length} chars` : undefined;
  }
  if (name === "edit") {
    const from = asText(args.oldString ?? args.old_string);
    const to = asText(args.newString ?? args.new_string);
    if (from !== undefined && to !== undefined) {
      return `${from.length} → ${to.length} chars`;
    }
    return "edit";
  }
  return rangeDetail(args);
}

function summarizeValue(value: unknown): string {
  if (typeof value === "string") {
    return value.length > MAX_ARG_VALUE ? `${value.slice(0, MAX_ARG_VALUE)}…` : value;
  }
  if (value === null || value === undefined) return "—";
  try {
    const serialized = JSON.stringify(value);
    if (serialized === undefined) return String(value);
    return serialized.length > MAX_ARG_VALUE
      ? `${serialized.slice(0, MAX_ARG_VALUE)}…`
      : serialized;
  } catch {
    return String(value);
  }
}

function genericEntries(
  args: Record<string, unknown> | undefined,
): { label: string; value: string }[] {
  if (!args) return [];
  return Object.entries(args).map(([label, value]) => ({
    label,
    value: summarizeValue(value),
  }));
}

export function describeTool(
  name: string,
  args?: Record<string, unknown>,
): ToolCallDescription {
  if (name === ASK_TOOL) {
    return {
      kind: "question",
      title: args ? asText(args.title) : undefined,
      description: args ? asText(args.question) : undefined,
      options: args ? asStringArray(args.options) : [],
    };
  }

  const command = args ? asText(args.command) : undefined;
  const pattern = args ? asText(args.pattern) : undefined;
  const path = pathOf(args);

  if (COMMAND_TOOLS.has(name) && command) {
    return { kind: "command", command, timeout: args ? asNumber(args.timeout) : undefined };
  }

  if (SEARCH_TOOLS.has(name) && pattern) {
    return { kind: "search", pattern, path };
  }

  if (FILE_TOOLS.has(name) || LIST_TOOLS.has(name)) {
    return {
      kind: "file",
      path: path ?? ".",
      detail: FILE_TOOLS.has(name) ? fileDetail(name, args) : undefined,
    };
  }

  // An unknown harness may still name its shell or file tool something else;
  // the argument shape is a better signal than the name we do not know.
  if (command) {
    return { kind: "command", command, timeout: args ? asNumber(args.timeout) : undefined };
  }
  if (pattern) return { kind: "search", pattern, path };
  if (path) return { kind: "file", path };

  return { kind: "generic", entries: genericEntries(args) };
}
