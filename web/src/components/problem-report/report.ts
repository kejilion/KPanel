import { version as webVersion } from '../../../package.json'
import type { Job } from '@/types/api'

export const MAX_DESCRIPTION_LENGTH = 2000
export const REPORT_FIELDS = ['capturedAt', 'source', 'feature', 'webVersion', 'browser', 'clientSystem',
  'recordState', 'errorCode', 'httpStatus', 'requestId', 'jobId', 'action', 'jobStatus', 'failureStage'] as const
export type ReportField = typeof REPORT_FIELDS[number]
export type ReportSnapshot = Readonly<Record<ReportField, string | number | null>>
export type ReportFeature = 'unknown' | 'settings' | 'jobs'
export interface ReportError { code?: string; status?: number; requestId?: string }
export interface ReportInput {
  source: 'manual' | 'error' | 'job'
  feature: ReportFeature
  error?: ReportError
  job?: Job
  recordState?: 'available' | 'refreshing' | 'unavailable'
}

function boundedMatch(value: unknown, pattern: RegExp, max: number): string | null {
  return typeof value === 'string' && value.length <= max && pattern.test(value) ? value : null
}
const identifier = (value: unknown) => boundedMatch(value, /^(?:[a-f0-9]{32}|fallback-[0-9]{1,20})$/, 40)
const code = (value: unknown) => boundedMatch(value, /^[a-z][a-z0-9_]{0,79}$/, 80)
const statuses = new Set(['queued', 'running', 'succeeded', 'failed', 'failed_rolled_back', 'failed_needs_attention', 'interrupted', 'cancelled'])
const failures = new Set(['failed', 'failed_rolled_back', 'failed_needs_attention', 'interrupted'])

// Only categories are retained; raw UA, navigator properties, locations and routes are never serialized.
function clientCategories(): { browser: string | null; clientSystem: string | null } {
  const ua = typeof navigator === 'undefined' ? '' : navigator.userAgent.slice(0, 512)
  const browser = /Edg\//.test(ua) ? 'Edge' : /Firefox\//.test(ua) ? 'Firefox'
    : /Chrom(?:e|ium)\//.test(ua) ? 'Chromium' : /Version\/.*Safari\//.test(ua) ? 'Safari' : null
  const clientSystem = /Android/.test(ua) ? 'Android' : /iPhone|iPad/.test(ua) ? 'iOS'
    : /Windows/.test(ua) ? 'Windows' : /Macintosh/.test(ua) ? 'macOS' : /Linux/.test(ua) ? 'Linux' : null
  return { browser, clientSystem }
}

export function captureReport(input: ReportInput): ReportSnapshot {
  const recordState = input.source === 'job' ? input.recordState || 'unavailable' : null
  const job = recordState === 'available' ? input.job : undefined
  const id = typeof job?.id === 'string' ? job.id : ''
  const jobId = boundedMatch(id, /^(?:docker|app|webenv):[a-f0-9]{32}$/, 40) || identifier(id)
  const failedStage = job?.stages?.slice(0, 32).find((stage) => failures.has(stage.status))
  // Project each field explicitly: do not spread/serialize errors, Job records, stage messages or logs.
  return Object.freeze({
    capturedAt: new Date().toISOString(), source: input.source, feature: input.feature, webVersion,
    ...clientCategories(), recordState,
    errorCode: code(input.error?.code) || code(job?.errorCode),
    httpStatus: Number.isInteger(input.error?.status) && input.error!.status! >= 0 && input.error!.status! <= 599
      ? input.error!.status! : null,
    requestId: identifier(input.error?.requestId), jobId,
    action: boundedMatch(job?.action, /^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*){1,4}$/, 96),
    jobStatus: job && statuses.has(job.status) ? job.status : null,
    failureStage: code(failedStage?.name),
  })
}

// Optional user-authored text is supplemental, never an automatic error/log attachment.
// Follow internal/redact's conservative treatment of incomplete/orphaned PEM blocks.
export function redactDescription(value: string): string {
  if (value.length > MAX_DESCRIPTION_LENGTH) return '[OMITTED: text exceeds limit]'
  const lines = value.replace(/\r\n?/g, '\n').split('\n')
  const result: string[] = []
  let inKey = false
  let uncertainStart = 0
  for (const line of lines) {
    const upper = line.toUpperCase()
    const begin = upper.indexOf('-----BEGIN ')
    const end = upper.indexOf('-----END ')
    const keyMarker = /-----[A-Z0-9 ]*PRIVATE KEY-----/i.test(line)
    if (keyMarker && end >= 0 && (begin < 0 || end < begin) && !inKey) {
      result.splice(uncertainStart)
      result.push('[REDACTED PRIVATE KEY]')
    } else if (keyMarker && !inKey) result.push('[REDACTED PRIVATE KEY]')
    if (keyMarker) {
      inKey = begin > end
      if (!inKey) uncertainStart = result.length
      continue
    }
    if (inKey) continue
    // Headers and assignment lines are withheld as a whole, including unknown value shapes.
    if (/(?:password|passwd|pwd|token|secret|api[_-]?key|access[_-]?key|authorization|cookie|credential|private[_-]?key)\s*["']?\s*[:=]|\bBearer\s|--?[\w-]*(?:password|token|secret|key)\s/i.test(line)) {
      result.push('[REDACTED SENSITIVE LINE]')
      continue
    }
    result.push(line.replace(/(?:[a-z][a-z0-9+.-]*:\/\/|www\.)[^\s<>"']+/gi, '[REDACTED URL]')
      .replace(/(?:[A-Z]:[\\/]|\\\\|\/(?:home|root|etc|var|tmp|mnt|opt|Users)\/)[^\s<>"']*/g, '[REDACTED PATH]')
      .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/g, ''))
  }
  return result.join('\n').trim()
}

export function buildReport(snapshot: ReportSnapshot, removed: readonly ReportField[], expected: string, actual: string): string {
  const report: Record<string, string | number | null> = { format: 'kpanel-local-problem-report/v1' }
  for (const field of REPORT_FIELDS) if (!removed.includes(field)) report[field] = snapshot[field]
  const expectedText = redactDescription(expected)
  const actualText = redactDescription(actual)
  if (expectedText) report.expected = expectedText
  if (actualText) report.actual = actualText
  return JSON.stringify(report, null, 2) + '\n'
}
