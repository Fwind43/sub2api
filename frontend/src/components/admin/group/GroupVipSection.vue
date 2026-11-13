<template>
  <div class="rounded-lg border border-amber-200 p-3 dark:border-amber-800/60">
    <h4 class="mb-2 text-sm font-medium text-amber-700 dark:text-amber-400">
      {{ t('admin.groups.vip.title') }}
    </h4>
    <p class="mb-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.groups.vip.hint') }}</p>
    <div class="flex items-center gap-2">
      <span class="text-sm text-gray-600 dark:text-gray-300">{{ t('admin.groups.vip.rate') }}</span>
      <input
        v-model.number="rate"
        type="number"
        step="0.001"
        min="0"
        autocomplete="off"
        class="hide-spinner input w-28"
        :placeholder="t('admin.groups.vip.ratePlaceholder')"
      />
      <button type="button" class="btn btn-primary btn-sm" :disabled="saving || rate == null || rate < 0" @click="saveRate">
        {{ t('common.save') }}
      </button>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="saving || serverRate == null" @click="clearRate">
        {{ t('admin.groups.vip.clear') }}
      </button>
    </div>

    <div class="mt-3 border-t border-gray-100 pt-3 dark:border-dark-600">
      <div class="mb-1 text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.groups.vip.users') }} ({{ vipUsers.length }})
      </div>
      <div class="relative">
        <input
          v-model="query"
          type="text"
          autocomplete="off"
          class="input w-full"
          :placeholder="t('admin.groups.searchUserPlaceholder')"
          @input="onSearch"
        />
        <div
          v-if="dropdown && results.length > 0"
          class="absolute left-0 right-0 top-full z-10 mt-1 max-h-48 overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-500 dark:bg-dark-700"
        >
          <button
            v-for="u in results"
            :key="u.id"
            type="button"
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-600"
            @click="addUser(u)"
          >
            <span class="text-gray-400">#{{ u.id }}</span>
            <span class="text-gray-900 dark:text-white">{{ u.username || u.email }}</span>
            <span v-if="u.username" class="text-xs text-gray-400">{{ u.email }}</span>
          </button>
        </div>
      </div>
      <div v-if="vipUsers.length === 0" class="py-3 text-center text-sm text-gray-400">
        {{ t('admin.groups.vip.empty') }}
      </div>
      <div v-else class="mt-2 flex max-h-40 flex-wrap gap-2 overflow-auto">
        <span
          v-for="v in vipUsers"
          :key="v.user_id"
          class="inline-flex items-center gap-1 rounded-full bg-amber-50 px-2.5 py-1 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-300"
        >
          #{{ v.user_id }} {{ v.username || v.email }}
          <button type="button" class="ml-1 text-amber-500 hover:text-red-500" @click="removeUser(v.user_id)">×</button>
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { VIPUserEntry } from '@/api/admin/groups'
import type { AdminUser } from '@/types'

const props = defineProps<{ show: boolean; groupId: number | null }>()
const emit = defineEmits<{ changed: [] }>()

const { t } = useI18n()
const appStore = useAppStore()

const rate = ref<number | null>(null)
const serverRate = ref<number | null>(null)
const saving = ref(false)
const vipUsers = ref<VIPUserEntry[]>([])
const query = ref('')
const results = ref<AdminUser[]>([])
const dropdown = ref(false)
let timer: ReturnType<typeof setTimeout>
let version = 0

const load = async () => {
  if (props.groupId == null) return
  const v = ++version
  try {
    const [r, users] = await Promise.all([
      adminAPI.groups.getGroupVIPRate(props.groupId),
      adminAPI.groups.listVIPUsers()
    ])
    if (v !== version) return
    serverRate.value = r
    rate.value = r
    vipUsers.value = users
  } catch (e) {
    if (v !== version) return
    appStore.showError(t('admin.groups.failedToLoad'))
    console.error('Error loading VIP settings:', e)
  }
}

watch([() => props.show, () => props.groupId], ([show]) => {
  if (show) {
    query.value = ''
    results.value = []
    load()
  }
}, { immediate: true })

const saveRate = async () => {
  if (props.groupId == null || rate.value == null || rate.value < 0) return
  saving.value = true
  try {
    await adminAPI.groups.setGroupVIPRate(props.groupId, rate.value)
    serverRate.value = rate.value
    appStore.showSuccess(t('admin.groups.vip.saved'))
    emit('changed')
  } catch (e) {
    appStore.showError(t('admin.groups.failedToLoad'))
    console.error(e)
  } finally {
    saving.value = false
  }
}

const clearRate = async () => {
  if (props.groupId == null) return
  saving.value = true
  try {
    await adminAPI.groups.setGroupVIPRate(props.groupId, null)
    serverRate.value = null
    rate.value = null
    appStore.showSuccess(t('admin.groups.vip.saved'))
    emit('changed')
  } catch (e) {
    appStore.showError(t('admin.groups.failedToLoad'))
    console.error(e)
  } finally {
    saving.value = false
  }
}

const onSearch = () => {
  clearTimeout(timer)
  if (!query.value.trim()) {
    results.value = []
    dropdown.value = false
    return
  }
  timer = setTimeout(async () => {
    try {
      const res = await adminAPI.users.list(1, 10, { search: query.value.trim() })
      results.value = res.items
      dropdown.value = true
    } catch {
      results.value = []
    }
  }, 300)
}

const addUser = async (u: AdminUser) => {
  try {
    await adminAPI.groups.addVIPUsers([u.id])
    query.value = ''
    results.value = []
    dropdown.value = false
    vipUsers.value = await adminAPI.groups.listVIPUsers()
  } catch (e) {
    appStore.showError(t('admin.groups.failedToLoad'))
    console.error(e)
  }
}

const removeUser = async (id: number) => {
  try {
    await adminAPI.groups.removeVIPUsers([id])
    vipUsers.value = vipUsers.value.filter(v => v.user_id !== id)
  } catch (e) {
    appStore.showError(t('admin.groups.failedToLoad'))
    console.error(e)
  }
}
</script>
