<template>
  <AppLayout>
    <section class="gip-shell">
      <header class="gip-header">
        <div class="gip-header-inner">
          <div class="flex items-start gap-2">
            <h1 class="text-lg font-bold tracking-tight text-gray-800 transition-colors dark:text-gray-100">
              GPT Image Playground
            </h1>
            <span class="mt-0.5 rounded border border-blue-500/30 bg-blue-500 px-1.5 py-0.5 text-[10px] font-bold leading-none text-white">
              sub2api
            </span>
          </div>
          <div class="flex items-center gap-1">
            <button type="button" class="gip-icon-btn" title="操作指南" @click="showHelp = true">
              <svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><path d="M12 17h.01"/></svg>
            </button>
            <button type="button" class="gip-icon-btn" title="设置" @click="showSettings = true">
              <Icon name="cog" size="md" />
            </button>
          </div>
        </div>
      </header>

      <main class="mx-auto max-w-7xl px-4 pb-48">
        <div class="mb-4 mt-6 flex gap-3">
          <div class="z-20 flex flex-shrink-0 gap-2">
            <button type="button" class="gip-filter-star" :class="filterFavorite ? 'gip-filter-star-on' : ''" :title="filterFavorite ? '取消只看收藏' : '只看收藏'" @click="filterFavorite = !filterFavorite">
              <svg class="h-5 w-5" :fill="filterFavorite ? 'currentColor' : 'none'" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z"/></svg>
            </button>
            <div class="relative w-28"><Select v-model="filterStatus" :options="statusOptions" /></div>
          </div>
          <div class="relative z-10 flex-1">
            <svg class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400 dark:text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>
            <input v-model.trim="searchQuery" type="text" placeholder="搜索提示词、参数..." class="gip-search-input" />
          </div>
        </div>

        <div v-if="!filteredTasks.length" class="py-20 text-center text-gray-400 dark:text-gray-500">
          <svg class="mx-auto mb-4 h-16 w-16 text-gray-200 dark:text-gray-700" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>
          <p class="text-sm">{{ searchQuery || filterFavorite ? '没有找到匹配的记录' : '输入提示词开始生成图片' }}</p>
        </div>

        <div v-else class="grid grid-cols-1 gap-4 pb-10 sm:grid-cols-2 lg:grid-cols-3">
          <div v-for="task in filteredTasks" :key="task.id" class="task-card-wrapper" :data-task-id="task.id">
            <article class="gip-task-card group" :class="task.status === 'running' || task.status === 'queued' ? 'border-blue-400 generating' : task.isFavorite ? 'ring-1 ring-yellow-400/40' : ''" @click="openTaskDetail(task.id)">
              <div class="flex h-40">
                <div class="relative flex h-full w-40 min-w-40 flex-shrink-0 items-center justify-center overflow-hidden bg-gray-100 dark:bg-black/20">
                  <div v-if="task.status === 'running' || task.status === 'queued'" class="flex flex-col items-center gap-2">
                    <svg class="h-8 w-8 animate-spin text-blue-400" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" /><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.37 0 0 5.37 0 12h4z" /></svg>
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ task.status === 'queued' ? '排队中...' : '生成中...' }}</span>
                  </div>
                  <div v-else-if="task.status === 'error'" class="flex flex-col items-center gap-1 px-3 text-center">
                    <svg class="h-7 w-7 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                    <span class="line-clamp-2 text-xs leading-tight text-red-400">{{ task.error || '失败' }}</span>
                  </div>
                  <img v-else-if="task.outputImages[0]" :src="imageSrc(task.outputImages[0])" class="h-full w-full object-cover" alt="output" />
                  <div v-else class="text-xs text-gray-400">No image</div>
                  <span v-if="task.outputImageCount > 1" class="absolute bottom-1 right-1 rounded bg-black/60 px-1.5 py-0.5 text-xs font-semibold text-white">{{ task.outputImageCount }}</span>
                  <div class="absolute left-1.5 top-1.5 flex items-center gap-1">
                    <span class="flex items-center gap-1 rounded bg-black/50 px-1.5 py-0.5 font-mono text-[10px] text-white backdrop-blur-sm sm:text-xs">
                      <svg class="h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                      {{ task.status === 'queued' ? '00:00' : taskElapsed(task) }}
                    </span>
                  </div>
                </div>
                <div class="flex min-w-0 flex-1 flex-col p-3">
                  <div class="min-h-0 flex-1">
                    <p class="line-clamp-3 text-sm leading-relaxed text-gray-700 dark:text-gray-300">{{ task.prompt || '(无提示词)' }}</p>
                  </div>
                  <div class="mt-auto flex flex-col gap-1.5">
                    <div class="mask-edge-r hide-scrollbar flex min-w-0 gap-1.5 overflow-x-auto whitespace-nowrap pr-2">
                      <span class="gip-param">{{ task.params.quality }}</span>
                      <span class="gip-param">{{ task.params.size }}</span>
                      <span class="gip-param">{{ task.params.output_format }}</span>
                      <span class="gip-param">n {{ task.outputImageCount || task.outputImages.length || task.params.n }}</span>
                      <span v-if="task.inputImageCount" class="gip-param">{{ task.inputImageCount }} 图</span>
                      <span class="gip-param">{{ task.apiMode }}</span>
                    </div>
                    <div class="flex flex-shrink-0 justify-end gap-1" @click.stop>
                      <button type="button" class="gip-icon-action" :class="task.isFavorite ? 'text-yellow-400 hover:bg-yellow-50 dark:hover:bg-yellow-500/10' : 'text-gray-400 hover:bg-yellow-50 hover:text-yellow-400 dark:hover:bg-yellow-500/10'" :title="task.isFavorite ? '取消收藏' : '收藏记录'" @click="toggleFavorite(task)">
                        <svg class="h-4 w-4" :fill="task.isFavorite ? 'currentColor' : 'none'" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z" /></svg>
                      </button>
                      <button type="button" class="gip-icon-action text-gray-400 hover:bg-blue-50 hover:text-blue-500 dark:hover:bg-blue-950/30" title="复用配置" @click="reuseConfig(task)">
                        <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 10h10a8 8 0 018 8v2M3 10l6 6m-6-6l6-6" /></svg>
                      </button>
                      <button type="button" class="gip-icon-action text-gray-400 hover:bg-green-50 hover:text-green-500 disabled:opacity-30 dark:hover:bg-green-950/30" :disabled="!task.outputImages.length" title="编辑输出" @click="editOutputs(task)">
                        <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" /></svg>
                      </button>
                      <button type="button" class="gip-icon-action text-gray-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-950/30" title="删除记录" @click="askDeleteTask(task)">
                        <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </article>
          </div>
        </div>
      </main>

      <section class="gip-input-bar" data-input-bar>
        <div class="mx-auto max-w-3xl px-3 pb-3 sm:px-4 sm:pb-4">
          <div class="gip-composer" :class="isDragging ? 'ring-2 ring-blue-400 ring-offset-2 ring-offset-gray-50 dark:ring-offset-gray-950' : ''" @dragenter.prevent="isDragging = true" @dragover.prevent="isDragging = true" @dragleave.prevent="isDragging = false" @drop.prevent="onDropImages">
            <div v-if="inputImages.length" class="grid grid-cols-[repeat(auto-fill,52px)] justify-between gap-x-2 gap-y-3 p-3 pb-0 sm:p-4 sm:pb-1">
              <div v-for="(img, index) in inputImages" :key="img.id" class="group relative inline-block">
                <div class="h-[52px] w-[52px] overflow-hidden rounded-xl border border-gray-200 shadow-sm dark:border-white/[0.08]">
                  <img :src="img.dataUrl" class="h-full w-full object-cover transition-opacity hover:opacity-90" alt="input" @click="openLightbox(img.id, inputImages.map((item) => item.id))" />
                </div>
                <button type="button" class="absolute -right-2 -top-2 flex h-[22px] w-[22px] items-center justify-center rounded-full bg-red-500 text-white opacity-0 shadow-md transition-opacity hover:bg-red-600 group-hover:opacity-100" @click="removeInputImage(index)">
                  <svg class="h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M6 18L18 6M6 6l12 12" /></svg>
                </button>
              </div>
              <button type="button" class="flex h-[52px] w-[52px] flex-col items-center justify-center gap-0.5 rounded-xl border border-dashed border-gray-300 text-gray-400 transition-all hover:border-red-300 hover:bg-red-50/50 hover:text-red-500 dark:border-white/[0.08] dark:text-gray-500 dark:hover:bg-red-950/30" title="清空全部参考图" @click="clearInputImages">
                <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                <span class="text-[9px] leading-none">清空</span>
              </button>
            </div>
            <textarea v-model="prompt" rows="2" class="gip-prompt" placeholder="描述你想生成的画面... 支持 Ctrl/⌘ + V 粘贴图片" @keydown.ctrl.enter.prevent="submitImageTask" @keydown.meta.enter.prevent="submitImageTask"></textarea>
            <div class="mt-3 px-3 pb-3 sm:px-4 sm:pb-4">
              <div class="hidden items-end justify-between gap-3 sm:flex">
                <div class="grid flex-1 grid-cols-6 gap-2 text-xs">
                  <label class="gip-param-field"><span>尺寸</span><button type="button" class="gip-size-button" @click="openSizePicker">{{ normalizeImageSize(params.size) || 'auto' }}</button></label>
                  <label class="gip-param-field"><span>质量</span><Select v-model="params.quality" :options="imageQualityOptions" :disabled="settings.codexCli" class="gip-param-select" /></label>
                  <label class="gip-param-field"><span>格式</span><Select v-model="params.output_format" :options="imageOutputFormatOptions" class="gip-param-select" /></label>
                  <label class="gip-param-field"><span>压缩率</span><input v-model.number="params.output_compression" :disabled="params.output_format === 'png'" type="number" min="0" max="100" placeholder="0-100" class="gip-param-input" @blur="normalizeCompression" /></label>
                  <label class="gip-param-field"><span>审核</span><Select v-model="params.moderation" :options="imageModerationOptions" :disabled="settings.apiMode === 'responses'" class="gip-param-select" /></label>
                  <label class="gip-param-field"><span>数量</span><input v-model.number="params.n" type="number" min="1" max="4" class="gip-param-input" @blur="normalizeCount" /></label>
                </div>
                <div class="mb-0.5 flex flex-shrink-0 gap-2">
                  <label class="gip-round-action cursor-pointer" title="添加参考图"><svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.172 7l-6.586 6.586a2 2 0 102.828 2.828l6.414-6.586a4 4 0 00-5.656-5.656l-6.415 6.585a6 6 0 108.486 8.486L20.5 13"/></svg><input type="file" accept="image/*" multiple class="sr-only" @change="onInputImagesSelected" /></label>
                  <button type="button" class="gip-round-submit" :disabled="!canSubmit" title="生成 (Ctrl+Enter)" @click="submitImageTask"><svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg></button>
                </div>
              </div>
              <div class="flex flex-col gap-2 sm:hidden">
                <div class="grid flex-1 grid-cols-2 gap-2 text-xs">
                  <label class="gip-param-field"><span>尺寸</span><button type="button" class="gip-size-button" @click="openSizePicker">{{ normalizeImageSize(params.size) || 'auto' }}</button></label>
                  <label class="gip-param-field"><span>质量</span><Select v-model="params.quality" :options="imageQualityOptions" :disabled="settings.codexCli" class="gip-param-select" /></label>
                  <label class="gip-param-field"><span>格式</span><Select v-model="params.output_format" :options="imageOutputFormatOptions" class="gip-param-select" /></label>
                  <label class="gip-param-field"><span>压缩率</span><input v-model.number="params.output_compression" :disabled="params.output_format === 'png'" type="number" min="0" max="100" placeholder="0-100" class="gip-param-input" @blur="normalizeCompression" /></label>
                  <label class="gip-param-field"><span>审核</span><Select v-model="params.moderation" :options="imageModerationOptions" :disabled="settings.apiMode === 'responses'" class="gip-param-select" /></label>
                  <label class="gip-param-field"><span>数量</span><input v-model.number="params.n" type="number" min="1" max="4" class="gip-param-input" @blur="normalizeCount" /></label>
                </div>
                <div class="flex items-center gap-2">
                  <label class="gip-round-action flex-shrink-0 cursor-pointer" title="添加参考图"><svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.172 7l-6.586 6.586a2 2 0 102.828 2.828l6.414-6.586a4 4 0 00-5.656-5.656l-6.415 6.585a6 6 0 108.486 8.486L20.5 13"/></svg><input type="file" accept="image/*" multiple class="sr-only" @change="onInputImagesSelected" /></label>
                  <button type="button" class="gip-mobile-submit" :disabled="!canSubmit" @click="submitImageTask"><svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>{{ isSubmitting ? '生成中' : '生成图像' }}</button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <div v-if="showSettings" class="gip-overlay" @click.self="showSettings = false">
        <section class="gip-modal max-w-2xl animate-slide-down-in">
          <div class="flex items-start justify-between border-b border-gray-200 p-5 dark:border-white/[0.08]"><div><h2 class="text-lg font-bold">设置</h2><p class="mt-1 text-sm text-gray-500">请求通过 sub2api 后端代理，进入平台网关计费链路。</p></div><button class="gip-tool-btn" @click="showSettings = false">×</button></div>
          <div class="grid gap-4 p-5 sm:grid-cols-2">
            <label class="gip-field sm:col-span-2"><span>API Key</span><Select v-model="selectedKeyId" :options="apiKeyOptions" :searchable="true" /></label>
            <label v-if="selectedKeyId === manualKeyValue" class="gip-field sm:col-span-2"><span>手动 API Key</span><input v-model.trim="manualApiKey" type="password" class="input" autocomplete="off" placeholder="sk-..." /></label>
            <label class="gip-field"><span>API 模式</span><Select v-model="settings.apiMode" :options="apiModeOptions" /></label>
            <label class="gip-field"><span>模型</span><input v-model.trim="settings.model" class="input font-mono text-sm" /></label>
            <label class="gip-field"><span>超时 / 秒</span><input v-model.number="settings.timeout" type="number" min="30" max="900" class="input" /></label>
            <label class="gip-field"><span>审核</span><Select v-model="params.moderation" :options="imageModerationOptions" /></label>
            <label v-if="params.output_format !== 'png'" class="gip-field"><span>压缩质量</span><input v-model.number="params.output_compression" type="number" min="0" max="100" class="input" /></label>
            <label class="flex items-start gap-3 rounded-2xl border border-gray-200 bg-gray-50 px-4 py-3 text-sm dark:border-white/[0.08] dark:bg-gray-950 sm:col-span-2"><input v-model="settings.codexCli" type="checkbox" class="mt-1 h-4 w-4 rounded border-gray-300 text-blue-600" /><span><span class="block font-semibold">Codex CLI 兼容模式</span><span class="block text-xs text-gray-500">保持原始提示词、不改写；质量参数固定为 auto，多图会拆分并发请求。</span></span></label>
            <div class="flex gap-2 sm:col-span-2"><button class="btn btn-secondary" @click="exportData">导出数据</button><label class="btn btn-secondary cursor-pointer">导入数据<input type="file" accept="application/json" class="sr-only" @change="onImportFile" /></label><button class="btn btn-secondary text-red-500" @click="askClearAll">清空全部</button></div>
          </div>
        </section>
      </div>

      <div v-if="showHelp" class="gip-overlay" @click.self="showHelp = false">
        <section class="gip-modal max-w-lg animate-modal-in p-6"><div class="mb-4 flex items-start justify-between"><h2 class="text-lg font-bold">操作指南</h2><button class="gip-tool-btn" @click="showHelp = false">×</button></div><ul class="space-y-2 text-sm leading-6 text-gray-600 dark:text-gray-300"><li>• 底部输入提示词，Ctrl/⌘ + Enter 生成。</li><li>• 支持拖拽上传、文件选择、Ctrl/⌘ + V 粘贴参考图。</li><li>• 上传参考图后自动走图片编辑接口。</li><li>• 点击记录查看详情，支持复用配置、输出图继续编辑。</li><li>• 任务记录保存在服务端，图片缓存保留在浏览器以加速预览。</li></ul></section>
      </div>

      <div v-if="detailTask" class="gip-overlay" @click.self="detailTaskId = null">
        <section class="gip-modal max-w-6xl animate-modal-in">
          <div class="flex items-start justify-between border-b border-gray-200 p-5 dark:border-white/[0.08]"><div class="min-w-0"><h2 class="text-lg font-bold">任务详情</h2><p class="mt-1 truncate text-sm text-gray-500">{{ detailTask.prompt }}</p><p v-if="detailLoading" class="mt-1 text-xs text-blue-500">正在加载完整图片...</p></div><button class="gip-tool-btn" @click="detailTaskId = null">×</button></div>
          <div class="grid gap-5 p-5 lg:grid-cols-[minmax(0,1fr)_340px]">
            <div class="space-y-5">
              <section>
                <div class="mb-2 flex items-center justify-between">
                  <h3 class="text-sm font-semibold text-gray-800 dark:text-gray-100">参考图</h3>
                  <span class="text-xs text-gray-400">{{ detailTask.inputImageIds.length }} 张</span>
                </div>
                <div v-if="detailTask.inputImageIds.length" class="grid grid-cols-3 gap-3 sm:grid-cols-4 lg:grid-cols-5">
                  <button v-for="imageId in detailTask.inputImageIds" :key="imageId" class="group relative overflow-hidden rounded-2xl border border-gray-200 bg-gray-100 dark:border-white/[0.08] dark:bg-gray-950" @click="openLightbox(imageId, detailTask.inputImageIds)">
                    <img :src="imageSrc(imageId)" class="aspect-square w-full object-cover transition-transform group-hover:scale-105" alt="reference"/>
                    <span class="absolute left-2 top-2 rounded bg-black/55 px-1.5 py-0.5 text-[10px] font-medium text-white backdrop-blur">REF</span>
                  </button>
                </div>
                <div v-else class="rounded-2xl border border-dashed border-gray-200 bg-gray-50 px-4 py-8 text-center text-sm text-gray-400 dark:border-white/[0.08] dark:bg-gray-950/60">
                  这个任务没有参考图
                </div>
              </section>

              <section>
                <div class="mb-2 flex items-center justify-between">
                  <h3 class="text-sm font-semibold text-gray-800 dark:text-gray-100">生成结果</h3>
                  <span class="text-xs text-gray-400">{{ detailTask.outputImages.length }} 张</span>
                </div>
                <div v-if="detailTask.outputImages.length" class="grid gap-3 sm:grid-cols-2">
                  <button v-for="imageId in detailTask.outputImages" :key="imageId" class="overflow-hidden rounded-2xl border border-gray-200 bg-gray-100 dark:border-white/[0.08] dark:bg-gray-950" @click="openLightbox(imageId, detailTask.outputImages)">
                    <img :src="imageSrc(imageId)" class="aspect-square w-full object-cover" alt="output"/>
                  </button>
                </div>
                <div v-else class="rounded-2xl border border-dashed border-gray-200 bg-gray-50 px-4 py-10 text-center text-sm text-gray-400 dark:border-white/[0.08] dark:bg-gray-950/60">
                  暂无生成结果
                </div>
              </section>
            </div>
            <aside class="space-y-4 text-sm"><div><h3 class="mb-2 font-semibold">Prompt</h3><p class="whitespace-pre-wrap rounded-2xl bg-gray-50 p-3 dark:bg-gray-950">{{ detailTask.prompt }}</p></div><div><h3 class="mb-2 font-semibold">Params</h3><pre class="rounded-2xl bg-gray-950 p-3 text-xs text-gray-100">{{ prettyJSON(detailTask.params) }}</pre></div><div v-if="detailTask.raw"><h3 class="mb-2 font-semibold">Raw</h3><pre class="max-h-72 overflow-auto rounded-2xl bg-gray-950 p-3 text-xs text-gray-100">{{ prettyJSON(detailTask.raw) }}</pre></div><div class="grid grid-cols-2 gap-2"><button class="btn btn-secondary justify-center" @click="reuseConfig(detailTask)">复用配置</button><button class="btn btn-secondary justify-center" @click="editOutputs(detailTask)">编辑输出</button></div></aside>
          </div>
        </section>
      </div>

      <div v-if="showSizePicker" class="fixed inset-0 z-[70] flex items-center justify-center p-4" @click="showSizePicker = false">
        <div class="absolute inset-0 animate-overlay-in bg-black/30 backdrop-blur-sm"></div>
        <section class="relative z-10 w-full max-w-md animate-modal-in rounded-3xl border border-white/50 bg-white/95 p-5 shadow-2xl ring-1 ring-black/5 dark:border-white/[0.08] dark:bg-gray-900/95 dark:ring-white/10" @click.stop>
          <div class="mb-5 flex items-start justify-between gap-4">
            <div>
              <h3 class="text-base font-semibold text-gray-800 dark:text-gray-100">设置图像尺寸</h3>
              <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">当前：{{ params.size || 'auto' }}</p>
            </div>
            <button type="button" class="rounded-full p-1 text-gray-400 transition hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-white/[0.06] dark:hover:text-gray-200" @click="showSizePicker = false">
              <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
          </div>

          <div class="space-y-6">
            <div class="flex rounded-xl bg-gray-100/80 p-1 dark:bg-white/[0.04]">
              <button type="button" class="gip-size-tab" :class="sizePicker.mode === 'auto' ? 'gip-size-tab-on' : ''" @click="sizePicker.mode = 'auto'">自动</button>
              <button type="button" class="gip-size-tab" :class="sizePicker.mode === 'ratio' ? 'gip-size-tab-on' : ''" @click="sizePicker.mode = 'ratio'">按比例</button>
              <button type="button" class="gip-size-tab" :class="sizePicker.mode === 'resolution' ? 'gip-size-tab-on' : ''" @click="sizePicker.mode = 'resolution'">自定义宽高</button>
            </div>

            <div class="min-h-[220px]">
              <div v-if="sizePicker.mode === 'auto'" class="flex h-full animate-fade-in items-center justify-center pb-4 pt-8 text-center">
                <div>
                  <div class="mb-3 inline-flex h-12 w-12 items-center justify-center rounded-full bg-blue-50 text-blue-500 dark:bg-blue-500/10">
                    <svg class="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" /></svg>
                  </div>
                  <h4 class="text-sm font-medium text-gray-800 dark:text-gray-200">自动尺寸</h4>
                  <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">不向模型传递具体的分辨率参数<br/>由模型自己决定生成尺寸</p>
                </div>
              </div>

              <div v-else-if="sizePicker.mode === 'ratio'" class="animate-fade-in space-y-5">
                <section>
                  <div class="mb-2 text-xs font-medium text-gray-400 dark:text-gray-500">基准分辨率</div>
                  <div class="grid grid-cols-3 gap-2">
                    <button v-for="tier in sizeTiers" :key="tier" type="button" class="gip-size-choice" :class="sizePicker.tier === tier ? 'gip-size-choice-on' : ''" @click="sizePicker.tier = tier">{{ tier }}</button>
                  </div>
                </section>
                <section>
                  <div class="mb-2 text-xs font-medium text-gray-400 dark:text-gray-500">图像比例</div>
                  <div class="grid grid-cols-4 gap-2">
                    <button v-for="ratio in sizeRatios" :key="ratio.value" type="button" class="gip-size-choice" :class="sizePicker.ratio === ratio.value ? 'gip-size-choice-on' : ''" @click="sizePicker.ratio = ratio.value">{{ ratio.label }}</button>
                    <button type="button" class="gip-size-choice col-span-4" :class="sizePicker.ratio === 'custom' ? 'gip-size-choice-on' : ''" @click="sizePicker.ratio = 'custom'">自定义比例</button>
                  </div>
                </section>
                <label v-if="sizePicker.ratio === 'custom'" class="block animate-fade-in">
                  <span class="mb-2 block text-xs font-medium text-gray-400 dark:text-gray-500">输入自定义比例</span>
                  <input v-model="sizePicker.customRatio" placeholder="例如 5:4 / 2.39:1" class="gip-size-input" :class="!customRatioValid ? 'border-red-300 focus:border-red-400 dark:border-red-500/40' : ''" />
                </label>
              </div>

              <div v-else class="animate-fade-in space-y-5">
                <section>
                  <div class="mb-4 text-xs font-medium text-gray-400 dark:text-gray-500">输入具体像素值</div>
                  <div class="flex items-center gap-4">
                    <label class="flex-1"><span class="mb-1.5 block text-xs text-gray-500 dark:text-gray-400">宽度 (Width)</span><input v-model="sizePicker.customW" type="number" placeholder="例如 1024" class="gip-size-input" /></label>
                    <div class="mt-5 text-gray-300 dark:text-gray-600"><svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" /></svg></div>
                    <label class="flex-1"><span class="mb-1.5 block text-xs text-gray-500 dark:text-gray-400">高度 (Height)</span><input v-model="sizePicker.customH" type="number" placeholder="例如 1024" class="gip-size-input" /></label>
                  </div>
                </section>
                <div class="rounded-xl border border-blue-100 bg-blue-50/50 p-3 text-xs text-blue-600 dark:border-blue-500/20 dark:bg-blue-500/10 dark:text-blue-400">{{ sizeLimitText }}</div>
              </div>
            </div>

            <div class="rounded-2xl bg-gray-50 px-4 py-3 dark:bg-white/[0.03]">
              <div class="text-xs text-gray-400 dark:text-gray-500">将使用</div>
              <div class="mt-1 flex items-center gap-2">
                <span class="font-mono text-lg font-semibold text-gray-800 dark:text-gray-100">{{ sizePreview || '尺寸无效' }}</span>
                <span v-if="sizeIsClamped" class="text-yellow-500" :title="sizeLimitText">
                  <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                </span>
              </div>
            </div>
          </div>

          <div class="mt-5 flex gap-2">
            <button type="button" class="flex-1 rounded-xl bg-gray-100 px-4 py-2.5 text-sm text-gray-600 transition hover:bg-gray-200 dark:bg-white/[0.06] dark:text-gray-300 dark:hover:bg-white/[0.1]" @click="showSizePicker = false">取消</button>
            <button type="button" class="flex-1 rounded-xl bg-blue-500 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-blue-600 disabled:cursor-not-allowed disabled:opacity-50" :disabled="!sizePreview" @click="applyPickedSize">确定</button>
          </div>
        </section>
      </div>

      <div v-if="lightboxImageId" class="fixed inset-0 z-[80] flex animate-fade-in items-center justify-center bg-black/90 p-4" @click.self="lightboxImageId = null"><button class="absolute right-4 top-4 rounded-full bg-white/10 px-4 py-2 text-white" @click="lightboxImageId = null">×</button><button v-if="lightboxList.length > 1" class="absolute left-4 top-1/2 rounded-full bg-white/10 px-4 py-2 text-white" @click="stepLightbox(-1)">‹</button><img :src="imageSrc(lightboxImageId)" class="max-h-[90vh] max-w-[92vw] animate-zoom-in rounded-2xl object-contain" alt="preview"/><button v-if="lightboxList.length > 1" class="absolute right-4 top-1/2 rounded-full bg-white/10 px-4 py-2 text-white" @click="stepLightbox(1)">›</button></div>

      <div v-if="confirmDialog" class="gip-overlay" @click.self="confirmDialog = null"><section class="w-full max-w-md animate-confirm-in rounded-2xl border border-gray-200 bg-white p-6 shadow-2xl dark:border-white/[0.08] dark:bg-gray-900"><h2 class="text-lg font-bold">{{ confirmDialog.title }}</h2><p class="mt-2 whitespace-pre-wrap text-sm leading-6 text-gray-600 dark:text-gray-300">{{ confirmDialog.message }}</p><div class="mt-5 flex justify-end gap-2"><button class="btn btn-secondary" @click="confirmDialog = null">取消</button><button class="btn btn-primary" @click="runConfirm">{{ confirmDialog.confirmText || '确认' }}</button></div></section></div>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import { keysAPI } from '@/api/keys'
import { createImageTask, deleteImageTask as deleteRemoteImageTask, getImageTask, listImageTasks, type BackendImageTask, type ImageApiMode } from '@/api/modelPlayground'
import { useAppStore, useAuthStore } from '@/stores'
import type { ApiKey } from '@/types'

type TaskStatus = 'queued' | 'running' | 'done' | 'error'
interface TaskParams { size: string; quality: 'auto' | 'low' | 'medium' | 'high'; output_format: 'png' | 'jpeg' | 'webp'; output_compression: number | null; moderation: 'auto' | 'low'; n: number }
interface InputImage { id: string; dataUrl: string }
interface StoredImage extends InputImage { createdAt: number; source?: 'upload' | 'generated' }
interface AppSettings { apiMode: ImageApiMode; model: string; timeout: number; codexCli: boolean }
interface TaskRecord { id: string; prompt: string; model: string; apiMode: ImageApiMode; params: TaskParams; actualParams?: Record<string, unknown>; inputImageIds: string[]; inputImageCount: number; outputImages: string[]; outputImageCount: number; revisedPromptByImage?: Record<string, string>; status: TaskStatus; error: string | null; createdAt: number; finishedAt: number | null; elapsed: number | null; isFavorite?: boolean; raw?: unknown }
interface ConfirmDialog { title: string; message: string; confirmText?: string; action: () => void | Promise<void> }
interface PersistedState { selectedKeyId?: number | string | null; settings?: Partial<AppSettings>; params?: Partial<TaskParams> }
type SizeTier = '1K' | '2K' | '4K'
type SizeMode = 'auto' | 'ratio' | 'resolution'

const appStore = useAppStore()
const authStore = useAuthStore()
const manualKeyValue = '__manual__'
const DEFAULT_IMAGES_MODEL = 'gpt-image-2'
const DEFAULT_RESPONSES_MODEL = 'gpt-5.5'
const API_MAX_IMAGES = 16

const apiKeys = ref<ApiKey[]>([])
const selectedKeyId = ref<string | number | null>(manualKeyValue)
const manualApiKey = ref('')
const settings = ref<AppSettings>({ apiMode: 'images', model: DEFAULT_IMAGES_MODEL, timeout: 300, codexCli: false })
const params = ref<TaskParams>({ size: 'auto', quality: 'auto', output_format: 'png', output_compression: null, moderation: 'auto', n: 1 })
const prompt = ref('')
const inputImages = ref<InputImage[]>([])
const tasks = ref<TaskRecord[]>([])
const imageCache = ref<Record<string, string>>({})
const searchQuery = ref('')
const filterStatus = ref<string | number | boolean | null>('all')
const filterFavorite = ref(false)
const isDragging = ref(false)
const showSettings = ref(false)
const showHelp = ref(false)
const showSizePicker = ref(false)
const sizePicker = ref({ mode: 'auto' as SizeMode, tier: '1K' as SizeTier, ratio: '1:1', customRatio: '16:9', customW: '1024', customH: '1024' })
const nowTick = ref(Date.now())
const detailTaskId = ref<string | null>(null)
const detailLoading = ref(false)
const lightboxImageId = ref<string | null>(null)
const lightboxList = ref<string[]>([])
const confirmDialog = ref<ConfirmDialog | null>(null)
let dbPromise: Promise<IDBDatabase> | null = null
let persistTimer: number | undefined
let tickerTimer: number | undefined
const pollingTaskIds = new Set<string>()
const imageIdByDataUrl = new Map<string, string>()

const storageKey = computed(() => `sub2api:gpt-image-playground:${authStore.user?.id || 'anonymous'}`)
const dbName = computed(() => `sub2api-gpt-image-playground-${authStore.user?.id || 'anonymous'}`)
const detailTask = computed(() => tasks.value.find((task) => task.id === detailTaskId.value) || null)
const isSubmitting = computed(() => tasks.value.some((task) => task.status === 'running' || task.status === 'queued'))
const canSubmit = computed(() => Boolean(prompt.value.trim()) && validateConnection(false))
const apiKeyOptions = computed(() => [{ value: manualKeyValue, label: '手动粘贴 API Key' }, ...apiKeys.value.map((key) => ({ value: key.id, label: `${key.name} · ${key.group?.name || '默认分组'} · ${key.status}`, disabled: key.status !== 'active' }))])
const apiModeOptions = [{ value: 'images', label: 'Images API' }, { value: 'responses', label: 'Responses API' }]
const statusOptions = [{ value: 'all', label: '全部状态' }, { value: 'done', label: '已完成' }, { value: 'queued', label: '排队中' }, { value: 'running', label: '生成中' }, { value: 'error', label: '失败' }]
const imageQualityOptions = [{ value: 'auto', label: 'auto' }, { value: 'low', label: 'low' }, { value: 'medium', label: 'medium' }, { value: 'high', label: 'high' }]
const imageOutputFormatOptions = [{ value: 'png', label: 'PNG' }, { value: 'jpeg', label: 'JPEG' }, { value: 'webp', label: 'WebP' }]
const imageModerationOptions = [{ value: 'auto', label: 'auto' }, { value: 'low', label: 'low' }]
const sizeTiers: SizeTier[] = ['1K', '2K', '4K']
const sizeRatios = [{ label: '1:1', value: '1:1' }, { label: '3:2', value: '3:2' }, { label: '2:3', value: '2:3' }, { label: '16:9', value: '16:9' }, { label: '9:16', value: '9:16' }, { label: '4:3', value: '4:3' }, { label: '3:4', value: '3:4' }, { label: '21:9', value: '21:9' }]
const sizeLimitText = '由于模型限制，最终输出会自动规整到合法尺寸：宽高均为 16 的倍数，最大边长 3840px，宽高比不超过 3:1，总像素限制为 655360-8294400。'
const customRatioValid = computed(() => sizePicker.value.ratio !== 'custom' || Boolean(parseRatio(sizePicker.value.customRatio)))
const sizePreview = computed(() => {
  const picker = sizePicker.value
  if (picker.mode === 'auto') return 'auto'
  if (picker.mode === 'ratio') return calculateImageSize(picker.tier, picker.ratio === 'custom' ? picker.customRatio : picker.ratio) || ''
  const w = parseInt(picker.customW, 10)
  const h = parseInt(picker.customH, 10)
  return Number.isFinite(w) && Number.isFinite(h) && w > 0 && h > 0 ? normalizeImageSize(`${w}x${h}`) : ''
})
const sizeIsClamped = computed(() => {
  const picker = sizePicker.value
  if (!sizePreview.value || sizePreview.value === 'auto') return false
  if (picker.mode === 'ratio' && picker.ratio === 'custom') {
    const parsed = parseRatio(picker.customRatio)
    return !!parsed && Math.max(parsed.width, parsed.height) / Math.min(parsed.width, parsed.height) > 3
  }
  if (picker.mode === 'resolution') return `${parseInt(picker.customW, 10)}x${parseInt(picker.customH, 10)}` !== sizePreview.value
  return false
})

const filteredTasks = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return [...tasks.value].sort((a, b) => b.createdAt - a.createdAt).filter((task) => {
    if (filterFavorite.value && !task.isFavorite) return false
    if (filterStatus.value !== 'all' && task.status !== filterStatus.value) return false
    if (!q) return true
    return task.prompt.toLowerCase().includes(q) || JSON.stringify(task.params).toLowerCase().includes(q) || task.model.toLowerCase().includes(q)
  })
})

function activeKeyPayload(): { apiKeyId?: number | string | null; apiKey?: string } { return selectedKeyId.value === manualKeyValue ? { apiKey: manualApiKey.value.trim() } : { apiKeyId: selectedKeyId.value } }
function validateConnection(show = true): boolean { const ok = selectedKeyId.value === manualKeyValue ? Boolean(manualApiKey.value.trim()) : Boolean(selectedKeyId.value); if (!ok && show) appStore.showError('请先选择或粘贴 API Key'); return ok }
function imageSrc(id: string): string { return imageCache.value[id] || '' }
function prettyJSON(value: unknown): string { try { return JSON.stringify(value, null, 2) } catch { return String(value) } }
function taskElapsed(task: TaskRecord): string { const ms = task.status === 'running' ? nowTick.value - task.createdAt : task.elapsed || 0; const seconds = Math.max(0, Math.floor(ms / 1000)); return `${String(Math.floor(seconds / 60)).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}` }
function normalizeCompression() { const value = Number(params.value.output_compression); params.value.output_compression = params.value.output_format === 'png' || !Number.isFinite(value) ? null : Math.max(0, Math.min(100, value)) }
function normalizeCount() { const value = Number(params.value.n); params.value.n = Number.isFinite(value) ? Math.max(1, Math.min(4, Math.round(value))) : 1 }
function roundToMultiple(value: number, multiple: number) { return Math.max(multiple, Math.round(value / multiple) * multiple) }
function floorToMultiple(value: number, multiple: number) { return Math.max(multiple, Math.floor(value / multiple) * multiple) }
function ceilToMultiple(value: number, multiple: number) { return Math.max(multiple, Math.ceil(value / multiple) * multiple) }
function normalizeDimensions(width: number, height: number) {
  let w = roundToMultiple(width, 16)
  let h = roundToMultiple(height, 16)
  const scaleToFit = (scale: number) => { w = floorToMultiple(w * scale, 16); h = floorToMultiple(h * scale, 16) }
  const scaleToFill = (scale: number) => { w = ceilToMultiple(w * scale, 16); h = ceilToMultiple(h * scale, 16) }
  for (let i = 0; i < 4; i++) {
    const maxEdge = Math.max(w, h)
    if (maxEdge > 3840) scaleToFit(3840 / maxEdge)
    if (w / h > 3) w = floorToMultiple(h * 3, 16)
    else if (h / w > 3) h = floorToMultiple(w * 3, 16)
    const pixels = w * h
    if (pixels > 8294400) scaleToFit(Math.sqrt(8294400 / pixels))
    else if (pixels < 655360) scaleToFill(Math.sqrt(655360 / pixels))
  }
  return { width: w, height: h }
}
function normalizeImageSize(size: string) {
  const match = size.trim().match(/^\s*(\d+)\s*[xX×]\s*(\d+)\s*$/)
  if (!match) return size.trim()
  const { width, height } = normalizeDimensions(Number(match[1]), Number(match[2]))
  return `${width}x${height}`
}
function parseRatio(ratio: string) {
  const match = ratio.match(/^\s*(\d+(?:\.\d+)?)\s*[:xX×]\s*(\d+(?:\.\d+)?)\s*$/)
  if (!match) return null
  const width = Number(match[1])
  const height = Number(match[2])
  return Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0 ? { width, height } : null
}
function calculateImageSize(tier: SizeTier, ratio: string) {
  const parsed = parseRatio(ratio)
  if (!parsed) return null
  const { width: rw, height: rh } = parsed
  if (rw === rh) return normalizeImageSize(`${tier === '1K' ? 1024 : tier === '2K' ? 2048 : 3840}x${tier === '1K' ? 1024 : tier === '2K' ? 2048 : 3840}`)
  if (tier === '1K') {
    const short = 1024
    return rw > rh ? `${roundToMultiple(short * rw / rh, 16)}x${short}` : `${short}x${roundToMultiple(short * rh / rw, 16)}`
  }
  const long = tier === '2K' ? 2048 : 3840
  return normalizeImageSize(rw > rh ? `${long}x${roundToMultiple(long * rh / rw, 16)}` : `${roundToMultiple(long * rw / rh, 16)}x${long}`)
}
function parseSize(size: string) { const match = size.match(/^\s*(\d+)\s*[xX×]\s*(\d+)\s*$/); return match ? { width: match[1], height: match[2] } : null }
function findPresetForSize(size: string) {
  const normalized = normalizeImageSize(size)
  for (const tier of sizeTiers) for (const ratio of sizeRatios) if (calculateImageSize(tier, ratio.value) === normalized) return { tier, ratio: ratio.value }
  return null
}
function openSizePicker() {
  const current = params.value.size || 'auto'
  const preset = findPresetForSize(current)
  const parsed = parseSize(current)
  sizePicker.value = {
    mode: !current || current === 'auto' ? 'auto' : preset ? 'ratio' : 'resolution',
    tier: preset?.tier || '1K',
    ratio: preset?.ratio || '1:1',
    customRatio: '16:9',
    customW: parsed?.width || '1024',
    customH: parsed?.height || '1024'
  }
  showSizePicker.value = true
}
function applyPickedSize() { if (!sizePreview.value) return; params.value.size = sizePreview.value; showSizePicker.value = false }

function openDB(): Promise<IDBDatabase> {
  if (dbPromise) return dbPromise
  dbPromise = new Promise((resolve, reject) => {
    const req = indexedDB.open(dbName.value, 1)
    req.onupgradeneeded = () => { const db = req.result; if (!db.objectStoreNames.contains('tasks')) db.createObjectStore('tasks', { keyPath: 'id' }); if (!db.objectStoreNames.contains('images')) db.createObjectStore('images', { keyPath: 'id' }) }
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
  return dbPromise
}
async function dbGetAll<T>(store: string): Promise<T[]> { const db = await openDB(); return new Promise((resolve, reject) => { const tx = db.transaction(store, 'readonly'); const req = tx.objectStore(store).getAll(); req.onsuccess = () => resolve(req.result as T[]); req.onerror = () => reject(req.error) }) }
async function dbPut(store: string, value: unknown): Promise<void> { const db = await openDB(); return new Promise((resolve, reject) => { const tx = db.transaction(store, 'readwrite'); tx.objectStore(store).put(value); tx.oncomplete = () => resolve(); tx.onerror = () => reject(tx.error) }) }
async function dbClear(store: string): Promise<void> { const db = await openDB(); return new Promise((resolve, reject) => { const tx = db.transaction(store, 'readwrite'); tx.objectStore(store).clear(); tx.oncomplete = () => resolve(); tx.onerror = () => reject(tx.error) }) }
async function hashData(value: string): Promise<string> { try { const bytes = new TextEncoder().encode(value); const digest = await crypto.subtle.digest('SHA-256', bytes); return Array.from(new Uint8Array(digest)).map((b) => b.toString(16).padStart(2, '0')).join('') } catch { let hash = 0; for (let i = 0; i < value.length; i++) hash = ((hash << 5) - hash + value.charCodeAt(i)) | 0; return `fallback-${Math.abs(hash)}-${value.length}` } }
async function storeImage(dataUrl: string, source: StoredImage['source'] = 'upload'): Promise<string> { const cachedId = imageIdByDataUrl.get(dataUrl); if (cachedId) return cachedId; const id = await hashData(dataUrl); imageIdByDataUrl.set(dataUrl, id); imageCache.value = { ...imageCache.value, [id]: dataUrl }; await dbPut('images', { id, dataUrl, source, createdAt: Date.now() } satisfies StoredImage); return id }
function fileToDataURL(file: File): Promise<string> { return new Promise((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result || '')); reader.onerror = () => reject(reader.error || new Error('Failed to read file')); reader.readAsDataURL(file) }) }
async function loadApiKeys() { try { const result = await keysAPI.list(1, 100, { status: 'active' }); apiKeys.value = result.items || []; const selectedExists = apiKeys.value.some((item) => item.id === selectedKeyId.value); const firstActive = apiKeys.value.find((item) => item.status === 'active'); if (!selectedExists && firstActive) selectedKeyId.value = firstActive.id } catch (error) { console.warn('Failed to load API keys for playground:', error) } }
async function loadLocalData() { const storedImages = await dbGetAll<StoredImage>('images'); const next: Record<string, string> = {}; imageIdByDataUrl.clear(); for (const image of storedImages || []) { next[image.id] = image.dataUrl; imageIdByDataUrl.set(image.dataUrl, image.id) } imageCache.value = next; await refreshBackendTasks() }

async function mapBackendTask(task: BackendImageTask): Promise<TaskRecord> {
  const inputIds: string[] = []
  for (const src of task.input_images || []) inputIds.push(await storeImage(src, 'upload'))
  const outputIds: string[] = []
  const revisedPromptByImage: Record<string, string> = {}
  for (const src of task.output_images || []) {
    const id = await storeImage(src, 'generated')
    outputIds.push(id)
    const revised = task.revised_prompt_by_image?.[src]
    if (revised) revisedPromptByImage[id] = revised
  }
  const createdAt = Date.parse(task.created_at) || Date.now()
  const finishedAt = task.finished_at ? Date.parse(task.finished_at) : null
  const inputImageCount = Number(task.input_image_count ?? inputIds.length) || 0
  const outputImageCount = Number(task.output_image_count ?? outputIds.length) || 0
  return {
    id: task.id,
    prompt: task.prompt,
    model: task.model,
    apiMode: task.api_mode,
    params: {
      size: String(task.params?.size || 'auto'),
      quality: (task.params?.quality || 'auto') as TaskParams['quality'],
      output_format: (task.params?.output_format || 'png') as TaskParams['output_format'],
      output_compression: task.params?.output_compression ?? null,
      moderation: (task.params?.moderation || 'auto') as TaskParams['moderation'],
      n: Number(task.params?.n) || 1
    },
    actualParams: task.actual_params,
    inputImageIds: inputIds,
    inputImageCount,
    outputImages: outputIds,
    outputImageCount,
    revisedPromptByImage,
    status: task.status,
    error: task.error || null,
    createdAt,
    finishedAt,
    elapsed: task.elapsed_ms ?? (finishedAt ? finishedAt - createdAt : null),
    raw: task.raw_response
  }
}

async function openTaskDetail(taskId: string) {
  detailTaskId.value = taskId
  detailLoading.value = true
  try {
    const remote = await getImageTask(taskId)
    const mapped = await mapBackendTask(remote)
    const existing = tasks.value.find((item) => item.id === taskId)
    tasks.value = [mapped, ...tasks.value.filter((item) => item.id !== taskId)]
      .map((item) => item.id === taskId ? { ...item, isFavorite: existing?.isFavorite } : item)
      .sort((a, b) => b.createdAt - a.createdAt)
  } catch (error) {
    console.warn('Failed to load image task detail:', error)
    appStore.showError((error as Error).message || '加载任务详情失败')
  } finally {
    detailLoading.value = false
  }
}

async function refreshBackendTasks() {
  try {
    const remote = await listImageTasks({ limit: 80 })
    tasks.value = await Promise.all(remote.map(mapBackendTask))
    for (const task of tasks.value) {
      if (task.status === 'queued' || task.status === 'running') pollTask(task.id)
    }
  } catch (error) {
    console.warn('Failed to load image tasks:', error)
  }
}

async function submitImageTask() {
  const value = prompt.value.trim()
  if (!value) { appStore.showError('请输入提示词'); return }
  if (!settings.value.model.trim() || !validateConnection()) return
  if (settings.value.codexCli) params.value.quality = 'auto'
  try {
    const remote = await createImageTask({ ...activeKeyPayload(), apiMode: settings.value.apiMode, model: settings.value.model.trim(), prompt: value, size: params.value.size, quality: params.value.quality, outputFormat: params.value.output_format, outputCompression: params.value.output_compression, moderation: params.value.moderation, n: Math.max(1, Math.min(4, Number(params.value.n) || 1)), timeout: settings.value.timeout, codexCli: settings.value.codexCli, inputImages: inputImages.value.map((img) => img.dataUrl) })
    const task = await mapBackendTask(remote)
    tasks.value = [task, ...tasks.value.filter((item) => item.id !== task.id)]
    prompt.value = ''
    pollTask(task.id)
    appStore.showSuccess('任务已提交，断开页面后后端会继续执行')
  } catch (error) {
    appStore.showError((error as Error).message)
  }
}
async function pollTask(taskId: string) {
  if (pollingTaskIds.has(taskId)) return
  pollingTaskIds.add(taskId)
  let attempts = 0
  try {
    while (attempts < 360) {
      attempts += 1
      try {
        const remote = await getImageTask(taskId)
        const mapped = await mapBackendTask(remote)
        tasks.value = [mapped, ...tasks.value.filter((item) => item.id !== taskId)].sort((a, b) => b.createdAt - a.createdAt)
        if (mapped.status === 'done') {
          appStore.showSuccess(`生成完成，共 ${mapped.outputImages.length} 张图片`)
          return
        }
        if (mapped.status === 'error') {
          detailTaskId.value = taskId
          appStore.showError(mapped.error || '任务失败')
          return
        }
      } catch (error) {
        console.warn('Failed to poll image task:', error)
      }
      await new Promise((resolve) => window.setTimeout(resolve, 2000))
    }
  } finally {
    pollingTaskIds.delete(taskId)
  }
}
function updateTask(id: string, patch: Partial<TaskRecord>) { tasks.value = tasks.value.map((task) => (task.id === id ? { ...task, ...patch } : task)) }
function toggleFavorite(task: TaskRecord) { updateTask(task.id, { isFavorite: !task.isFavorite }) }
async function reuseConfig(task: TaskRecord) { prompt.value = task.prompt; settings.value.model = task.model; settings.value.apiMode = task.apiMode; params.value = { ...task.params }; inputImages.value = task.inputImageIds.map((id) => ({ id, dataUrl: imageSrc(id) })).filter((img) => img.dataUrl); appStore.showSuccess('已复用配置到输入框') }
function editOutputs(task: TaskRecord) { const images = task.outputImages.map((id) => ({ id, dataUrl: imageSrc(id) })).filter((img) => img.dataUrl && img.dataUrl.startsWith('data:')); inputImages.value = [...images, ...inputImages.value].slice(0, API_MAX_IMAGES); if (!images.length) appStore.showError('只有 Base64 图片结果可以直接作为参考图继续编辑'); else appStore.showSuccess(`已添加 ${images.length} 张输出图到输入`) }
async function deleteTask(task: TaskRecord) { try { await deleteRemoteImageTask(task.id) } catch (error) { console.warn('Failed to delete remote task:', error) }; tasks.value = tasks.value.filter((item) => item.id !== task.id); if (detailTaskId.value === task.id) detailTaskId.value = null }
function askDeleteTask(task: TaskRecord) { confirmDialog.value = { title: '删除记录', message: '确定要删除这条记录吗？', action: () => deleteTask(task) } }
function askClearAll() { confirmDialog.value = { title: '清空全部', message: '确定清空模型试炼场的本地任务和图片记录吗？此操作只影响当前浏览器。', confirmText: '清空全部', action: clearAllData } }
async function clearAllData() { for (const task of [...tasks.value]) { try { await deleteRemoteImageTask(task.id) } catch { /* ignore */ } }; await dbClear('images'); tasks.value = []; imageCache.value = {}; imageIdByDataUrl.clear(); inputImages.value = []; detailTaskId.value = null; lightboxImageId.value = null }
function runConfirm() { const action = confirmDialog.value?.action; confirmDialog.value = null; action?.() }

