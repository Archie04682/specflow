import { request } from "./client";
import type { ServerIdentity } from "../types/server";

// Which build is answering this page, from the server itself.
export async function getServerIdentity(): Promise<ServerIdentity> {
  return request<ServerIdentity>("/api/server");
}
