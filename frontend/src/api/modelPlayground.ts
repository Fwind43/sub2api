import { apiClient } from './client'

export interface PlaygroundKeyRef {
  apiKeyId?: number | string | null
  apiKey?: string
}

export type ImageApiMode = 'images' | 'responses'

export interface ImageGenerationRequest extends PlaygroundKeyRef {
  apiMode: ImageApiMode
  model: string
  prompt: string
  size?: string
  quality?: string
  outputFormat?: 'png' | 'jpeg' | 'webp'
  outputCompression?: number | null
  moderation?: 'auto' | 'low'
  n?: number
  timeout?: number
  codexCli?: boolean
  inputImages?: string[]
}

export interface GeneratedImage {
  src: string
  revisedPrompt?: string
  actualParams?: Record<string, unknown>
}

export interface ImageGenerationResult {
  images: GeneratedImage[]
  actualParams?: Record<string, unknown>
  raw: unknown
}

export type ImageTaskStatus = 'queued' | 'running' | 'done' | 'error'

export interface BackendImageTask {
  id: string
  user_id: number
  api_key_id?: number | null
  api_mode: ImageApiMode
  model: string
  prompt: string
  params: Record<string, any>
  input_images: string[]
  output_images: string[]
  input_image_count?: number
  output_image_count?: number
  revised_prompt_by_image?: Record<string, string>
  actual_params?: Record<string, unknown>
  raw_response?: Record<string, unknown>
  codex_cli: boolean
  status: ImageTaskStatus
  error?: string | null
  created_at: string
  started_at?: string | null
  finished_at?: string | null
  elapsed_ms?: number | null
}

function keyPayload(request: PlaygroundKeyRef): Record<string, unknown> {
  const payload: Record<string, unknown> = {}
  if (request.apiKeyId !== undefined && request.apiKeyId !== null && request.apiKeyId !== '') {
    const numeric = Number(request.apiKeyId)
    payload.api_key_id = Number.isFinite(numeric) ? numeric : request.apiKeyId
  } else if (request.apiKey) {
    payload.api_key = request.apiKey
  }
  return payload
}

function imageTaskPayload(request: ImageGenerationRequest): Record<string, unknown> {
  return {
    ...keyPayload(request),
    api_mode: request.apiMode,
    model: request.model,
    prompt: request.prompt,
    timeout_seconds: request.timeout || 300,
    params: {
      size: request.size || 'auto',
      quality: request.quality || 'auto',
      output_format: request.outputFormat || 'png',
      output_compression: request.outputCompression ?? null,
      moderation: request.moderation || 'auto',
      n: request.n || 1
    },
    input_images: request.inputImages || [],
    codex_cli: !!request.codexCli
  }
}

function mimeForFormat(format?: string): string {
  switch (format) {
    case 'jpeg':
      return 'image/jpeg'
    case 'webp':
      return 'image/webp'
    default:
      return 'image/png'
  }
}

function pickActualParams(value: any): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const key of ['size', 'quality', 'output_format', 'output_compression', 'moderation', 'n']) {
    if (value?.[key] !== undefined && value?.[key] !== null) out[key] = value[key]
  }
  return out
}

function parseImagesAPI(raw: any): GeneratedImage[] {
  const data = Array.isArray(raw?.data) ? raw.data : []
  return data
    .map((item: any) => {
      const format = item?.output_format || item?.actual_params?.output_format || raw?.output_format || 'png'
      const src = item?.b64_json ? `data:${mimeForFormat(format)};base64,${item.b64_json}` : item?.url
      if (!src) return null
      return {
        src,
        revisedPrompt: item?.revised_prompt,
        actualParams: { ...pickActualParams(raw), ...pickActualParams(item), ...(item?.actual_params || {}) }
      }
    })
    .filter(Boolean)
}

