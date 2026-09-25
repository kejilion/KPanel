import * as THREE from 'three'
import { FOG_GLSL, HASH_GLSL, type AtmosphereUniforms } from './atmosphere'
import { AVENUE_X, PITCH, ROAD_CENTERS, roadWidth, type Building } from './layout'

// Generic shop words only: no brands, no real businesses.
const WORDS = ['拉面', '旅馆', '电玩', '夜市', '咖啡', '书店', '酒吧', '茶楼']
const CELLS = WORDS.length
const FONT = '"PingFang SC", "Microsoft YaHei", "Noto Sans CJK SC", "Source Han Sans SC", sans-serif'

/** One canvas holds every vertical sign: a dark board with white neon tubes, tinted per sign in the shader. */
function signAtlas(): THREE.CanvasTexture {
  const canvas = document.createElement('canvas')
  canvas.width = 128 * CELLS
  canvas.height = 512
  const context = canvas.getContext('2d')!
  context.textAlign = 'center'
  context.textBaseline = 'middle'
  WORDS.forEach((word, index) => {
    const x = index * 128
    context.fillStyle = 'rgb(10, 10, 14)'
    context.beginPath()
    context.roundRect(x + 6, 6, 116, 500, 14)
    context.fill()
    context.strokeStyle = '#fff'
    context.lineWidth = 5
    context.beginPath()
    context.roundRect(x + 18, 18, 92, 476, 10)
    context.stroke()
    const glyphs = [...word]
    const step = 440 / glyphs.length
    context.font = `bold ${Math.min(96, step * 0.82)}px ${FONT}`
    glyphs.forEach((glyph, row) => {
      const y = 36 + step * (row + 0.5)
      context.lineWidth = 5
      context.strokeText(glyph, x + 64, y)
      context.globalAlpha = 0.35
      context.fillStyle = '#fff'
      context.fillText(glyph, x + 64, y)
      context.globalAlpha = 1
    })
  })
  const texture = new THREE.CanvasTexture(canvas)
  texture.colorSpace = THREE.SRGBColorSpace
  texture.anisotropy = 4
  return texture
}

