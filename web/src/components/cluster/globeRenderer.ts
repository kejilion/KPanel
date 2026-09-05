import { landDots, regionCenters } from './globeData'
import type { ClusterHost, PublicClusterShareHost } from '@/types/api'

export type GlobeHost = ClusterHost | PublicClusterShareHost

export function isPublicGlobeHost(host: GlobeHost): host is PublicClusterShareHost {
  return 'location' in host
}

export function globeHostLocation(host: GlobeHost) {
  return isPublicGlobeHost(host) ? host.location : host.lastSnapshot?.telemetry.publicNetwork
}

export interface GlobeRegion {
  code: string
  latitude: number
  longitude: number
  hosts: GlobeHost[]
}

export function groupGlobeHosts(hosts: readonly GlobeHost[]): GlobeRegion[] {
  const groups = new Map<string, GlobeRegion>()
  for (const host of hosts) {
    const code = globeHostLocation(host)?.countryCode?.trim().toUpperCase() || ''
    const center = Object.hasOwn(regionCenters, code) ? regionCenters[code] : undefined
    if (!center) continue
    let group = groups.get(code)
    if (!group) {
      group = { code, latitude: center[0], longitude: center[1], hosts: [] }
      groups.set(code, group)
    }
    group.hosts.push(host)
  }
  return [...groups.values()]
}

const radians = Math.PI / 180
type Vector = readonly [number, number, number]
function vector(latitude: number, longitude: number): Vector {
  const lat = latitude * radians, lon = longitude * radians
  return [Math.cos(lat) * Math.sin(lon), Math.sin(lat), Math.cos(lat) * Math.cos(lon)]
}
const land = landDots.map(([lat, lon]) => vector(lat, lon))
const grid: Vector[][] = []
for (let lat = -60; lat <= 60; lat += 30) {
  grid.push(Array.from({ length: 73 }, (_, i) => vector(lat, i * 5 - 180)))
}
for (let lon = -180; lon < 180; lon += 30) {
  grid.push(Array.from({ length: 37 }, (_, i) => vector(i * 5 - 90, lon)))
}

export function projectGlobePoint(latitude: number, longitude: number, viewLat: number, viewLon: number): Vector {
  return rotate(vector(latitude, longitude), camera(viewLat, viewLon))
}

function camera(lat: number, lon: number) {
  return [Math.sin(lat * radians), Math.cos(lat * radians), Math.sin(lon * radians), Math.cos(lon * radians)] as const
}
function rotate([x, y, z]: Vector, [slat, clat, slon, clon]: readonly number[]): Vector {
  const depth = x * slon! + z * clon!
  return [x * clon! - z * slon!, y * clat! - depth * slat!, y * slat! + depth * clat!]
}

export interface GlobePalette {
  brand: string
  land: string
  light: boolean
  text: string
  surface: string
  border: string
  success: string
  warning: string
}

export interface GlobeLabelAnchor {
  region: GlobeRegion
  x: number
  y: number
  selected: boolean
  depth?: number
  width: number
  text: string
}

export interface GlobeLabel extends GlobeLabelAnchor {
  left: number
  top: number
  height: number
}

/** A bounded, deterministic label pass. Geographic anchors never move. */
export function layoutGlobeLabels(anchors: GlobeLabelAnchor[], width: number, height: number): GlobeLabel[] {
  const labels: GlobeLabel[] = []
  const limit = Math.max(4, Math.min(16, Math.floor(width / 58)))
  const ordered = [...anchors].sort((a, b) => Number(b.selected) - Number(a.selected) || Math.floor((b.depth || 0) * 3) - Math.floor((a.depth || 0) * 3) || a.region.code.localeCompare(b.region.code))
  for (const anchor of ordered) {
    if (labels.length >= limit) break
    const boxHeight = 30
    const preferred = anchor.x < width / 2 ? 1 : -1
    const offsets = [0, -36, 36, -72, 72, -108, 108]
    let placed: GlobeLabel | undefined
    for (const dy of offsets) {
      for (const side of [preferred, -preferred]) {
        const left = anchor.x + (side > 0 ? 22 : -anchor.width - 22)
        const top = anchor.y - boxHeight / 2 + dy
        if (left < 10 || top < 10 || left + anchor.width > width - 10 || top + boxHeight > height - 10) continue
        if (labels.some(label => left < label.left + label.width + 6 && left + anchor.width + 6 > label.left && top < label.top + label.height + 6 && top + boxHeight + 6 > label.top)) continue
        if (anchors.some(point => point.x > left - 9 && point.x < left + anchor.width + 9 && point.y > top - 9 && point.y < top + boxHeight + 9)) continue
        placed = { ...anchor, left, top, height: boxHeight }
        break
      }
      if (placed) break
    }
    if (!placed && anchor.selected) {
      placed = { ...anchor, left: Math.max(10, Math.min(width - anchor.width - 10, anchor.x + 22)), top: Math.max(10, Math.min(height - boxHeight - 10, anchor.y - boxHeight / 2)), height: boxHeight }
    }
    if (placed) labels.push(placed)
  }
  return labels
}

