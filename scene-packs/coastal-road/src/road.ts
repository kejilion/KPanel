import * as THREE from 'three'
import { mergeGeometries } from 'three/examples/jsm/utils/BufferGeometryUtils.js'
import { NOISE_GLSL } from './noise'
import { LIGHTING_GLSL, type LightingUniforms } from './shading'
import { colored, paintedMaterial } from './terrain'

/**
 * The road: asphalt with a dashed centre line and solid edge lines, a steel
 * guardrail on the sea side, and street lamps on the land side whose pools of
 * light come up on the tarmac at dusk.
 */
export const ROAD_HALF_WIDTH = 4.6
export const LAMP_SPACING = 42

const ROAD_VERTEX = /* glsl */ `
attribute vec2 aRoad; // metres across (+ towards the sea), metres along
varying vec3 vWorld;
varying vec2 vRoad;
void main() {
  vRoad = aRoad;
  vec4 world = modelMatrix * vec4(position, 1.0);
  vWorld = world.xyz;
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const ROAD_FRAGMENT = /* glsl */ `
varying vec3 vWorld;
varying vec2 vRoad;
${NOISE_GLSL}
${LIGHTING_GLSL}
void main() {
  float across = vRoad.x;
  float along = vRoad.y;
  vec3 asphalt = vec3(0.075, 0.075, 0.08) * mix(0.8, 1.2, snoise(vec3(vWorld.xz * 0.6, 1.0)) * 0.5 + 0.5);
  float fw = fwidth(across) * 1.5;
  float centre = (1.0 - smoothstep(0.08, 0.08 + fw, abs(across))) * step(0.55, fract(along / 12.0));
  float edges = 1.0 - smoothstep(0.07, 0.07 + fw, abs(abs(across) - 3.9));
  vec3 albedo = mix(asphalt, vec3(0.8, 0.78, 0.7), clamp(centre + edges, 0.0, 1.0) * 0.85);
  vec3 n = vec3(0.0, 1.0, 0.0);
  vec3 color = shade(albedo, n, 0.0, keyShadow(vWorld + vec3(0.0, 0.6, 0.0)));
  // Pools of warm light under the lamps (on the land side) once it gets dark.
  // Lamps stand every LAMP_SPACING metres; their bulbs hang over the land-side lane.
  float lamp = fract(along / ${LAMP_SPACING.toFixed(1)} + 0.5) - 0.5;
  float pool = exp(-(pow(lamp * ${LAMP_SPACING.toFixed(1)}, 2.0) + pow(across + 4.1, 2.0) * 0.6) / 40.0);
  color += vec3(1.0, 0.72, 0.42) * pool * 0.16 * uNight;
  gl_FragColor = vec4(atmosphere(color, vWorld), 1.0);
}
`

const GLOW_VERTEX = /* glsl */ `
uniform float uPointScale;
uniform float uNight;
attribute vec3 aGlow; // colour
attribute float aSize;
varying vec3 vGlow;
void main() {
  vec4 view = viewMatrix * modelMatrix * vec4(position, 1.0);
  float size = aSize * uPointScale / max(-view.z, 1.0);
  gl_PointSize = clamp(size, 1.5, 36.0);
  vGlow = aGlow * clamp(size / 1.5, 0.3, 1.0);
  gl_Position = projectionMatrix * view;
}
`

const GLOW_FRAGMENT = /* glsl */ `
varying vec3 vGlow;
void main() {
  vec2 p = gl_PointCoord - 0.5;
  gl_FragColor = vec4(vGlow * exp(-dot(p, p) * 16.0), 1.0);
}
`

/** Additive glowing points; colours are multiplied by the night factor where lights are switched off by day. */
export function glowPoints(uniforms: LightingUniforms, count: number): { points: THREE.Points, positions: Float32Array, colors: Float32Array, sizes: Float32Array, update(): void } {
  const positions = new Float32Array(count * 3)
  const colors = new Float32Array(count * 3)
  const sizes = new Float32Array(count)
  const geometry = new THREE.BufferGeometry()
  const position = new THREE.BufferAttribute(positions, 3)
  const color = new THREE.BufferAttribute(colors, 3)
  position.setUsage(THREE.DynamicDrawUsage)
  color.setUsage(THREE.DynamicDrawUsage)
  geometry.setAttribute('position', position)
  geometry.setAttribute('aGlow', color)
  geometry.setAttribute('aSize', new THREE.BufferAttribute(sizes, 1))
  const points = new THREE.Points(geometry, new THREE.ShaderMaterial({
    uniforms: { uPointScale: uniforms.uPointScale, uNight: uniforms.uNight },
    vertexShader: GLOW_VERTEX,
    fragmentShader: GLOW_FRAGMENT,
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  }))
  points.frustumCulled = false
  return {
    points,
    positions,
    colors,
    sizes,
    update() {
      position.needsUpdate = true
      color.needsUpdate = true
    },
  }
}

export function createRoad(uniforms: LightingUniforms, curve: THREE.CatmullRomCurve3): THREE.Group & { update(night: number): void } {
  const group = new THREE.Group()
  const length = curve.getLength()
  const count = Math.ceil(length / 4)
  const up = new THREE.Vector3(0, 1, 0)
  const point = new THREE.Vector3()
  const tangent = new THREE.Vector3()
  const sea = new THREE.Vector3()

  // Tarmac.
  const positions: number[] = []
  const road: number[] = []
  const indices: number[] = []
  // Guardrail band on the sea side.
  const rail: number[] = []
  const railIndex: number[] = []
  const posts: THREE.Matrix4[] = []
  const lamps: THREE.Vector3[] = []
  const lampSides: THREE.Vector3[] = []
  for (let index = 0; index <= count; index++) {
    const u = index / count
    curve.getPointAt(u, point)
    curve.getTangentAt(u, tangent)
    sea.crossVectors(tangent, up).normalize()
    const along = u * length
    for (const side of [-1, 1]) {
      const edge = point.clone().addScaledVector(sea, side * ROAD_HALF_WIDTH)
      positions.push(edge.x, point.y + 0.08, edge.z)
      road.push(side * ROAD_HALF_WIDTH, along)
    }
    const railBase = point.clone().addScaledVector(sea, ROAD_HALF_WIDTH + 0.7)
    rail.push(railBase.x, point.y + 0.55, railBase.z, railBase.x, point.y + 0.9, railBase.z)
    if (index > 0) {
      const a = (index - 1) * 2
      indices.push(a, a + 2, a + 1, a + 1, a + 2, a + 3)
      railIndex.push(a, a + 2, a + 1, a + 1, a + 2, a + 3)
    }
    posts.push(new THREE.Matrix4().compose(new THREE.Vector3(railBase.x, point.y, railBase.z), new THREE.Quaternion(), new THREE.Vector3(0.14, 0.9, 0.14)))
    if (Math.abs(along % LAMP_SPACING) < 2 || LAMP_SPACING - (along % LAMP_SPACING) < 2) {
      lamps.push(point.clone().addScaledVector(sea, -(ROAD_HALF_WIDTH + 1.2)))
      lampSides.push(sea.clone())
    }
  }
  const tarmac = new THREE.BufferGeometry()
  tarmac.setAttribute('position', new THREE.Float32BufferAttribute(positions, 3))
  tarmac.setAttribute('aRoad', new THREE.Float32BufferAttribute(road, 2))
  tarmac.setIndex(indices)
  const tarmacMesh = new THREE.Mesh(tarmac, new THREE.ShaderMaterial({ uniforms: { ...uniforms }, vertexShader: ROAD_VERTEX, fragmentShader: ROAD_FRAGMENT, side: THREE.DoubleSide }))
  tarmacMesh.frustumCulled = false
  group.add(tarmacMesh)

  const steel = new THREE.Color().setRGB(0.55, 0.56, 0.56)
  const railGeometry = new THREE.BufferGeometry()
  railGeometry.setAttribute('position', new THREE.Float32BufferAttribute(rail, 3))
  railGeometry.setIndex(railIndex)
  railGeometry.computeVertexNormals()
  const railMesh = new THREE.Mesh(colored(railGeometry, steel), paintedMaterial(uniforms, 0.6))
  railMesh.material.side = THREE.DoubleSide
  railMesh.frustumCulled = false
  group.add(railMesh)
  const postBox = new THREE.BoxGeometry(1, 1, 1)
  postBox.translate(0, 0.5, 0)
  const postMesh = new THREE.InstancedMesh(colored(postBox, steel), paintedMaterial(uniforms, 0.4), posts.length)
  posts.forEach((matrix, index) => postMesh.setMatrixAt(index, matrix))
  postMesh.frustumCulled = false
  group.add(postMesh)

  // Street lamps: a pole with an arm reaching over the road, and the lamp's glow at night.
  const dark = new THREE.Color().setRGB(0.2, 0.2, 0.21)
  const pole = new THREE.CylinderGeometry(0.09, 0.12, 7.5, 6)
  pole.translate(0, 3.75, 0)
  const arm = new THREE.BoxGeometry(0.1, 0.1, 1.8)
  arm.translate(0, 7.45, 0.9)
  const head = new THREE.BoxGeometry(0.35, 0.18, 0.7)
  head.translate(0, 7.35, 1.7)
  const lampMesh = new THREE.InstancedMesh(mergeGeometries([colored(pole, dark), colored(arm, dark), colored(head, dark)])!, paintedMaterial(uniforms, 0.3), lamps.length)
  const glow = glowPoints(uniforms, lamps.length)
  lamps.forEach((base, index) => {
    const facing = Math.atan2(lampSides[index]!.x, lampSides[index]!.z)
    lampMesh.setMatrixAt(index, new THREE.Matrix4().compose(base, new THREE.Quaternion().setFromAxisAngle(up, facing), new THREE.Vector3(1, 1, 1)))
    const bulb = base.clone().addScaledVector(lampSides[index]!, 1.7).add(new THREE.Vector3(0, 7.2, 0))
    glow.positions.set([bulb.x, bulb.y, bulb.z], index * 3)
    glow.sizes[index] = 1.6
  })
  lampMesh.frustumCulled = false
  group.add(lampMesh)
  group.add(glow.points)
  const warm = new THREE.Color().setRGB(1, 0.7, 0.4)
  return Object.assign(group, {
    update(night: number) {
      for (let index = 0; index < lamps.length; index++) glow.colors.set([warm.r * 2.4 * night, warm.g * 2.4 * night, warm.b * 2.4 * night], index * 3)
      glow.update()
      glow.points.visible = night > 0.01
    },
  })
}
