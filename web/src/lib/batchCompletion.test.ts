import { describe, expect, it } from 'vitest'
import { execFileSync } from 'node:child_process'
import { batchExitCode, createBatchMarker, stripBatchWrapper, wrapBatchCommand } from './batchCompletion'

describe('batch completion marker', () => {
  it('does not mistake the echoed PowerShell wrapper for completion', () => {
    const marker = createBatchMarker()
    expect(batchExitCode(wrapBatchCommand('exit -1', marker, 'powershell'), marker, 'powershell')).toBeNull()
    expect(batchExitCode(`${marker}-1__`, marker, 'powershell')).toBe(-1)
    expect(batchExitCode(`${marker}2147483648__`, marker, 'powershell')).toBeNull()
  })

  it.runIf(process.platform === 'win32').each([
    ['Write-Output "中文测试" # trailing comment', 0, '中文测试'],
    ['Write-Output @\'\nhello "quotes"\n\'@', 0, 'hello "quotes"'],
    ['exit 7', 7, ''],
    ['exit -1', -1, ''],
    ['throw "expected failure"', 1, ''],
    ['Get-Item -LiteralPath "C:\\kpanel-no-such-file-test" -ErrorAction Stop', 1, ''],
  ])('executes Windows batch input with evaluated completion: %s', (command, status, output) => {
    const marker = createBatchMarker()
    const script = '[Console]::OutputEncoding=[Text.UTF8Encoding]::new(); ' + wrapBatchCommand(command, marker, 'powershell')
    const result = execFileSync(`${process.env.SystemRoot}\\System32\\WindowsPowerShell\\v1.0\\powershell.exe`, ['-NoProfile','-NonInteractive','-EncodedCommand',Buffer.from(script,'utf16le').toString('base64')], { encoding:'utf8', timeout:10000, stdio:['ignore','pipe','pipe'], windowsHide:true })
    expect(result).toContain(output)
    expect(batchExitCode(result, marker, 'powershell')).toBe(status)
  })
  it('creates unique unforgeable markers', () => {
    const first = createBatchMarker()
    expect(first).toMatch(/^__KPANEL_DONE_[0-9a-f]{32}_$/)
    expect(createBatchMarker()).not.toBe(first)
  })

  it('wraps the command so comments and background jobs still reach the marker', () => {
    const marker = '__KPANEL_DONE_00_'
    expect(wrapBatchCommand('echo hi # note\n', marker)).toBe(`{ echo hi # note\n}; printf '\\n${marker}%s__\\n' "$?"\r`)
    expect(wrapBatchCommand('sleep 5 &', marker).startsWith('{ sleep 5 &\n}')).toBe(true)
  })

  it('reads only the evaluated exit status, never the echoed format string', () => {
    const marker = '__KPANEL_DONE_ab_'
    expect(batchExitCode(`printf '\\n${marker}%s__\\n' "$?"`, marker)).toBeNull()
    expect(batchExitCode(`out\n${marker}0__\n`, marker)).toBe(0)
    expect(batchExitCode(`out\n${marker}127__\n`, marker)).toBe(127)
    expect(batchExitCode(`out\n${marker}999__\n`, marker)).toBeNull()
    expect(batchExitCode(`out\n__KPANEL_DONE_cd_0__\n`, marker)).toBeNull()
  })

  it('removes wrapper echo and marker lines from displayed output', () => {
    const marker = '__KPANEL_DONE_ab_'
    const output = `root@h:~# { df -h\n> }; printf '\\n${marker}%s__\\n' "$?"\nFilesystem Size\n\n${marker}0__\nroot@h:~#`
    expect(stripBatchWrapper(output, 'df -h', marker)).toBe('root@h:~# df -h\nFilesystem Size\n\nroot@h:~#')
  })
})

describe('batch wrapper stripping edge cases', () => {
  it('leaves output untouched when the command starts with a blank line', () => {
    const marker = '__KPANEL_DONE_ab_'
    expect(stripBatchWrapper('{ "json": true }\nok', '\necho ok', marker)).toBe('{ "json": true }\nok')
  })
})

describe('batch wrapper stripping repeats', () => {
  it('strips the group prefix from every echoed copy of the command', () => {
    const marker = '__KPANEL_DONE_ab_'
    const output = `uptime\nroot@h:~# { uptime\n> }; printf '\n${marker}%s__\n' "$?"\n up 13 min\n${marker}0__\nroot@h:~#`
    const stripped = stripBatchWrapper(output, 'uptime', marker)
    expect(stripped).not.toContain('{ uptime')
    expect(stripped).toContain('root@h:~# uptime')
    expect(stripped).toContain('up 13 min')
  })
})
