import { afterEach, describe, expect, it, vi } from 'vitest'
import { createPasskey, decodeBase64URL, encodeBase64URL, getPasskey, parseCreationOptions, parseRequestOptions, passkeyError, passkeysSupported, serializeCredential } from './passkeys'

afterEach(() => vi.unstubAllGlobals())

describe('WebAuthn browser conversion', () => {
  it('round trips all byte values as unpadded base64url', () => {
    const bytes = Uint8Array.from({ length: 256 }, (_, index) => index)
    const encoded = encodeBase64URL(bytes.buffer)
    expect(encoded).not.toMatch(/[+/=]/)
    expect(new Uint8Array(decodeBase64URL(encoded))).toEqual(bytes)
  })

  it('decodes all creation binary fields without changing server policy or the input', () => {
    const options = {
      challenge: 'AQI', rp: { name: 'KPanel', id: 'panel.example.com' },
      user: { id: 'AwQ', name: 'admin', displayName: 'Admin' },
      pubKeyCredParams: [{ type: 'public-key' as const, alg: -7 }],
      excludeCredentials: [{ type: 'public-key' as const, id: '-_8', transports: ['usb' as const] }],
      authenticatorSelection: { userVerification: 'required' as const, residentKey: 'required' as const },
    }
    const parsed = parseCreationOptions(options)
    expect(new Uint8Array(parsed.challenge as ArrayBuffer)).toEqual(new Uint8Array([1, 2]))
    expect(new Uint8Array(parsed.user.id as ArrayBuffer)).toEqual(new Uint8Array([3, 4]))
    expect(new Uint8Array(parsed.excludeCredentials![0]!.id as ArrayBuffer)).toEqual(new Uint8Array([251, 255]))
    expect(parsed.authenticatorSelection).toEqual(options.authenticatorSelection)
    expect(options.challenge).toBe('AQI')
    expect(options.user.id).toBe('AwQ')
  })

  it('decodes request allow credentials and preserves required user verification', () => {
    const parsed = parseRequestOptions({ challenge: 'AA', rpId: 'panel.example.com', userVerification: 'required', allowCredentials: [{ id: 'AQ', type: 'public-key' }] })
    expect(new Uint8Array(parsed.allowCredentials![0]!.id as ArrayBuffer)).toEqual(new Uint8Array([1]))
    expect(parsed.userVerification).toBe('required')
  })

  it('serializes attestation and assertion responses without depending on newer toJSON APIs', () => {
    const key = {
      id: '-_8', rawId: new Uint8Array([251, 255]).buffer, type: 'public-key',
      getClientExtensionResults: () => ({ credProps: { rk: true } }),
      response: { clientDataJSON: new Uint8Array([1]).buffer, attestationObject: new Uint8Array([2]).buffer, getTransports: () => ['usb'] },
    }
    expect(serializeCredential(key as unknown as Credential)).toMatchObject({ rawId: '-_8', clientExtensionResults: { credProps: { rk: true } }, response: { clientDataJSON: 'AQ', attestationObject: 'Ag', transports: ['usb'] } })
    const assertion = { ...key, response: { clientDataJSON: new Uint8Array([1]).buffer, authenticatorData: new Uint8Array([2]).buffer, signature: new Uint8Array([3]).buffer, userHandle: null } }
    expect(serializeCredential(assertion as unknown as Credential).response).toEqual({ clientDataJSON: 'AQ', authenticatorData: 'Ag', signature: 'Aw', userHandle: null })
  })

  it('passes the abort signal and reports cancellation without inventing a credential', async () => {
    const create = vi.fn().mockResolvedValue(null)
    const get = vi.fn().mockRejectedValue(new DOMException('cancelled', 'NotAllowedError'))
    vi.stubGlobal('navigator', { credentials: { create, get } })
    const signal = new AbortController().signal
    await expect(createPasskey({ challenge: 'AA', rp: { name: 'Panel' }, user: { id: 'AQ', name: 'admin', displayName: 'Admin' }, pubKeyCredParams: [] }, signal)).rejects.toMatchObject({ name: 'NotAllowedError' })
    await expect(getPasskey({ challenge: 'AA' }, signal)).rejects.toMatchObject({ name: 'NotAllowedError' })
    expect(create).toHaveBeenCalledWith(expect.objectContaining({ signal }))
    expect(get).toHaveBeenCalledWith(expect.objectContaining({ signal }))
    expect(passkeyError(new DOMException('cancelled', 'NotAllowedError'))).toContain('取消或超时')
  })

  it('does not require a platform authenticator, but requires a secure context and both credential methods', () => {
    vi.stubGlobal('window', { isSecureContext: true })
    vi.stubGlobal('navigator', { credentials: { create: vi.fn(), get: vi.fn() } })
    vi.stubGlobal('PublicKeyCredential', function () {})
    expect(passkeysSupported()).toBe(true)
    vi.stubGlobal('window', { isSecureContext: false })
    expect(passkeysSupported()).toBe(false)
  })
})
