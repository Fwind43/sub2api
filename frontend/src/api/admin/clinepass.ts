/**
 * Admin ClinePass API endpoints.
 * Wraps the ClinePass device-code OAuth flow for administrators.
 */

import { apiClient } from '../client'

export interface ClinePassOAuthCapabilities {
  device_code_supported?: boolean
}

export interface ClinePassDeviceStartRequest {
  proxy_id?: number | null
}

export interface ClinePassDeviceStartResponse {
  device_code: string
  user_code: string
  verification_uri: string
  verification_uri_complete?: string
  interval?: number
  expires_in?: number
}

export interface ClinePassTokenInfo {
  access_token?: string
  refresh_token?: string
  token_type?: string
  expires_at?: number | string
  email?: string
  user_id?: string
  [key: string]: unknown
}

export interface ClinePassDevicePollRequest {
  device_code: string
  proxy_id?: number | null
}

export type ClinePassDevicePollResponse =
  | { status: 'pending' }
  | { status: 'approved'; token_info: ClinePassTokenInfo }

export interface ClinePassCreateFromDeviceRequest {
  device_code: string
  name?: string
  proxy_id?: number | null
  concurrency?: number
  priority?: number
  group_ids?: number[]
}

export type ClinePassCreateFromDeviceResponse =
  | { status: 'pending' }
  | { status: 'approved'; account: unknown }

export interface ClinePassModelsRequest {
  access_token?: string
  refresh_token?: string
  proxy_id?: number | null
}

export interface ClinePassModelEntry {
  id?: string
  name?: string
  [key: string]: unknown
}

const CLINEPASS_BASE = '/admin/clinepass'

export async function getCapabilities(): Promise<ClinePassOAuthCapabilities> {
  const { data } = await apiClient.get<ClinePassOAuthCapabilities>(`${CLINEPASS_BASE}/oauth/capabilities`)
  return data
}

export async function startDeviceAuth(
  payload: ClinePassDeviceStartRequest = {}
): Promise<ClinePassDeviceStartResponse> {
  const { data } = await apiClient.post<ClinePassDeviceStartResponse>(
    `${CLINEPASS_BASE}/oauth/device/start`,
    payload
  )
  return data
}

export async function pollDeviceAuth(
  payload: ClinePassDevicePollRequest
): Promise<ClinePassDevicePollResponse> {
  const { data } = await apiClient.post<ClinePassDevicePollResponse>(
    `${CLINEPASS_BASE}/oauth/device/poll`,
    payload
  )
  return data
}

export async function createFromDevice(
  payload: ClinePassCreateFromDeviceRequest
): Promise<ClinePassCreateFromDeviceResponse> {
  const { data } = await apiClient.post<ClinePassCreateFromDeviceResponse>(
    `${CLINEPASS_BASE}/oauth/device/create`,
    payload
  )
  return data
}

export async function refreshToken(
  refreshTokenValue: string,
  proxyId?: number | null
): Promise<ClinePassTokenInfo> {
  const payload: Record<string, unknown> = { refresh_token: refreshTokenValue }
  if (proxyId) payload.proxy_id = proxyId
  const { data } = await apiClient.post<ClinePassTokenInfo>(
    `${CLINEPASS_BASE}/oauth/refresh-token`,
    payload
  )
  return data
}

export async function refreshAccount(id: number): Promise<unknown> {
  const { data } = await apiClient.post(`${CLINEPASS_BASE}/accounts/${id}/refresh`)
  return data
}

export async function listModels(
  payload: ClinePassModelsRequest
): Promise<ClinePassModelEntry[]> {
  const { data } = await apiClient.post<{ models: ClinePassModelEntry[] }>(
    `${CLINEPASS_BASE}/oauth/models`,
    payload
  )
  return data?.models ?? []
}


export interface ClinePassUpstreamProbeResult {
  model: string
  vercel?: string[]
  openrouter?: string[]
  vercel_raw?: string
  openrouter_raw?: string
}

export async function probeUpstreams(
  accountId: number,
  model: string
): Promise<ClinePassUpstreamProbeResult> {
  const { data } = await apiClient.post<ClinePassUpstreamProbeResult>(
    `/admin/accounts/${accountId}/clinepass-upstreams/probe`,
    { model }
  )
  return data
}

export default {
  getCapabilities,
  startDeviceAuth,
  pollDeviceAuth,
  createFromDevice,
  refreshToken,
  refreshAccount,
  listModels
}
