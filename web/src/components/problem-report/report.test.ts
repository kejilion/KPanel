import { describe, expect, it } from 'vitest'
import { buildReport, captureReport, redactDescription, MAX_DESCRIPTION_LENGTH } from './report'

const seed = 'seed-secret-DO-NOT-EXPORT'
const privateKey = `-----BEGIN OPENSSH PRIVATE KEY-----\n${seed}\n-----END OPENSSH PRIVATE KEY-----`
const job = {
  id: `docker:${'a'.repeat(32)}`, action: 'docker.image_pull', status: 'failed' as const,
  createdAt: '2026-09-06T00:00:00Z', errorCode: 'image_pull_failed',
  errorMessage: seed, resourceName: `/home/${seed}`, actor: seed,
  stages: [{ name: 'failed', status: 'failed' as const, message: privateKey }],
}

describe('local problem report data boundary', () => {
  it('projects a field whitelist without visiting exception details or logs', () => {
    const error = { code: 'network_error', requestId: 'b'.repeat(32), status: 503,
      message: privateKey, get details() { throw new Error('must never inspect details') } }
    const snapshot = captureReport({ source: 'job', feature: 'jobs', job, error, recordState: 'available' })
    const serialized = buildReport(snapshot, [], '', '')
    expect(serialized).not.toContain(seed)
    expect(serialized).not.toContain('/home/')
    expect(JSON.parse(serialized)).toMatchObject({ source: 'job', feature: 'jobs', jobId: job.id,
      requestId: error.requestId, errorCode: error.code, jobStatus: 'failed', failureStage: 'failed' })
  })

  it('never recovers missing identifiers from messages, URL or unavailable records', () => {
    const report = JSON.parse(buildReport(captureReport({ source: 'job', feature: 'jobs',
      job, recordState: 'unavailable' }), [], '', ''))
    expect(report.jobId).toBeNull()
    expect(report.requestId).toBeNull()
    expect(report.jobStatus).toBeNull()
    expect(report.recordState).toBe('unavailable')
  })

  it('does not infer a failed stage from the last successful stage', () => {
    const report = captureReport({ source: 'job', feature: 'jobs', recordState: 'available',
      job: { ...job, stages: [{ name: 'completed', status: 'succeeded' }] } })
    expect(report.failureStage).toBeNull()
  })

  it('rejects malformed and excessive values instead of truncating them into identifiers', () => {
    const snapshot = captureReport({ source: 'error', feature: 'unknown', error: {
      code: `https://user:${seed}@example.test/?token=${seed}`, requestId: privateKey, status: 999999,
    } })
    expect(snapshot.errorCode).toBeNull()
    expect(snapshot.requestId).toBeNull()
    expect(snapshot.httpStatus).toBeNull()
    const longJob = captureReport({ source: 'job', feature: 'jobs', recordState: 'available',
      job: { ...job, action: seed.repeat(1000) } })
    expect(longJob.action).toBeNull()
  })

  it('captures a stable snapshot and applies removals to the exported bytes', () => {
    const input = { ...job }
    const snapshot = captureReport({ source: 'job', feature: 'jobs', job: input, recordState: 'available' })
    input.status = 'running' as typeof input.status
    const report = JSON.parse(buildReport(snapshot, ['jobId', 'action'], 'expected behavior', 'actual behavior'))
    expect(report.jobStatus).toBe('failed')
    expect(report).not.toHaveProperty('jobId')
    expect(report).not.toHaveProperty('action')
    expect(report.expected).toBe('expected behavior')
  })

  it.each([privateKey, `-----BEGIN RSA PRIVATE KEY-----\n${seed}`, `${seed}\n-----END PRIVATE KEY-----`,
    `-----begin private key-----\n${seed}`, `-----BEGIN ED25519 PRIVATE KEY-----\n${seed}`,
    `https://user:${seed}@example.test/private/${seed}?token=${seed}`,
    `Authorization: Bearer ${seed}\nCookie: value=${seed}\npassword=${seed}`,
    seed.repeat(MAX_DESCRIPTION_LENGTH),
  ])('removes secrets from optional descriptions, including incomplete multiline keys', (value) => {
    expect(redactDescription(value)).not.toContain(seed)
  })

  it('supports a manual report without an exception, network or persisted state', () => {
    const report = JSON.parse(buildReport(captureReport({ source: 'manual', feature: 'settings' }), [], 'A', 'B'))
    expect(report.source).toBe('manual')
    expect(report.requestId).toBeNull()
    expect(report.jobId).toBeNull()
    expect(report.webVersion).toMatch(/^\d+\.\d+\.\d+/)
    expect(report.expected).toBe('A')
  })
})
