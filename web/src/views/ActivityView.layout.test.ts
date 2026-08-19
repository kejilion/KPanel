import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const source = readFileSync(new URL('./ActivityView.vue', import.meta.url), 'utf8')
const jobsSource = readFileSync(new URL('./JobsView.vue', import.meta.url), 'utf8')
const styles = readFileSync(new URL('../styles/main.css', import.meta.url), 'utf8')

describe('activity page layout', () => {
  it('does not stretch the tab row inside a desktop window', () => {
    expect(source).toMatch(/\.activity-page\s*\{[^}]*align-content:\s*start;/)
  })

  it('keeps task stage badges inside the activity detail dialog', () => {
    expect(jobsSource).toContain('<span class="stage-list__marker" />')
    expect(styles).toMatch(/\.stage-list__marker\s*\{[^}]*width:\s*8px;[^}]*height:\s*8px;/)
    expect(styles).not.toContain('.stage-list li > span {')
  })
})
