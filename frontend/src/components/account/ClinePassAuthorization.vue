<template>
  <section
    class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600"
    data-testid="clinepass-authorization"
  >
    <h3 class="text-sm font-medium text-gray-900 dark:text-gray-100">
      {{ t('admin.accounts.clinePassAuth.title') }}
    </h3>
    <p class="input-hint">{{ t('admin.accounts.clinePassAuth.description') }}</p>

    <button v-if="!deviceCode" type="button" class="btn btn-primary" :disabled="loading" @click="start">
      {{ loading ? t('admin.accounts.clinePassAuth.starting') : t('admin.accounts.clinePassAuth.start') }}
    </button>

    <template v-else>
      <p class="input-hint">{{ t('admin.accounts.clinePassAuth.instructions') }}</p>
      <div class="flex flex-wrap items-center gap-2">
        <a
          :href="verificationUri"
          target="_blank"
          rel="noreferrer"
          class="btn btn-secondary"
        >
          {{ t('admin.accounts.clinePassAuth.openPage') }}
        </a>
        <code
          data-testid="clinepass-user-code"
          class="select-all rounded bg-gray-100 px-3 py-1.5 font-mono text-sm dark:bg-dark-700"
        >{{ userCode }}</code>
      </div>
      <p class="input-hint" data-testid="clinepass-status">{{ statusText }}</p>
      <div class="flex gap-2">
        <button
          type="button"
          class="btn btn-primary"
          :disabled="loading || approved"
          @click="onCreateClick"
        >
          {{ approved ? t('admin.accounts.clinePassAuth.creating') : t('admin.accounts.clinePassAuth.checkAndCreate') }}
        </button>
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="cancel">
          {{ t('common.cancel') }}
        </button>
      </div>
    </template>

    <p v-if="error" class="text-sm text-red-600 dark:text-red-400" role="alert">{{ error }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'

interface Props {
  name?: string
  proxyId?: number | null
  concurrency?: number
  priority?: number
  groupIds?: number[]
}

const props = withDefaults(defineProps<Props>(), {
  name: '',
  proxyId: null,
  concurrency: 10,
  priority: 1,
  groupIds: () => []
})

const emit = defineEmits<{ created: [] }>()

const { t } = useI18n()

const deviceCode = ref('')
const userCode = ref('')
const verificationUri = ref('')
const intervalSeconds = ref(5)
const loading = ref(false)
const approved = ref(false)
const error = ref('')

let timer: ReturnType<typeof setTimeout> | null = null

const statusText = computed(() =>
  approved.value
    ? t('admin.accounts.clinePassAuth.approved')
    : t('admin.accounts.clinePassAuth.waiting')
)

function stopTimer() {
  if (timer !== null) {
    clearTimeout(timer)
    timer = null
  }
}

onBeforeUnmount(stopTimer)

function cancel() {
  stopTimer()
  deviceCode.value = ''
  userCode.value = ''
  verificationUri.value = ''
  loading.value = false
  approved.value = false
  error.value = ''
}

async function start() {
  error.value = ''
  loading.value = true
  try {
    const result = await adminAPI.clinepass.startDeviceAuth({ proxy_id: props.proxyId })
    deviceCode.value = result.device_code
    userCode.value = result.user_code
    verificationUri.value = result.verification_uri_complete || result.verification_uri
    intervalSeconds.value = result.interval && result.interval > 0 ? result.interval : 5
  } catch (cause: any) {
    error.value = cause?.response?.data?.message || cause?.response?.data?.detail || t('admin.accounts.clinePassAuth.errors.start')
  } finally {
    loading.value = false
  }
}

function schedulePoll() {
  stopTimer()
  timer = setTimeout(() => {
    void createAccount()
  }, Math.max(2, intervalSeconds.value) * 1000)
}

function onCreateClick() {
  void createAccount()
}

async function createAccount() {
  if (!deviceCode.value) return
  stopTimer()
  loading.value = true
  error.value = ''
  try {
    const result = await adminAPI.clinepass.createFromDevice({
      device_code: deviceCode.value,
      name: props.name?.trim() || undefined,
      proxy_id: props.proxyId,
      concurrency: props.concurrency,
      priority: props.priority,
      group_ids: props.groupIds
    })
    if (result.status === 'approved') {
      approved.value = true
      deviceCode.value = ''
      emit('created')
      return
    }
    schedulePoll()
  } catch (cause: any) {
    error.value = cause?.response?.data?.message || cause?.response?.data?.detail || t('admin.accounts.clinePassAuth.errors.create')
  } finally {
    loading.value = false
  }
}
</script>
