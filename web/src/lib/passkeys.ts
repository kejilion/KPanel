import { t } from '@/i18n'
import { localizeError } from '@/i18n/errors'

type DescriptorJSON = Omit<PublicKeyCredentialDescriptor, 'id'> & { id: string }
export type PasskeyCreationOptions = Omit<PublicKeyCredentialCreationOptions, 'challenge' | 'user' | 'excludeCredentials'> & {
  challenge: string
  user: Omit<PublicKeyCredentialUserEntity, 'id'> & { id: string }
  excludeCredentials?: DescriptorJSON[]
}
export type PasskeyRequestOptions = Omit<PublicKeyCredentialRequestOptions, 'challenge' | 'allowCredentials'> & {
  challenge: string
  allowCredentials?: DescriptorJSON[]
}
export interface PasskeyCredentialJSON {
  id: string
  rawId: string
  type: string
  authenticatorAttachment?: string | null
  clientExtensionResults: AuthenticationExtensionsClientOutputs
  response: {
    clientDataJSON: string
    attestationObject?: string
    transports?: string[]
    authenticatorData?: string
    signature?: string
    userHandle?: string | null
  }
}

export function passkeysSupported(): boolean {
  return typeof window !== 'undefined' && window.isSecureContext &&
    typeof PublicKeyCredential !== 'undefined' &&
    typeof navigator.credentials?.create === 'function' && typeof navigator.credentials?.get === 'function'
}

export function decodeBase64URL(value: string): ArrayBuffer {
  const binary = atob(value.replace(/-/g, '+').replace(/_/g, '/'))
  return Uint8Array.from(binary, character => character.charCodeAt(0)).buffer
}

export function encodeBase64URL(value: ArrayBuffer): string {
  let binary = ''
  for (const byte of new Uint8Array(value)) binary += String.fromCharCode(byte)
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export function parseCreationOptions(options: PasskeyCreationOptions): PublicKeyCredentialCreationOptions {
  return {
    ...options,
    challenge: decodeBase64URL(options.challenge),
    user: { ...options.user, id: decodeBase64URL(options.user.id) },
    excludeCredentials: options.excludeCredentials?.map(item => ({ ...item, id: decodeBase64URL(item.id) })),
  }
}

export function parseRequestOptions(options: PasskeyRequestOptions): PublicKeyCredentialRequestOptions {
  return {
    ...options,
    challenge: decodeBase64URL(options.challenge),
    allowCredentials: options.allowCredentials?.map(item => ({ ...item, id: decodeBase64URL(item.id) })),
  }
}

export function serializeCredential(credential: Credential | null): PasskeyCredentialJSON {
  if (!credential || credential.type !== 'public-key') throw new DOMException('No public key credential', 'NotAllowedError')
  const key = credential as PublicKeyCredential
  const response = key.response
  const result: PasskeyCredentialJSON = {
    id: key.id,
    rawId: encodeBase64URL(key.rawId),
    type: key.type,
    authenticatorAttachment: key.authenticatorAttachment,
    clientExtensionResults: key.getClientExtensionResults(),
    response: { clientDataJSON: encodeBase64URL(response.clientDataJSON) },
  }
  // Structural checks also work on browsers without exposed response constructors.
  if ('attestationObject' in response) {
    const attestation = response as AuthenticatorAttestationResponse
    result.response.attestationObject = encodeBase64URL(attestation.attestationObject)
    result.response.transports = attestation.getTransports?.() || []
  } else {
    const assertion = response as AuthenticatorAssertionResponse
    result.response.authenticatorData = encodeBase64URL(assertion.authenticatorData)
    result.response.signature = encodeBase64URL(assertion.signature)
    result.response.userHandle = assertion.userHandle === null ? null : encodeBase64URL(assertion.userHandle)
  }
  return result
}

export async function createPasskey(options: PasskeyCreationOptions, signal?: AbortSignal): Promise<PasskeyCredentialJSON> {
  return serializeCredential(await navigator.credentials.create({ publicKey: parseCreationOptions(options), signal }))
}

export async function getPasskey(options: PasskeyRequestOptions, signal?: AbortSignal): Promise<PasskeyCredentialJSON> {
  return serializeCredential(await navigator.credentials.get({ publicKey: parseRequestOptions(options), signal }))
}

export function passkeyError(reason: unknown): string {
  if (reason && typeof reason === 'object' && 'code' in reason) {
    switch (reason.code) {
      case 'passkey_unavailable': return t('passkey.unavailable')
      case 'passkey_limit': return t('passkey.limit')
      case 'passkey_failed': return t('passkey.failed')
      case 'invalid_credentials': return t('passkey.invalid')
      case 'invalid_second_factor': return t('passkey.invalidFactor')
      case 'totp_required': return t('passkey.currentFactor')
      case 'audit_unavailable': return t('passkey.auditUnavailable')
      case 'backup_busy': return t('passkey.backupBusy')
    }
  }
  if (reason instanceof DOMException) {
    if (reason.name === 'NotAllowedError' || reason.name === 'AbortError') return t('passkey.cancelled')
    if (reason.name === 'InvalidStateError') return t('passkey.duplicate')
    if (reason.name === 'SecurityError' || reason.name === 'NotSupportedError') return t('passkey.unsupported')
  }
  return localizeError(reason, 'passkey.failed')
}
