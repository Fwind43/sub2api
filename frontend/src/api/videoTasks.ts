import { apiClient } from './client'
import type { BasePaginationResponse } from '@/types'

export interface VideoTask {
  id: string
  status: string
  delivery_status: string
  billing_status: string
  /** Price locked at submission; only charged once billing_status is settled. */
  cost: number | null
  queue_position?: number
  finished_at?: string
  created_at: string
}

export async function listVideoTasks(page: number, pageSize: number, signal: AbortSignal, status = '') {
  const { data } = await apiClient.get<BasePaginationResponse<VideoTask>>('/user/video-tasks', {
    params: { page, page_size: pageSize, ...(status ? { status } : {}) },
    signal,
  })
  return data
}

export async function refreshVideoTask(id: string) {
  const { data } = await apiClient.post<VideoTask>(
    `/user/video-tasks/${encodeURIComponent(id)}/refresh`,
    undefined,
    // Refresh may download and verify the finished MP4 before settling.
    { timeout: 120_000 },
  )
  return data
}

export async function fetchVideoTaskContent(id: string, signal?: AbortSignal) {
  const { data } = await apiClient.get<Blob>(`/user/video-tasks/${encodeURIComponent(id)}/content`, {
    responseType: 'blob',
    timeout: 120_000,
    signal,
  })
  return data
}
