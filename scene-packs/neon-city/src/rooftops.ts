import * as THREE from 'three'
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js'
import { FOG_GLSL, type AtmosphereUniforms } from './atmosphere'
import { type Building, mulberry32 } from './layout'

/**
 * What stands on the roofs: a parapet round every roof, and the clutter real
 * roofs carry (air-conditioning units, cooling towers, water tanks, stair
 * housings, masts, dishes and vents), modelled in Blender (see
 * blender/rooftops.py) and scattered by seed, so the skyline has a broken,
 * busy top instead of clean box lids.
 */
const VERTEX = /* glsl */ `
attribute vec3 color;
varying vec3 vColor;
varying vec3 vWorld;
varying vec3 vNormal;
void main() {
  vColor = color;
  vec4 world = modelMatrix * instanceMatrix * vec4(position, 1.0);
  vWorld = world.xyz;
  vNormal = normalize(mat3(modelMatrix) * mat3(instanceMatrix) * normal);
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const FRAGMENT = /* glsl */ `
uniform float uAmbient;
uniform float uSun;
uniform vec3 uSkyTop;
uniform vec3 uHorizon;
uniform vec3 uSunDir;
uniform vec3 uSunColor;
varying vec3 vColor;
varying vec3 vWorld;
varying vec3 vNormal;
${FOG_GLSL}
void main() {
  vec3 n = normalize(vNormal);
  // Lit like the facades: the sky from above, the afterglow of the set sun, and at night the
  // faint glow of the city below on the undersides.
  vec3 sky = mix(uHorizon * 0.6, uSkyTop, n.y * 0.5 + 0.5);
  vec3 light = 0.012 + sky * uAmbient * 1.4;
  light += uSunColor * max(dot(n, normalize(vec3(uSunDir.x, 0.0, uSunDir.z))), 0.0) * uSun * 0.55;
  light += uHorizon * 0.25 * max(-n.y, 0.0);
  gl_FragColor = vec4(applyFog(vColor * light, vWorld), 1.0);
}
`

interface Kind {
  name: string
  /** Footprint half-size, m, for spacing. */
  radius: number
  weight: number
  /** Allowed on roofs whose top is at least this high, and whose sides are at least this long. */
  minTop: number
  minSide: number
  /** At most this many per roof. */
  most: number
}

const KINDS: readonly Kind[] = [
  { name: 'hvac', radius: 2.0, weight: 30, minTop: 0, minSide: 8, most: 6 },
  { name: 'vents', radius: 1.3, weight: 20, minTop: 0, minSide: 8, most: 4 },
  { name: 'housing', radius: 2.6, weight: 14, minTop: 0, minSide: 12, most: 1 },
  { name: 'tank', radius: 2.2, weight: 10, minTop: 0, minSide: 12, most: 2 },
  { name: 'chiller', radius: 4.2, weight: 10, minTop: 40, minSide: 16, most: 2 },
  { name: 'dish', radius: 1.4, weight: 8, minTop: 30, minSide: 8, most: 2 },
  { name: 'antenna', radius: 1.3, weight: 5, minTop: 120, minSide: 8, most: 1 },
]

export async function createRooftops(buildings: readonly Building[], uniforms: AtmosphereUniforms): Promise<THREE.Group> {
  const gltf = await new GLTFLoader().loadAsync('assets/rooftops.glb')
  const material = new THREE.ShaderMaterial({
    uniforms: {
      uAmbient: uniforms.uAmbient,
      uSun: uniforms.uSun,
      uSkyTop: uniforms.uSkyTop,
      uHorizon: uniforms.uHorizon,
      uSunDir: uniforms.uSunDir,
      uSunColor: uniforms.uSunColor,
      uFogColor: uniforms.uFogColor,
      uFogDensity: uniforms.uFogDensity,
    },
    vertexShader: VERTEX,
    fragmentShader: FRAGMENT,
  })
  const random = mulberry32(20260925)
  const placements = new Map<string, THREE.Matrix4[]>()
  KINDS.forEach((kind) => placements.set(kind.name, []))
  const parapets: THREE.Matrix4[] = []

  // A roof is the top of a building nothing else stands on (setback tiers and spires do).
  const covered = new Set(buildings.filter((b) => b.y > 0).map((b) => `${b.x.toFixed(2)},${b.z.toFixed(2)},${b.y.toFixed(2)}`))
  const matrix = new THREE.Matrix4()
  const turn = new THREE.Quaternion()
  const up = new THREE.Vector3(0, 1, 0)
  for (const building of buildings) {
    const top = building.y + building.h
    if (building.w < 6 || !building.street && building.y === 0) continue // spires and the far ring
    if (covered.has(`${building.x.toFixed(2)},${building.z.toFixed(2)},${top.toFixed(2)}`)) continue
    // Parapet: four low walls round the edge.
    const rim = 1.1
    const thick = 0.35
    for (const [x, z, w, d] of [
      [building.x, building.z - building.d / 2 + thick / 2, building.w, thick],
      [building.x, building.z + building.d / 2 - thick / 2, building.w, thick],
      [building.x - building.w / 2 + thick / 2, building.z, thick, building.d],
      [building.x + building.w / 2 - thick / 2, building.z, thick, building.d],
    ] as const) {
      parapets.push(new THREE.Matrix4().compose(new THREE.Vector3(x, top + rim / 2, z), turn.identity(), new THREE.Vector3(w, rim, d)))
    }
    // Clutter, spaced so nothing overlaps, kept clear of the parapet.
    const count = THREE.MathUtils.clamp(Math.floor((building.w * building.d) / 220), 1, 7)
    const placed: { x: number, z: number, r: number }[] = []
    const used = new Map<string, number>()
    for (let attempt = 0; attempt < count * 6 && placed.length < count; attempt++) {
      const allowed = KINDS.filter((kind) => top >= kind.minTop && Math.min(building.w, building.d) >= kind.minSide && (used.get(kind.name) ?? 0) < kind.most)
      if (!allowed.length) break
      let pick = random() * allowed.reduce((sum, kind) => sum + kind.weight, 0)
      const kind = allowed.find((candidate) => (pick -= candidate.weight) <= 0) ?? allowed[0]!
      const margin = kind.radius + 1.2
      if (building.w < margin * 2 || building.d < margin * 2) continue
      const x = building.x + (random() - 0.5) * (building.w - margin * 2)
      const z = building.z + (random() - 0.5) * (building.d - margin * 2)
      if (placed.some((other) => Math.hypot(other.x - x, other.z - z) < other.r + kind.radius + 0.8)) continue
      placed.push({ x, z, r: kind.radius })
      used.set(kind.name, (used.get(kind.name) ?? 0) + 1)
      turn.setFromAxisAngle(up, Math.floor(random() * 4) * (Math.PI / 2))
      placements.get(kind.name)!.push(matrix.clone().compose(new THREE.Vector3(x, top, z), turn.clone(), new THREE.Vector3(1, 1, 1)))
    }
  }

  const group = new THREE.Group()
  const instanced = (geometry: THREE.BufferGeometry, matrices: THREE.Matrix4[]) => {
    const mesh = new THREE.InstancedMesh(geometry, material, matrices.length)
    matrices.forEach((each, index) => mesh.setMatrixAt(index, each))
    mesh.instanceMatrix.needsUpdate = true
    mesh.computeBoundingSphere()
    group.add(mesh)
  }
  for (const kind of KINDS) {
    const source = gltf.scene.getObjectByName(kind.name) as THREE.Mesh | undefined
    if (!source || !placements.get(kind.name)!.length) continue
    // The model is authored z up in Blender and exported y up; bake its node transform in.
    source.updateWorldMatrix(true, false)
    const geometry = source.geometry.clone().applyMatrix4(source.matrixWorld)
    instanced(geometry, placements.get(kind.name)!)
  }
  const wall = new THREE.BoxGeometry(1, 1, 1)
  const concrete = new Float32Array(wall.getAttribute('position').count * 3).fill(0.22)
  wall.setAttribute('color', new THREE.BufferAttribute(concrete, 3))
  instanced(wall, parapets)
  return group
}
