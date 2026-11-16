import { request } from "./client";
import type { HarnessInfo, HarnessesResponse } from "../types/harness";

export async function listHarnesses(): Promise<HarnessInfo[]> {
  const response = await request<HarnessesResponse>("/api/harnesses");
  return response.harnesses ?? [];
}