/** Bounded Canvas 2D renderer. No DOM writes, network, timers or WebGL resources. */
export class GlobeRenderer {
  latitude = 18
  longitude = 105
  zoom = 1
  private width = 1
  private height = 1
  private radius = 1
  private regions: GlobeRegion[] = []
  private selected = ''
  private flags = new Map<string, CanvasImageSource>()
  private markers: { x: number; y: number; region: GlobeRegion }[] = []
  private labels: GlobeLabel[] = []
  private ocean?: CanvasGradient
  private atmosphere?: CanvasGradient

  constructor(private context: CanvasRenderingContext2D, private palette: GlobePalette) {}

  resize(width: number, height: number, dpr: number): void {
    this.width = width
    this.height = height
    this.radius = Math.min(width * 0.34, height * 0.39) * this.zoom
    const ratio = Math.max(1, Math.min(dpr || 1, 1.75))
    this.context.canvas.width = Math.round(width * ratio)
    this.context.canvas.height = Math.round(height * ratio)
    this.context.setTransform(ratio, 0, 0, ratio, 0, 0)
    this.setPalette(this.palette)
  }

  setPalette(palette: GlobePalette): void {
    this.palette = palette
    const ctx = this.context, x = this.width / 2, y = this.height / 2, r = this.radius
    this.ocean = ctx.createRadialGradient(x - r * .4, y - r * .4, 0, x, y, r)
    this.ocean.addColorStop(0, palette.surface)
    this.ocean.addColorStop(1, palette.border)
    this.atmosphere = ctx.createRadialGradient(x, y, r * .98, x, y, r * 1.12)
    this.atmosphere.addColorStop(0, palette.brand)
    this.atmosphere.addColorStop(1, 'transparent')
  }

  setHosts(regions: GlobeRegion[], selected: string): void {
    this.regions = regions
    this.selected = selected
    const codes = new Set(regions.map(region => region.code))
    for (const code of this.flags.keys()) {
      if (!codes.has(code)) this.flags.delete(code)
    }
  }

  setFlag(code: string, image: CanvasImageSource): void {
    const normalized = code.trim().toUpperCase()
    if (this.regions.some(region => region.code === normalized)) this.flags.set(normalized, image)
  }

  focus(region: GlobeRegion): void {
    this.latitude = region.latitude
    this.longitude = region.longitude
  }

  move(dx: number, dy: number): void {
    this.longitude = ((this.longitude + dx) % 360 + 360) % 360
    this.latitude = Math.max(-75, Math.min(75, this.latitude + dy))
  }

  setZoom(zoom: number): void {
    this.zoom = Math.max(1, Math.min(2, zoom))
    this.radius = Math.min(this.width * .34, this.height * .39) * this.zoom
    this.setPalette(this.palette)
  }

  hitTest(x: number, y: number): GlobeRegion | undefined {
    const label = this.labels.find(item => x >= item.left && x <= item.left + item.width && y >= item.top && y <= item.top + item.height)
    if (label) return label.region
    let closest: GlobeRegion | undefined, distance = 24 ** 2
    for (const marker of this.markers) {
      const next = (x - marker.x) ** 2 + (y - marker.y) ** 2
      if (next < distance) { closest = marker.region; distance = next }
    }
    return closest
  }