const SIGN_VERTEX = /* glsl */ `
attribute vec4 aSign; // atlas cell (or screen mode), palette index, seed, unused
varying vec2 vUv;
varying vec4 vSign;
varying vec3 vWorld;
void main() {
  vec4 world = modelMatrix * instanceMatrix * vec4(position, 1.0);
  vWorld = world.xyz;
  vUv = uv;
  vSign = aSign;
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const SIGN_FRAGMENT = /* glsl */ `
uniform sampler2D uAtlas;
uniform vec3 uNeonColors[6];
uniform float uNeon;
uniform float uTime;
varying vec2 vUv;
varying vec4 vSign;
varying vec3 vWorld;
${HASH_GLSL}
${FOG_GLSL}
void main() {
  vec2 uv = vUv;
  if (!gl_FrontFacing) uv.x = 1.0 - uv.x;
  vec4 texel = texture2D(uAtlas, vec2((vSign.x + uv.x) / ${CELLS.toFixed(1)}, uv.y));
  if (texel.a < 0.5) discard;
  // Neon hums: a slow, slight swell in brightness, different for every sign. Nothing flickers.
  float hum = 0.9 + 0.1 * sin(uTime * (0.35 + vSign.z * 0.4) + vSign.z * 40.0);
  vec3 tube = uNeonColors[int(vSign.y)] * smoothstep(0.35, 0.9, texel.r) * 2.4 * uNeon * hum;
  vec3 color = vec3(0.01, 0.01, 0.014) + tube;
  gl_FragColor = vec4(applyFog(color, vWorld), 1.0);
}
`

const SCREEN_FRAGMENT = /* glsl */ `
uniform vec3 uNeonColors[6];
uniform float uNeon;
uniform float uTime;
varying vec2 vUv;
varying vec4 vSign;
varying vec3 vWorld;
${HASH_GLSL}
${FOG_GLSL}
vec3 palette(float t, vec3 a, vec3 b) { return mix(a, b, 0.5 + 0.5 * sin(t)); }
void main() {
  vec2 uv = vUv;
  float t = uTime * 0.12 + vSign.z * 10.0;
  vec3 a = uNeonColors[int(vSign.y)];
  vec3 b = uNeonColors[int(mod(vSign.y + 1.0, 6.0))];
  float mode = vSign.x;
  vec3 content;
  if (mode < 0.5) {
    // Slow flowing bands.
    float wave = sin(uv.y * 5.0 + t * 1.7 + sin(uv.x * 3.0 + t) * 1.5);
    content = palette(wave * 2.0 + t, a, b) * (0.35 + 0.65 * smoothstep(-0.6, 1.0, wave));
  } else if (mode < 1.5) {
    // Rings breathing out from the centre.
    float r = length((uv - 0.5) * vec2(1.78, 1.0));
    float ring = 0.5 + 0.5 * sin(r * 10.0 - t * 2.0);
    content = palette(r * 3.0 - t, a, b) * (0.45 + 0.55 * ring) * (1.0 - smoothstep(0.2, 0.95, r));
  } else {
    // Soft drifting shapes.
    vec2 p = uv * vec2(1.78, 1.0);
    float field = 0.0;
    for (int i = 0; i < 4; i++) {
      float fi = float(i);
      vec2 c = vec2(0.89 + 0.6 * sin(t * (0.5 + fi * 0.13) + fi * 2.1), 0.5 + 0.32 * cos(t * (0.4 + fi * 0.11) + fi));
      field += 0.06 / (dot(p - c, p - c) + 0.02);
    }
    content = mix(a, b, smoothstep(0.8, 3.0, field)) * smoothstep(0.6, 2.2, field);
  }
  // LED dot grid up close, averaged away with distance.
  vec2 grid = uv * vec2(192.0, 108.0);
  vec2 fw = fwidth(grid);
  float dots = mix(smoothstep(0.5, 0.2, length(fract(grid) - 0.5)), 0.55, smoothstep(0.25, 0.7, max(fw.x, fw.y)));
  vec3 color = content * dots * 1.0 * uNeon + vec3(0.004);
  gl_FragColor = vec4(applyFog(color, vWorld), 1.0);
}
`

function signUniforms(uniforms: AtmosphereUniforms) {
  return {
    uNeonColors: uniforms.uNeonColors,
    uNeon: uniforms.uNeon,
    uTime: uniforms.uTime,
    uFogColor: uniforms.uFogColor,
    uFogDensity: uniforms.uFogDensity,
  }
}

/** Faces of a street building that look onto a road, with the outward normal. */
function streetFaces(building: Building): { nx: number, nz: number, avenue: boolean }[] {
  const faces: { nx: number, nz: number, avenue: boolean }[] = []
  for (const [nx, nz] of [[1, 0], [-1, 0], [0, 1], [0, -1]] as const) {
    const face = nx ? building.x + (nx * building.w) / 2 : building.z + (nz * building.d) / 2
    const alongZ = nx !== 0
    const road = ROAD_CENTERS.reduce((best, center) => (Math.abs(center - face) < Math.abs(best - face) ? center : best), ROAD_CENTERS[0]!)
    const gap = Math.abs(road - face) - roadWidth(road, alongZ) / 2
    if ((road - face) * (nx || nz) > 0 && gap < 5) faces.push({ nx, nz, avenue: alongZ && Math.abs(road - AVENUE_X) < 1 })
  }
  return faces
}

export function createSigns(buildings: readonly Building[], landmarks: readonly Building[], uniforms: AtmosphereUniforms, random: () => number): THREE.Group {
  const group = new THREE.Group()
  const matrix = new THREE.Matrix4()
  const rotation = new THREE.Quaternion()
  const up = new THREE.Vector3(0, 1, 0)

  // Vertical signs sticking out over the pavement.
  const signs: { position: THREE.Vector3, angle: number, height: number, data: number[] }[] = []
  for (const building of buildings) {
    if (!building.street || building.h < 16) continue
    for (const face of streetFaces(building)) {
      const count = face.avenue ? 2 + Math.floor(random() * 3) : random() < 0.4 ? 1 : 0
      for (let index = 0; index < count; index++) {
        const span = (face.nx ? building.d : building.w) - 5
        if (span < 3) break
        const along = (random() - 0.5) * span
        const height = 6 + random() * 5
        const y = 5.5 + height / 2 + random() * Math.min(20, building.h - height - 8)
        const out = 1.7
        const position = face.nx
          ? new THREE.Vector3(building.x + face.nx * (building.w / 2 + out), y, building.z + along)
          : new THREE.Vector3(building.x + along, y, building.z + face.nz * (building.d / 2 + out))
        // A sign sticking out along x lies in the xy plane; along z it is turned a quarter.
        signs.push({ position, angle: face.nx ? 0 : Math.PI / 2, height, data: [Math.floor(random() * CELLS), Math.floor(random() * 6), random(), 0] })
      }
    }
  }
  const signGeometry = new THREE.PlaneGeometry(1, 1)
  const signData = new Float32Array(signs.length * 4)
  signs.forEach((sign, index) => signData.set(sign.data, index * 4))
  signGeometry.setAttribute('aSign', new THREE.InstancedBufferAttribute(signData, 4))
  const signMaterial = new THREE.ShaderMaterial({
    uniforms: { ...signUniforms(uniforms), uAtlas: { value: signAtlas() } },
    vertexShader: SIGN_VERTEX,
    fragmentShader: SIGN_FRAGMENT,
    side: THREE.DoubleSide,
  })
  const signMesh = new THREE.InstancedMesh(signGeometry, signMaterial, signs.length)
  signs.forEach((sign, index) => {
    rotation.setFromAxisAngle(up, sign.angle)
    signMesh.setMatrixAt(index, matrix.compose(sign.position, rotation, new THREE.Vector3(2.3, sign.height, 1)))
  })
  signMesh.instanceMatrix.needsUpdate = true
  signMesh.computeBoundingSphere()
  group.add(signMesh)

  // Big screens: on the tallest towers facing the cameras, and a few over the avenue.
  const screens: { position: THREE.Vector3, angle: number, width: number, data: number[] }[] = []
  for (const tower of landmarks.slice(0, 7)) {
    const width = Math.min(tower.w * 0.8, 46)
    if (width < 18) continue
    // Keep the whole 16:9 screen on the facade, clear of the ground floors and the crown.
    const height = width * 0.5625
    screens.push({
      position: new THREE.Vector3(tower.x, tower.y + Math.max(24 + height / 2, Math.min(tower.h * (0.42 + random() * 0.22), tower.h - height / 2 - 12)), tower.z + tower.d / 2 + 0.5),
      angle: 0,
      width,
      data: [Math.floor(random() * 3), Math.floor(random() * 6), random(), 0],
    })
  }
  const avenueFaces = buildings.filter((building) => building.street && building.h > 40 && Math.abs(building.z) < 2.2 * PITCH && building.z < 60)
  for (const building of avenueFaces) {
    const face = streetFaces(building).find((candidate) => candidate.avenue)
    if (!face || random() > 0.5 || building.d < 22) continue
    const width = Math.min(building.d * 0.7, 24)
    screens.push({
      position: new THREE.Vector3(building.x + face.nx * (building.w / 2 + 0.5), 18 + random() * 14, building.z),
      angle: face.nx > 0 ? Math.PI / 2 : -Math.PI / 2,
      width,
      data: [Math.floor(random() * 3), Math.floor(random() * 6), random(), 0],
    })
  }
  const screenGeometry = new THREE.PlaneGeometry(1, 1)
  const screenData = new Float32Array(screens.length * 4)
  screens.forEach((screen, index) => screenData.set(screen.data, index * 4))
  screenGeometry.setAttribute('aSign', new THREE.InstancedBufferAttribute(screenData, 4))
  const screenMesh = new THREE.InstancedMesh(screenGeometry, new THREE.ShaderMaterial({
    uniforms: signUniforms(uniforms),
    vertexShader: SIGN_VERTEX,
    fragmentShader: SCREEN_FRAGMENT,
  }), screens.length)
  screens.forEach((screen, index) => {
    rotation.setFromAxisAngle(up, screen.angle)
    screenMesh.setMatrixAt(index, matrix.compose(screen.position, rotation, new THREE.Vector3(screen.width, screen.width * 0.5625, 1)))
  })
  screenMesh.instanceMatrix.needsUpdate = true
  screenMesh.computeBoundingSphere()
  group.add(screenMesh)
  return group
}
