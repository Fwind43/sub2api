<template>
  <AppLayout>
    <section class="space-y-5">
      <header class="flex flex-wrap items-center justify-between gap-3">
        <div><h1 class="text-2xl font-semibold">MiniMax H3 状态</h1>
          <p class="text-sm text-gray-500">每 30 秒刷新<span v-if="updated"> · 上次更新 {{ updated }}</span></p></div>
        <button class="btn btn-secondary" :disabled="loading" @click="refresh">{{ loading ? '刷新中…' : '立即刷新' }}</button>
      </header>
      <p v-if="error" role="alert">状态读取失败，请稍后刷新。</p>
      <p v-else-if="!loading && !nodes.length">尚无 H3 账号。</p>
      <article v-for="node in nodes" :key="node.id" class="card space-y-3 p-5">
        <h2 class="font-semibold">{{ node.name }} <span class="text-sm text-gray-500">#{{ node.id }}</span></h2>
        <p class="font-medium">{{ node.health?.stale || node.health?.accepting == null ? '接单状态未知' : node.health.accepting ? '可接新单' : '暂停接单' }}</p>
        <p class="text-sm">{{ node.health?.ok == null ? '服务健康未报告' : node.health.ok ? '服务在线' : '服务异常' }} · {{ node.health?.upstream == null ? '推理后端状态未知' : node.health.upstream ? '推理后端在线' : '推理后端不可用' }}</p>
        <p v-if="node.health?.observed_at || node.health?.stale" class="text-sm text-gray-500">上游采样 {{ node.health?.observed_at }} {{ node.health?.stale ? '（已过期）' : '' }}</p>
        <p v-if="node.health?.computing != null || node.health?.queued != null || node.health?.collecting != null" class="text-sm">节点任务：处理中 {{ node.health?.computing ?? '—' }} · 等待中 {{ node.health?.queued ?? '—' }} · 保存成片 {{ node.health?.collecting ?? '—' }}</p>
        <p v-if="node.health?.gpu" class="text-sm">{{ node.health.gpu }}</p>
        <dl class="grid grid-cols-2 gap-3 text-sm md:grid-cols-4">
          <div><dt>节点活跃任务</dt><dd>{{ node.health?.active_jobs ?? '—' }}</dd></div>
          <div><dt>节点队列上限</dt><dd>{{ node.health?.max_queue ?? '—' }}</dd></div>
          <div><dt>网关在途任务</dt><dd>{{ node.pending }}</dd></div>
          <div><dt>其中已回报排队位置</dt><dd>{{ node.queued }}</dd></div>
        </dl>
      </article>
    </section>
  </AppLayout>
</template>
<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import apiClient from '@/api/client'
interface NodeStatus {
  id: number; name: string; pending: number; queued: number
  health: { accepting: boolean | null; observed_at: string; stale: boolean; queued: number | null; computing: number | null; collecting: number | null; ok: boolean | null; upstream: boolean | null; gpu: string; active_jobs: number | null; max_queue: number | null } | null
}
const nodes = ref<NodeStatus[]>([])
const updated = ref('')
const error = ref(false)
const loading = ref(false)
let timer: ReturnType<typeof setInterval>
async function refresh() {
  if (loading.value) return
  loading.value = true
  try {
    const { data } = await apiClient.get<NodeStatus[]>('/admin/h3-status')
    nodes.value = data
    updated.value = new Date().toLocaleTimeString()
    error.value = false
  } catch {
    nodes.value = []
    error.value = true
  } finally {
    loading.value = false
  }
}
onMounted(() => { void refresh(); timer = setInterval(() => { if (!document.hidden) void refresh() }, 30000) })
onBeforeUnmount(() => clearInterval(timer))
</script>
