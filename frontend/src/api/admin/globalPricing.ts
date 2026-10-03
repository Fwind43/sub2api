/**
 * Admin Global Model Pricing API endpoints
 * Maintains a single per-model price table that applies across every platform.
 */

import { apiClient } from '../client'

/** Raw LiteLLM-compatible price entry, stored per-token. */
export type GlobalPricingEntry = Record<string, unknown>

export interface GlobalPricingListResponse {
  entries: Record<string, GlobalPricingEntry>
  file: string
}

export interface GlobalPricingSaveResponse {
  model: string
}

/**
 * List every model that has a global unified price configured.
 */
export async function list(): Promise<GlobalPricingListResponse> {
  const { data } = await apiClient.get<GlobalPricingListResponse>('/admin/model-pricing/global')
  return data
}

/**
 * Create or update the global unified price of one model.
 * Body is a LiteLLM-compatible price object (per-token); null deletes a field
 * from the underlying catalog entry.
 */
export async function save(model: string, pricing: GlobalPricingEntry): Promise<GlobalPricingSaveResponse> {
  const { data } = await apiClient.put<GlobalPricingSaveResponse>('/admin/model-pricing/global', pricing, {
    params: { model }
  })
  return data
}

/**
 * Remove the global unified price of one model (falls back to the built-in catalog).
 */
export async function remove(model: string): Promise<GlobalPricingSaveResponse> {
  const { data } = await apiClient.delete<GlobalPricingSaveResponse>('/admin/model-pricing/global', {
    params: { model }
  })
  return data
}

/**
 * Rebuild the in-memory price table after editing the file on disk directly.
 */
export async function reload(): Promise<void> {
  await apiClient.post('/admin/model-pricing/global/reload')
}

const globalPricingAPI = { list, save, remove, reload }
export default globalPricingAPI
