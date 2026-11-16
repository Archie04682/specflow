// SpecFlow's Pi bridge.
//
// Pi has no built-in "ask the user" tool: its dialog sub-protocol only fires
// when an extension calls ctx.ui. This extension registers exactly one tool,
// specflow_ask, which renders as an extension_ui_request over RPC. The
// SpecFlow adapter answers it from the web UI, so a workflow node can question
// the operator without Pi's terminal interface existing at all.
//
// Keep this file dependency-free: it uses only a type-only import so loading it
// never requires a package install next to the user's Pi.
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

// BRIDGE_MARKER is emitted once per session so the adapter can tell that this
// extension loaded. The adapter swallows the notification; it is a handshake,
// not something the user needs to read.
const BRIDGE_MARKER = "specflow-bridge:ready";

export default function specflowBridge(pi: ExtensionAPI) {
  pi.registerTool({
    name: "specflow_ask",
    label: "Ask the user",
    description:
      "Ask the human operator a question and wait for the answer. Use this when a decision, " +
      "a missing fact, or an approval blocks progress and guessing would be wrong.",
    parameters: {
      type: "object",
      properties: {
        question: {
          type: "string",
          description: "The question, stated so it can be answered without extra context.",
        },
        kind: {
          type: "string",
          enum: ["input", "select", "confirm", "editor"],
          description: "How to answer: free text, one of several options, yes/no, or a long text.",
        },
        options: {
          type: "array",
          items: { type: "string" },
          description: "The allowed answers when kind is select.",
        },
        title: { type: "string", description: "A short heading for the question." },
        placeholder: { type: "string", description: "A hint shown next to a free-text answer." },
        prefill: { type: "string", description: "Initial text when kind is editor." },
      },
      required: ["question"],
      additionalProperties: false,
    },
    async execute(_toolCallId: string, params: any, _signal: any, _onUpdate: any, ctx: any) {
      const question = String(params.question ?? "");
      const title = params.title ?? "SpecFlow question";
      const kind = params.kind ?? "input";

      let answer = "";
      try {
        if (kind === "select") {
          const options = Array.isArray(params.options) ? params.options.map(String) : [];
          answer = (await ctx.ui.select(title, options)) ?? "";
        } else if (kind === "confirm") {
          const confirmed = await ctx.ui.confirm(title, question);
          answer = confirmed ? "yes" : "no";
        } else if (kind === "editor") {
          answer = (await ctx.ui.editor(title, params.prefill ?? "")) ?? "";
        } else {
          answer = (await ctx.ui.input(title, params.placeholder ?? "")) ?? "";
        }
      } catch (error) {
        // A cancelled dialog or a disconnected client is not a tool failure:
        // the model gets a fact it can act on instead of a broken turn.
        return {
          content: [{ type: "text", text: `No answer was available: ${String(error)}` }],
          details: {},
        };
      }

      return {
        content: [{ type: "text", text: answer === "" ? "(the user gave no answer)" : answer }],
        details: {},
      };
    },
  });

  pi.on("session_start", async (_event: unknown, ctx: any) => {
    ctx.ui.notify(BRIDGE_MARKER, "info");
  });
}