async function handleFiles(files: FileList | File[]) {
  const picked = Array.from(files).filter((file) => file.type.startsWith('image/')).slice(0, Math.max(0, API_MAX_IMAGES - inputImages.value.length))
  if (!picked.length) return
  try {
    const imgs: InputImage[] = []
    for (const file of picked) { const dataUrl = await fileToDataURL(file); imgs.push({ id: await storeImage(dataUrl, 'upload'), dataUrl }) }
    inputImages.value = [...inputImages.value, ...imgs].slice(0, API_MAX_IMAGES)
  } catch (error) { appStore.showError((error as Error).message) }
}
async function onInputImagesSelected(event: Event) { const target = event.target as HTMLInputElement; await handleFiles(target.files || []); target.value = '' }
async function onDropImages(event: DragEvent) { isDragging.value = false; if (event.dataTransfer?.files?.length) await handleFiles(event.dataTransfer.files) }
async function onPasteImages(event: ClipboardEvent) {
  const clipboard = event.clipboardData
  if (!clipboard) return
  const files = Array.from(clipboard.items || [])
    .filter((item) => item.kind === 'file' && item.type.startsWith('image/'))
    .map((item) => item.getAsFile())
    .filter((file): file is File => Boolean(file))

  const imageFiles = files.length ? files : Array.from(clipboard.files || []).filter((file) => file.type.startsWith('image/'))
  if (!imageFiles.length) return

  event.preventDefault()
  if (inputImages.value.length >= API_MAX_IMAGES) {
    appStore.showError(`参考图最多支持 ${API_MAX_IMAGES} 张`)
    return
  }
  const before = inputImages.value.length
  await handleFiles(imageFiles)
  const added = inputImages.value.length - before
  if (added > 0) appStore.showSuccess(`已从剪贴板添加 ${added} 张参考图`)
}
function removeInputImage(index: number) { inputImages.value.splice(index, 1) }
function clearInputImages() { inputImages.value = [] }
function openLightbox(id: string, list: string[]) { lightboxImageId.value = id; lightboxList.value = list }
function stepLightbox(delta: number) { if (!lightboxImageId.value || !lightboxList.value.length) return; const index = lightboxList.value.indexOf(lightboxImageId.value); lightboxImageId.value = lightboxList.value[(index + delta + lightboxList.value.length) % lightboxList.value.length] }
function exportData() { const payload = { version: 1, exportedAt: new Date().toISOString(), settings: { selectedKeyId: selectedKeyId.value, settings: settings.value, params: params.value }, tasks: tasks.value, images: imageCache.value }; const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' }); const url = URL.createObjectURL(blob); const a = document.createElement('a'); a.href = url; a.download = `sub2api-gpt-image-playground-${Date.now()}.json`; a.click(); URL.revokeObjectURL(url) }
async function onImportFile(event: Event) { const target = event.target as HTMLInputElement; const file = target.files?.[0]; if (!file) return; try { const payload = JSON.parse(await file.text()); if (payload.images && typeof payload.images === 'object') { for (const [id, dataUrl] of Object.entries(payload.images)) await dbPut('images', { id, dataUrl, source: 'generated', createdAt: Date.now() }); imageCache.value = { ...imageCache.value, ...(payload.images as Record<string, string>) } } appStore.showSuccess('图片缓存已导入；任务记录现在以服务端为准') } catch (error) { appStore.showError((error as Error).message) } finally { target.value = '' } }
function persistState() { if (persistTimer) window.clearTimeout(persistTimer); persistTimer = window.setTimeout(() => localStorage.setItem(storageKey.value, JSON.stringify({ selectedKeyId: selectedKeyId.value, settings: settings.value, params: params.value } satisfies PersistedState)), 250) }
function restoreState() { try { const payload = JSON.parse(localStorage.getItem(storageKey.value) || '{}') as PersistedState; if (payload.selectedKeyId !== undefined) selectedKeyId.value = payload.selectedKeyId; if (payload.settings) settings.value = { ...settings.value, ...payload.settings }; if (payload.params) params.value = { ...params.value, ...payload.params } } catch { /* ignore */ } }

onMounted(async () => {
  await appStore.fetchPublicSettings()
  restoreState()
  await Promise.all([loadApiKeys(), loadLocalData()])
  tickerTimer = window.setInterval(() => { nowTick.value = Date.now() }, 1000)
  window.addEventListener('paste', onPasteImages)
})
onUnmounted(() => {
  if (tickerTimer) window.clearInterval(tickerTimer)
  if (persistTimer) window.clearTimeout(persistTimer)
  window.removeEventListener('paste', onPasteImages)
})
watch(() => settings.value.apiMode, (mode) => { if (mode === 'responses' && settings.value.model === DEFAULT_IMAGES_MODEL) settings.value.model = DEFAULT_RESPONSES_MODEL; if (mode === 'images' && settings.value.model === DEFAULT_RESPONSES_MODEL) settings.value.model = DEFAULT_IMAGES_MODEL; if (mode === 'responses') params.value.moderation = 'auto' })
watch(() => settings.value.codexCli, (enabled) => { if (enabled) params.value.quality = 'auto' })
watch(() => params.value.output_format, (format) => { if (format === 'png') params.value.output_compression = null })
watch([selectedKeyId, settings, params], persistState, { deep: true })
</script>

<style scoped>
.gip-shell { @apply min-h-screen bg-gray-50 text-gray-900 dark:bg-gray-950 dark:text-gray-100; }
.gip-header { @apply sticky top-0 z-40 border-b border-gray-200 bg-white/80 backdrop-blur dark:border-white/[0.08] dark:bg-gray-950/80; }
.gip-header-inner { @apply mx-auto flex h-14 max-w-7xl items-center justify-between px-4; }
.gip-icon-btn { @apply rounded-lg p-2 text-gray-600 transition-colors hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-900; }
.gip-filter-star { @apply rounded-xl border border-gray-200 bg-white p-2.5 text-gray-400 transition-all hover:bg-gray-50 dark:border-white/[0.08] dark:bg-gray-900 dark:hover:bg-white/[0.06]; }
.gip-filter-star-on { @apply border-yellow-400 bg-yellow-50 text-yellow-500 dark:bg-yellow-500/10; }
.gip-search-input { @apply w-full rounded-xl border border-gray-200 bg-white py-2.5 pl-10 pr-4 text-sm transition focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-500/30 dark:border-white/[0.08] dark:bg-gray-900; }
.gip-task-card { @apply relative cursor-pointer overflow-hidden rounded-xl border border-gray-200 bg-white transition-[box-shadow,border-color,background-color,transform] duration-200 hover:border-gray-300 hover:shadow-lg dark:border-white/[0.08] dark:bg-gray-900 dark:hover:border-white/[0.18] dark:hover:bg-gray-800/80; }
.gip-param { @apply flex-shrink-0 rounded-md bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-white/[0.06] dark:text-gray-400; }
.gip-icon-action { @apply inline-flex h-7 w-7 items-center justify-center rounded-md transition-colors disabled:cursor-not-allowed; }
.gip-card-action { @apply rounded-lg border border-gray-200 px-2 py-1 text-xs font-medium text-gray-500 transition hover:bg-gray-50 disabled:opacity-40 dark:border-white/[0.08] dark:text-gray-400 dark:hover:bg-white/[0.06]; }
.gip-input-bar { @apply fixed inset-x-0 bottom-0 z-40 pointer-events-none; }
.gip-input-bar > div { @apply pointer-events-auto; }
.gip-composer { @apply rounded-2xl border border-gray-200 bg-white shadow-2xl shadow-gray-900/10 transition-all dark:border-white/[0.08] dark:bg-gray-900 dark:shadow-black/40; }
.gip-prompt { @apply mx-3 mt-3 block w-[calc(100%-1.5rem)] resize-none rounded-2xl border border-gray-200/60 bg-white/50 px-4 py-3 text-sm leading-relaxed text-gray-900 shadow-sm outline-none transition-[border-color,box-shadow] duration-200 placeholder:text-gray-400 focus:border-blue-300 focus:ring-2 focus:ring-blue-500/20 dark:border-white/[0.08] dark:bg-white/[0.03] dark:text-gray-100 sm:mx-4 sm:w-[calc(100%-2rem)]; }
.gip-tool-btn { @apply inline-flex h-10 items-center justify-center rounded-xl border border-gray-200 bg-white px-3 text-sm text-gray-500 transition hover:bg-gray-50 dark:border-white/[0.08] dark:bg-gray-950 dark:text-gray-300 dark:hover:bg-white/[0.06]; }
.gip-submit { @apply inline-flex h-10 items-center justify-center rounded-xl bg-blue-600 px-4 text-sm font-semibold text-white transition hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50; }
.gip-param-field { @apply flex flex-col gap-0.5; }
.gip-param-field > span { @apply ml-1 text-xs text-gray-400 dark:text-gray-500; }
.gip-param-input { @apply h-[31px] rounded-xl border border-gray-200/60 bg-white/50 px-3 py-1.5 text-xs shadow-sm outline-none transition-all duration-200 placeholder:text-gray-400 focus:border-blue-300 focus:ring-2 focus:ring-blue-500/20 disabled:cursor-not-allowed disabled:bg-gray-100/50 disabled:opacity-50 dark:border-white/[0.08] dark:bg-white/[0.03] dark:disabled:bg-white/[0.05]; }
.gip-size-button { @apply h-[31px] rounded-xl border border-gray-200/60 bg-white/50 px-3 py-1.5 text-left font-mono text-xs shadow-sm outline-none transition-all duration-200 hover:bg-white focus:border-blue-300 focus:ring-2 focus:ring-blue-500/20 dark:border-white/[0.08] dark:bg-white/[0.03] dark:hover:bg-white/[0.06]; }
.gip-param-select :deep(.select-trigger) { @apply h-[31px] rounded-xl border-gray-200/60 bg-white/50 px-3 py-1.5 text-xs shadow-sm focus:border-blue-300 focus:ring-2 focus:ring-blue-500/20 dark:border-white/[0.08] dark:bg-white/[0.03]; }
.gip-param-select :deep(.select-trigger-disabled) { @apply cursor-not-allowed bg-gray-100/50 opacity-50 dark:bg-white/[0.05]; }
.gip-param-select :deep(.select-icon svg) { @apply h-3.5 w-3.5; }
.gip-round-action { @apply inline-flex h-10 w-10 items-center justify-center rounded-xl bg-gray-200 text-gray-500 shadow-sm transition-all hover:bg-gray-300 hover:shadow dark:bg-white/[0.06] dark:text-gray-300 dark:hover:bg-white/[0.1]; }
.gip-round-submit { @apply inline-flex h-10 w-10 items-center justify-center rounded-xl bg-blue-500 text-white shadow-sm transition-all hover:bg-blue-600 hover:shadow disabled:cursor-not-allowed disabled:bg-gray-300 disabled:opacity-50 dark:disabled:bg-white/[0.04]; }
.gip-mobile-submit { @apply flex flex-1 items-center justify-center gap-2 rounded-xl bg-blue-500 py-2.5 text-sm font-medium text-white shadow-sm transition-all hover:bg-blue-600 disabled:cursor-not-allowed disabled:bg-gray-300 disabled:opacity-50 dark:disabled:bg-white/[0.04]; }
.gip-size-tab { @apply flex-1 rounded-lg py-1.5 text-sm font-medium text-gray-500 transition hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200; }
.gip-size-tab-on { @apply bg-white text-gray-800 shadow-sm dark:bg-gray-700 dark:text-gray-100; }
.gip-size-choice { @apply rounded-xl border border-gray-200/70 bg-white/60 px-3 py-2 text-sm text-gray-600 transition hover:bg-gray-50 dark:border-white/[0.08] dark:bg-white/[0.03] dark:text-gray-300 dark:hover:bg-white/[0.06]; }
.gip-size-choice-on { @apply border-blue-400 bg-blue-50 text-blue-600 dark:border-blue-500/50 dark:bg-blue-500/10 dark:text-blue-300; }
.gip-size-input { @apply w-full rounded-xl border border-gray-200/70 bg-white/60 px-3 py-2 text-sm text-gray-700 outline-none transition focus:border-blue-300 dark:border-white/[0.08] dark:bg-white/[0.03] dark:text-gray-200 dark:focus:border-blue-500/50; }
.gip-overlay { @apply fixed inset-0 z-[70] flex animate-overlay-in items-center justify-center bg-black/50 p-4 backdrop-blur-sm; }
.gip-modal { @apply max-h-[92vh] w-full overflow-auto rounded-2xl border border-gray-200 bg-white shadow-2xl dark:border-white/[0.08] dark:bg-gray-900; }
.gip-field { @apply block space-y-1.5 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400; }
.hide-scrollbar { -ms-overflow-style: none; scrollbar-width: none; }
.hide-scrollbar::-webkit-scrollbar { display: none; }
.mask-edge-r { -webkit-mask-image: linear-gradient(to right, #000 86%, transparent 100%); mask-image: linear-gradient(to right, #000 86%, transparent 100%); }
@keyframes pulse-border { 0%, 100% { border-color: rgb(96 165 250); box-shadow: 0 0 0 0 rgb(59 130 246 / 0.18); } 50% { border-color: rgb(147 197 253); box-shadow: 0 0 0 4px rgb(59 130 246 / 0.08); } }
.generating { animation: pulse-border 1.8s ease-in-out infinite; }
@keyframes overlay-in { from { opacity: 0; } to { opacity: 1; } }
.animate-overlay-in { animation: overlay-in .2s ease-out both; }
@keyframes modal-in { from { opacity: 0; transform: scale(.95) translateY(10px); } to { opacity: 1; transform: scale(1) translateY(0); } }
.animate-modal-in { animation: modal-in .25s cubic-bezier(.16,1,.3,1) both; }
@keyframes slide-down-in { from { opacity: 0; transform: translateY(-20px); } to { opacity: 1; transform: translateY(0); } }
.animate-slide-down-in { animation: slide-down-in .25s cubic-bezier(.16,1,.3,1) both; }
@keyframes fade-in { from { opacity: 0; } to { opacity: 1; } }
.animate-fade-in { animation: fade-in .2s ease-out both; }
@keyframes zoom-in { from { opacity: 0; transform: scale(.9); } to { opacity: 1; transform: scale(1); } }
.animate-zoom-in { animation: zoom-in .25s cubic-bezier(.16,1,.3,1) both; }
@keyframes confirm-in { from { opacity: 0; transform: scale(.92) translateY(16px); } to { opacity: 1; transform: scale(1) translateY(0); } }
.animate-confirm-in { animation: confirm-in .2s cubic-bezier(.16,1,.3,1) both; }
</style>
