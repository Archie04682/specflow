// Wire types for GET /api/harnesses.

export interface Capabilities {
  streaming: boolean;
  thinking: boolean;
  toolEvents: boolean;
  fileEvents: boolean;
  questions: boolean;
  injection: boolean;
  abort: boolean;
  skills: boolean;
  modelSelect: boolean;
  toolPolicy: boolean;
  plugins: boolean;
  usage: boolean;
  sessionResume: boolean;
}

export interface ModelOption {
  /** The value a workflow stores, for example "deepseek/deepseek-flash". */
  id: string;
  name?: string;
  provider?: string;
  reasoning: boolean;
  /** Levels this model supports; absent means the harness-wide list. */
  thinkingLevels?: string[];
}

export interface ToolOption {
  name: string;
  description?: string;
}

export interface HarnessInfo {
  id: string;
  name: string;
  version: string;
  protocol: number;
  ready: boolean;
  readyError?: string;
  capabilities: Capabilities;
  /** Optional catalog; adapters that cannot describe choices omit it. */
  models?: ModelOption[];
  thinkingLevels?: string[];
  tools?: ToolOption[];
  catalogError?: string;
}

export interface HarnessesResponse {
  harnesses: HarnessInfo[];
}
