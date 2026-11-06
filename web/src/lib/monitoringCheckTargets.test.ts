import { describe, expect, it } from 'vitest'
import { validMonitoringCheckTarget } from './monitoringCheckTargets'
import type { MonitoringCheckKind } from '@/types/api'

describe('monitoring check address formats', () => {
  const valid: Record<MonitoringCheckKind, string[]> = {
    ping: ['1.1.1.1', '2606:4700:4700::1111', 'example.com', 'localhost', '例子.测试', 'example.com.', 'fe80::1%eth0', 'my_service'],
    tcp: ['1.1.1.1:443', '[2606:4700:4700::1111]:443', 'example.com:443', 'localhost:80', '例子.测试:443', '[fe80::1%eth0]:443', 'my_service:80'],
    http: ['http://1.1.1.1/', 'http://[::1]:8080/health', 'https://example.com/health', 'http://localhost/', 'http://例子.测试/', 'http://[fe80::1%25eth0]/', 'http://my_service/'],
  }
  const invalid: Record<MonitoringCheckKind, string[]> = {
    ping: ['999.1.1.1', '127.1', '012.0.0.1', '[::1]', '2001:::1', 'example.com:80', 'https://example.com', 'a/b', '-bad.example', 'a..b', 'a;b', `${'a'.repeat(64)}.com`],
    tcp: ['::1:443', '[bad]:80', 'example.com:0', 'example.com:65536', 'example.com', 'bad/host:80', '999.1.1.1:80'],
    http: ['http://::1/', 'http://999.1.1.1/', 'http://127.1/', 'http://example.com:0/', 'http://example.com:65536/', 'http://example.com:/', 'https://user:secret@example.com/', 'ftp://example.com/', 'http://-bad.example/'],
  }
  for (const kind of ['ping', 'tcp', 'http'] as const) {
    it.each(valid[kind])(`accepts ${kind} %s`, (target) => expect(validMonitoringCheckTarget(kind, target)).toBe(true))
    it.each(invalid[kind])(`rejects ${kind} %s`, (target) => expect(validMonitoringCheckTarget(kind, target)).toBe(false))
  }
})
