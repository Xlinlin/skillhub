import { apiFetch, apiFetchRaw } from './client';

export interface AgentSummary {
  id: string; slug: string; displayName: string; kind: 'single' | 'suite'; summary: string;
  tags: string[]; ownerId: string; ownerHandle: string; latestVersion: string;
  versionsCount: number; createdAt: string; updatedAt: string;
}
export interface AgentVersion {
  id: string; version: string; instructions: string; changelog: string;
  files: { path: string; size: number; sha256: string }[];
  archiveSHA256: string; archiveSize: number; createdBy: string; createdAt: string;
}
export interface AgentDetail { agent: AgentSummary; version: AgentVersion }
export interface AgentPage<T> { data: T[]; page: number; pageSize: number; total: number }
export type AgentVersionSummary = Pick<AgentVersion, 'id' | 'version' | 'changelog' | 'createdAt'>;
const base = (slug: string) => `/agents/${encodeURIComponent(slug)}`;
export function listAgents(params: URLSearchParams, signal?: AbortSignal) {
  return apiFetch<AgentPage<AgentSummary>>(`/agents?${params}`, { signal });
}
export function getAgent(slug: string, version = 'latest', signal?: AbortSignal) {
  return apiFetch<AgentDetail>(`${base(slug)}?version=${encodeURIComponent(version)}`, { signal });
}
export function listAgentVersions(slug: string, page = 1, signal?: AbortSignal) {
  return apiFetch<AgentPage<AgentVersionSummary>>(`${base(slug)}/versions?page=${page}&pageSize=20`, { signal });
}
export async function getAgentFile(slug: string, version: string, path: string, signal?: AbortSignal) {
  const response = await apiFetchRaw(`${base(slug)}/file?${new URLSearchParams({ version, path })}`, { signal });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new Error(body.error || `HTTP ${response.status}`);
  }
  return response.text();
}
export const deleteAgent = (slug: string) => apiFetch<{ message: string }>(base(slug), { method: 'DELETE' });
export const agentDownloadURL = (slug: string, version: string) => `/api/v1${base(slug)}/download?version=${encodeURIComponent(version)}`;
export const publishAgent = (body: FormData) => apiFetch<AgentDetail>('/agents', { method: 'POST', body });
