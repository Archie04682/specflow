export interface SkillInfo {
  name: string;
  description?: string;
  source: string;
}

export interface SkillCatalogResponse {
  skills: SkillInfo[];
}
