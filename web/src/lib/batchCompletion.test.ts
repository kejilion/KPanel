import { describe, expect, it } from 'vitest'
import { batchExitCode, createBatchMarker, stripBatchWrapper, wrapBatchCommand } from './batchCompletion'

describe('batch completion marker', () => {
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