function parseResponsesAPI(raw: any): GeneratedImage[] {
  const output = Array.isArray(raw?.output) ? raw.output : []
  const tool = Array.isArray(raw?.tools) ? raw.tools.find((item: any) => item?.type === 'image_generation') : null
  const images: GeneratedImage[] = []
  for (const item of output) {
    if (item?.type !== 'image_generation_call') continue
    const result = item.result || item.image || item.b64_json
    const b64 = typeof result === 'string' ? result : result?.b64_json || result?.image || result?.data
    if (!b64 || typeof b64 !== 'string') continue
    const format = item.output_format || item.actual_params?.output_format || tool?.output_format || 'png'
    const src = b64.startsWith('data:') || b64.startsWith('http') ? b64 : `data:${mimeForFormat(format)};base64,${b64}`
    images.push({
      src,
      revisedPrompt: item.revised_prompt,
      actualParams: { ...pickActualParams(tool), ...pickActualParams(item), ...(item.actual_params || {}) }
    })
  }
  return images
}

function mergeRawPayloads(raws: unknown[], apiMode: ImageApiMode): unknown {
  if (raws.length === 1) return raws[0]
  if (apiMode === 'responses') {
    const first = (raws[0] as any) || {}
    return {
      ...first,
      output: raws.flatMap((raw: any) => (Array.isArray(raw?.output) ? raw.output : [])),
      playground_parallel_raw: raws
    }
  }
  const first = (raws[0] as any) || {}
  return {
    ...first,
    data: raws.flatMap((raw: any) => (Array.isArray(raw?.data) ? raw.data : [])),
    playground_parallel_raw: raws
  }
}

async function generateImageSingle(request: ImageGenerationRequest): Promise<ImageGenerationResult> {
  const body = imageTaskPayload(request)

  const { data: raw } = await apiClient.post<unknown>('/model-playground/images', body, {
    timeout: Math.max(30, request.timeout || 300) * 1000 + 15_000
  })
  const images = request.apiMode === 'responses' ? parseResponsesAPI(raw) : parseImagesAPI(raw)
  return { images, actualParams: images[0]?.actualParams, raw }
}

export async function createImageTask(request: ImageGenerationRequest): Promise<BackendImageTask> {
  const { data } = await apiClient.post<BackendImageTask>('/model-playground/images/tasks', imageTaskPayload(request), {
    timeout: 30_000
  })
  return data
}

export async function listImageTasks(params?: { status?: string; search?: string; limit?: number; offset?: number }): Promise<BackendImageTask[]> {
  const { data } = await apiClient.get<{ items: BackendImageTask[] }>('/model-playground/images/tasks', {
    params: {
      status: params?.status || undefined,
      search: params?.search || undefined,
      limit: params?.limit || 50,
      offset: params?.offset || 0
    }
  })
  return data.items || []
}

export async function getImageTask(id: string): Promise<BackendImageTask> {
  const { data } = await apiClient.get<BackendImageTask>(`/model-playground/images/tasks/${encodeURIComponent(id)}`)
  return data
}

export async function deleteImageTask(id: string): Promise<void> {
  await apiClient.delete(`/model-playground/images/tasks/${encodeURIComponent(id)}`)
}

export async function generateImage(request: ImageGenerationRequest): Promise<ImageGenerationResult> {
  const count = Math.max(1, Math.min(10, Number(request.n) || 1))
  const shouldSplit = count > 1 && (request.apiMode === 'responses' || request.codexCli)
  if (!shouldSplit) return generateImageSingle({ ...request, n: count })

  const results = await Promise.allSettled(
    Array.from({ length: count }, () => generateImageSingle({ ...request, n: 1, quality: request.codexCli ? 'auto' : request.quality }))
  )
  const fulfilled = results.filter((item): item is PromiseFulfilledResult<ImageGenerationResult> => item.status === 'fulfilled')
  if (!fulfilled.length) {
    const rejected = results.find((item): item is PromiseRejectedResult => item.status === 'rejected')
    throw rejected?.reason || new Error('所有并发请求均失败')
  }
  const images = fulfilled.flatMap((item) => item.value.images)
  return {
    images,
    actualParams: { ...(fulfilled[0].value.actualParams || {}), n: images.length },
    raw: mergeRawPayloads(fulfilled.map((item) => item.value.raw), request.apiMode)
  }
}
