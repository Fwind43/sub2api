<template>
  <section class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600" data-testid="commandcode-authorization">
    <h3 class="text-sm font-medium text-gray-900 dark:text-gray-100">{{ t('admin.accounts.commandCodeAuth.title') }}</h3>
    <p class="input-hint">{{ t('admin.accounts.commandCodeAuth.description') }}</p>
    <button type="button" class="btn btn-primary" @click="start">{{ t('admin.accounts.commandCodeAuth.start') }}</button>
    <template v-if="session">
      <p class="input-hint">{{ t('admin.accounts.commandCodeAuth.instructions') }}</p>
      <a :href="authorizationUrl" target="_blank" rel="noopener noreferrer" referrerpolicy="no-referrer" class="text-sm text-primary-600 underline dark:text-primary-400">{{ t('admin.accounts.commandCodeAuth.open') }}</a>
      <label class="input-label" for="commandcode-callback-result">{{ t('admin.accounts.commandCodeAuth.result') }}</label>
      <input id="commandcode-callback-result" v-model="callbackResult" type="password" class="input font-mono" autocomplete="off" autocapitalize="off" :spellcheck="false" :placeholder="t('admin.accounts.commandCodeAuth.placeholder')" @keydown.enter.prevent="accept" />
      <p class="input-hint">{{ t('admin.accounts.commandCodeAuth.secretWarning') }}</p>
      <div class="flex gap-2">
        <button type="button" class="btn btn-primary" :disabled="!callbackResult.trim()" @click="accept">{{ t('admin.accounts.commandCodeAuth.accept') }}</button>
        <button type="button" class="btn btn-secondary" @click="cancel">{{ t('admin.accounts.commandCodeAuth.cancel') }}</button>
      </div>
    </template>
    <p v-if="error" class="text-sm text-red-600 dark:text-red-400" role="alert">{{ t(`admin.accounts.commandCodeAuth.errors.${error}`) }}</p>
    <p v-if="accepted" class="text-sm text-green-600 dark:text-green-400" role="status">{{ t('admin.accounts.commandCodeAuth.accepted') }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  createCommandCodeAuthorizationSession,
  commandCodeAuthorizationUrl,
  consumeCommandCodeAuthorizationResult,
  type CommandCodeAuthorizationSession,
  type CommandCodeAuthorizationResult
} from '@/utils/commandcodeAuthorization'

const emit = defineEmits<{ authorized: [result: CommandCodeAuthorizationResult] }>()
const { t } = useI18n()
const session = ref<CommandCodeAuthorizationSession | null>(null)
const callbackResult = ref('')
const error = ref('')
const accepted = ref(false)
const authorizationUrl = computed(() => session.value ? commandCodeAuthorizationUrl(session.value) : '')
function cancel() {
  if (session.value) session.value.consumed = true
  session.value = null
  callbackResult.value = ''
  error.value = ''
  accepted.value = false
}
function start() {
  cancel()
  try {
    session.value = createCommandCodeAuthorizationSession()
  } catch {
    error.value = 'unavailable'
  }
}
function accept() {
  error.value = ''
  if (!session.value) return
  try {
    const result = consumeCommandCodeAuthorizationResult(callbackResult.value, session.value)
    callbackResult.value = ''
    session.value = null
    emit('authorized', result)
    accepted.value = true
  } catch (cause) {
    const code = cause instanceof Error ? cause.message : 'invalid'
    error.value = ['used', 'expired', 'invalid', 'state', 'denied', 'missingKey'].includes(code) ? code : 'invalid'
    callbackResult.value = ''
    if (code === 'expired' || code === 'used') session.value = null
  }
}
onBeforeUnmount(cancel)
</script>
