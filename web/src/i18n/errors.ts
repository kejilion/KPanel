import { t, type TranslationParams } from './index'
import type { MessageKey } from './messages/zh-CN'

interface CodedError {
  code?: string
  message?: string
  status?: number
}

const errorMessageKeys: Readonly<Record<string, MessageKey>> = {
  network_error: 'error.network',
  unauthenticated: 'error.authenticationRequired',
  authentication_required: 'error.authenticationRequired',
  invalid_credentials: 'error.invalidCredentials',
  auth_failed: 'error.invalidCredentials',
  login_failed: 'error.invalidCredentials',
  login_rate_limited: 'error.authenticationRateLimited',
  authentication_rate_limited: 'error.authenticationRateLimited',
  rate_limited: 'error.authenticationRateLimited',
  invalid_bootstrap_token: 'error.invalidBootstrapToken',
  bootstrap_unavailable: 'error.bootstrapUnavailable',
  bootstrap_failed: 'error.bootstrapFailed',
  second_factor_unavailable: 'error.secondFactorUnavailable',
  forbidden: 'error.forbidden',
  agent_unavailable: 'error.agentUnavailable',
  local_agent_unavailable: 'error.agentUnavailable',
  resource_conflict: 'error.resourceChanged',
  resource_version_changed: 'error.resourceChanged',
  validation_failed: 'error.validationFailed',
  invalid_input: 'error.validationFailed',
  invalid_request: 'error.validationFailed',
}

export function localizeError(
  reason: unknown,
  fallbackKey: MessageKey = 'error.requestFailed',
  params?: TranslationParams,
): string {
  if (reason && typeof reason === 'object') {
    const error = reason as CodedError
    const key = error.code ? errorMessageKeys[error.code] : undefined
    if (key) return t(key, params)
    if (error.code?.startsWith('http_') && typeof error.status === 'number' && Number.isFinite(error.status)) {
      return t('error.httpStatus', { ...params, status: error.status })
    }
    if (error.message) return error.message
  }
  return t(fallbackKey, params)
}
