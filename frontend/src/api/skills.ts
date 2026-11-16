import { request } from "./client";
import type { SkillCatalogResponse, SkillInfo } from "../types/skill";

export async function listSkills(project: string): Promise<SkillInfo[]> {
  const response = await request<SkillCatalogResponse>("/api/skills", { project });
  return response.skills ?? [];
}