  draw(): void {
    const ctx = this.context, { width, height, radius: r, palette: p } = this
    const cx = width / 2, cy = height / 2, view = camera(this.latitude, this.longitude)
    ctx.clearRect(0, 0, width, height)
    ctx.globalAlpha = .13
    ctx.fillStyle = this.atmosphere || p.brand
    ctx.beginPath(); ctx.arc(cx, cy, r * 1.12, 0, Math.PI * 2); ctx.fill()
    ctx.globalAlpha = 1
    ctx.fillStyle = this.ocean || p.surface
    ctx.beginPath(); ctx.arc(cx, cy, r, 0, Math.PI * 2); ctx.fill()
    ctx.strokeStyle = p.brand
    ctx.globalAlpha = .16
    ctx.lineWidth = 1
    ctx.stroke()
    ctx.beginPath()
    for (const line of grid) {
      let connected = false
      for (const point of line) {
        const [x, y, z] = rotate(point, view)
        if (z < 0) { connected = false; continue }
        if (connected) ctx.lineTo(cx + x * r, cy - y * r)
        else ctx.moveTo(cx + x * r, cy - y * r)
        connected = true
      }
    }
    ctx.stroke()
    // Three depth buckets keep the land dots legible with just three fills.
    const projected = land.map(point => rotate(point, view))
    ctx.fillStyle = p.light ? p.land : p.brand
    for (let bucket = 0; bucket < 3; bucket++) {
      ctx.globalAlpha = p.light ? .58 + bucket * .20 : .32 + bucket * .23
      ctx.beginPath()
      for (const [x, y, z] of projected) {
        if (z <= 0 || Math.min(2, Math.floor(z * 3)) !== bucket) continue
        const size = Math.max(.75, r / 155) * (.65 + z * .35) * (p.light ? 1.15 : 1)
        ctx.moveTo(cx + x * r + size, cy - y * r)
        ctx.arc(cx + x * r, cy - y * r, size, 0, Math.PI * 2)
      }
      ctx.fill()
    }
    ctx.globalAlpha = .24
    ctx.beginPath()
    ctx.ellipse(cx, cy, r * 1.2, r * 1.06, -.3, .15, Math.PI * 1.55)
    ctx.stroke()
    ctx.globalAlpha = 1
    this.markers = []
    const anchors: GlobeLabelAnchor[] = []
    // Regions on the far hemisphere cannot be hit. Same-region nodes share a marker.
    for (const region of this.regions) {
      const [x, y, z] = projectGlobePoint(region.latitude, region.longitude, this.latitude, this.longitude)
      if (z < .06) continue
      const sx = cx + x * r, sy = cy - y * r
      if (sx < 10 || sx > width - 10 || sy < 10 || sy > height - 10) continue
      const selected = region.hosts.some(host => host.id === this.selected)
      const color = region.hosts.every(host => host.state === 'online') ? p.success : p.warning
      ctx.fillStyle = color
      ctx.globalAlpha = selected ? .24 : .14
      ctx.beginPath(); ctx.arc(sx, sy, selected ? 15 : 11, 0, Math.PI * 2); ctx.fill()
      ctx.globalAlpha = 1
      ctx.beginPath(); ctx.arc(sx, sy, selected ? 6 : 4, 0, Math.PI * 2); ctx.fill()
      ctx.strokeStyle = p.surface; ctx.lineWidth = 2; ctx.stroke()
      if (selected) {
        ctx.strokeStyle = p.brand; ctx.lineWidth = 1
        ctx.beginPath(); ctx.arc(sx, sy, 18, 0, Math.PI * 2); ctx.stroke()
      }
      const text = `${region.code}${region.hosts.length > 1 ? ` · ${region.hosts.length}` : ''}`
      anchors.push({ region, x: sx, y: sy, selected, depth: z, text, width: 48 + text.length * 7.5 })
      this.markers.push({ x: sx, y: sy, region })
    }
    this.labels = layoutGlobeLabels(anchors, width, height)
    ctx.font = '600 13px system-ui, sans-serif'
    ctx.textAlign = 'left'; ctx.textBaseline = 'middle'
    for (const label of this.labels) {
      const { left, top, width: labelWidth, height: labelHeight, region, selected } = label
      ctx.strokeStyle = p.brand; ctx.lineWidth = 1
      ctx.globalAlpha = selected ? .7 : .3
      ctx.beginPath(); ctx.moveTo(label.x, label.y)
      ctx.lineTo(Math.max(left, Math.min(left + labelWidth, label.x)), Math.max(top, Math.min(top + labelHeight, label.y))); ctx.stroke()
      ctx.globalAlpha = .96; ctx.fillStyle = p.surface
      ctx.beginPath(); ctx.roundRect(left, top, labelWidth, labelHeight, 8); ctx.fill()
      ctx.globalAlpha = 1; ctx.strokeStyle = selected ? p.brand : p.border; ctx.stroke()
      const flag = this.flags.get(region.code)
      if (flag) ctx.drawImage(flag, left + 7, top + 6, 18, 18)
      else {
        ctx.fillStyle = region.hosts.every(host => host.state === 'online') ? p.success : p.warning
        ctx.beginPath(); ctx.arc(left + 16, top + 15, 4, 0, Math.PI * 2); ctx.fill()
      }
      ctx.fillStyle = p.text; ctx.fillText(label.text, left + 32, top + 15)
    }
    ctx.globalAlpha = 1
  }
}
