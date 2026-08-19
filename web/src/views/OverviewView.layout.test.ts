import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const source = readFileSync(new URL('./OverviewView.vue', import.meta.url), 'utf8')
const systemCenterSource = readFileSync(new URL('./SystemCenterView.vue', import.meta.url), 'utf8')
const styles = readFileSync(new URL('../styles/main.css', import.meta.url), 'utf8')
const desktopStyles = readFileSync(new URL('../styles/desktop.css', import.meta.url), 'utf8')

describe('OverviewView service status layout', () => {
  it('uses an explicit details wrapper instead of styling every service item span', () => {
    expect(source).toContain('<span class="service-item__details">')
    expect(styles).toMatch(/\.service-item__details\s*\{[^}]*display:\s*grid;/)
    expect(styles).not.toContain('.service-item > span:not(.service-item__icon)')
  })

  it('places the process manager beside monitoring history without adding sidebar navigation', () => {
    expect(source).toContain('class="realtime-monitoring__actions"')
    expect(source).toContain('to="/processes"')
    expect(source.indexOf('to="/processes"')).toBeLessThan(source.indexOf('to="/monitoring"'))
    expect(styles).toMatch(/\.realtime-monitoring__actions\s*\{[^}]*display:\s*flex;/)
  })

  it('keeps resource dialogs on the overview route and preserves the read-only DNS fact', () => {
    expect(source).toContain("title: '网络与流量'")
    expect(source).toContain('<dt>DNS 地址</dt>')
    expect(source).toContain('<HostsManagerDialog')
    expect(source).toContain('<CronManagerDialog')
    expect(source).toContain('<NetworkInterfacesDialog')
    expect(source).toContain('<FirewallManagerDialog')
  })

  it('gives the longer application summary its own full-width detail row', () => {
    expect(source).toContain('class="resource-summary__item resource-summary__item--stacked"')
    expect(styles).toMatch(
      /\.resource-summary__item--stacked\s*\{[^}]*grid-template-columns:\s*auto minmax\(0, 1fr\);/,
    )
    expect(styles).toMatch(
      /\.resource-summary__item--stacked > \.resource-summary__icon\s*\{[^}]*grid-row:\s*1 \/ span 2;/,
    )
    expect(styles).toMatch(
      /\.resource-summary__item--stacked > em\s*\{[^}]*grid-column:\s*2;[^}]*word-break:\s*keep-all;/,
    )
  })

  it('replaces overview management with six common tools while preserving the full system center', () => {
    expect(source).toContain("const overviewSystemToolIDs = ['swap', 'ssh-port', 'dns', 'ip-preference', 'bbr', 'system-tuning']")
    expect(source).toContain('<template v-if="props.systemCenterOnly">')
    expect(source).toContain('v-else')
    expect(source).toContain('class="panel-card overview-system-management"')
    expect(source).toContain('class="overview-system-grid"')
    expect(source).toContain('class="button button--secondary button--small overview-system-management__more"')
    expect(source).toContain('to="/system"')
    expect(source).toContain('@click="rememberViewportBeforeSystemCenter"')
    expect(source).toContain('ref="overviewSystemSection"')
    expect(source).toContain("overviewSystemSection.value?.scrollIntoView({ block: 'start' })")
    expect(source).not.toContain('class="system-center-entry"')
    expect(source).toContain('v-for="section in systemCenterSections"')
    expect(source).toContain("title: '日常维护'")
    expect(source).toContain("title: '基础配置'")
    expect(source).toContain("title: '登录与安全'")
    expect(source).toContain("title: '网络与流量'")
    expect(source).toContain("title: '性能优化'")
    expect(source).toContain('<h2 id="system-center-danger-title">危险操作</h2>')
    expect(source).toContain('v-if="tool.recommended" class="system-tool__recommend">推荐</span>')
    expect(source).not.toContain('<h2>基础系统设置</h2>')
    expect(source).not.toContain('<h2>网络工具</h2>')
    expect(source).toContain('<SystemTuningDialog')
    expect(systemCenterSource).toContain('<OverviewView system-center-only />')
    expect(styles).toMatch(/\.overview-system-grid\s*\{[^}]*grid-template-columns:\s*repeat\(3,/)
    expect(styles).toMatch(/\.system-center-grid\s*\{[^}]*grid-template-columns:\s*repeat\(4,/)
    expect(desktopStyles).toMatch(
      /@container desktop-window \(max-width: 980px\)[\s\S]*?\.desktop-window__body \.system-center-grid\s*\{[^}]*repeat\(3,/,
    )
  })
})
