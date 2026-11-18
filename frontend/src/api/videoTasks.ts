import { apiClient } from './client'
import type { BasePaginationResponse } from '@/types'

export interface VideoTask {
  id: string
  status: string
  delivery_status: string
  billing_status: string
  created_at: string
}

export async function listVideoTasks(page: number, pageSize: number, signal: AbortSignal) {
  const { data } = await apiClient.get<BasePaginationResponse<VideoTask>>('/user/video-tasks', {
    params: { page, page_size: pageSize },
    signal,
  })
  return data
}
