// The closed set RenderPrompt substitutes at run start. Keep this list in sync
// with orchestrator.RunContext.Values: an unknown placeholder fails compilation
// before the node starts, so the editor only offers values the server knows.
export interface PromptVariable {
  token: string;
  label: string;
  description: string;
}

export const PROMPT_VARIABLES: PromptVariable[] = [
  {
    token: "objective",
    label: "Objective",
    description: "The run's objective text.",
  },
  {
    token: "change",
    label: "Change",
    description: "The OpenSpec change id for this run.",
  },
  {
    token: "run_id",
    label: "Run ID",
    description: "The current run identifier.",
  },
  {
    token: "workdir",
    label: "Workdir",
    description: "The directory the node executes in.",
  },
];
