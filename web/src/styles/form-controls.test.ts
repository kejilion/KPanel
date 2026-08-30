// @vitest-environment node
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const styles = readFileSync(new URL('./main.css', import.meta.url), 'utf8')
const desktopStyles = readFileSync(new URL('./desktop.css', import.meta.url), 'utf8')
const appsSource = readFileSync(new URL('../views/AppsView.vue', import.meta.url), 'utf8')
const clusterSource = readFileSync(new URL('../views/ClusterView.vue', import.meta.url), 'utf8')
const dockerSource = readFileSync(new URL('../views/DockerView.vue', import.meta.url), 'utf8')
const dockerEditorSource = readFileSync(new URL('../components/docker/DockerDeploymentEditor.vue', import.meta.url), 'utf8')
const filesSource = readFileSync(new URL('../views/FilesView.vue', import.meta.url), 'utf8')
const systemLogsSource = readFileSync(new URL('../components/overview/SystemLogsDialog.vue', import.meta.url), 'utf8')
const terminalSource = readFileSync(new URL('../views/TerminalView.vue', import.meta.url), 'utf8')
const processSource = readFileSync(new URL('../views/ProcessManagerView.vue', import.meta.url), 'utf8')

describe('form control focus contract', () => {
  it('keeps shared form controls on one compact focus ring', () => {
    expect(styles).toMatch(
      /\.field input:focus,[\s\S]*?\.select-field:focus-within\s*\{[\s\S]*?outline:\s*none;[\s\S]*?box-shadow:\s*0 0 0 2px color-mix\(in srgb, var\(--brand\) 16%, transparent\);/,
    )
    expect(styles).toMatch(/\.ai-composer:has\(textarea:focus-visible\) \{ outline: 2px solid var\(--brand\);/)
    expect(styles).toMatch(/\.search-field input:focus,\s*\.select-field select:focus\s*\{\s*outline: none;/)
  })

  it('keeps page-specific standard controls at the same focus weight', () => {
    expect(appsSource).toMatch(/\.market-search:focus-within\s*\{[^}]*box-shadow: 0 0 0 2px/)
    expect(clusterSource).toMatch(/\.cluster-search:focus-within\s*\{[^}]*box-shadow: 0 0 0 2px/)
    expect(desktopStyles).toMatch(/\.desktop__upload-location-form input:focus\s*\{[^}]*outline: none;[^}]*box-shadow: 0 0 0 2px/)
    expect(desktopStyles).toMatch(/\.desktop-shortcut-form__control:focus-within\s*\{[^}]*outline: 2px solid/)
    expect(dockerSource).toMatch(/\.text-input:focus,[\s\S]*?\.select-input:focus\s*\{[^}]*box-shadow: 0 0 0 2px/)
    expect(dockerEditorSource).toMatch(/\.deployment-editor__surface:focus-within\s*\{[^}]*box-shadow: 0 0 0 2px/)
    expect(filesSource).toMatch(/\.operation-form input:focus,[\s\S]*?\.operation-form select:focus\s*\{[^}]*box-shadow: 0 0 0 2px/)
    expect(systemLogsSource).toMatch(/\.system-log-control select:focus,[\s\S]*?outline: 2px solid/)
    expect(terminalSource).toMatch(/\.terminal-search input:focus\s*\{[^}]*box-shadow:0 0 0 2px/)
    expect(processSource).toMatch(/\.process-search:focus-within\s*\{[^}]*box-shadow: 0 0 0 2px/)
  })
})
