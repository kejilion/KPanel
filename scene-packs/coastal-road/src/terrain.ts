import * as THREE from 'three'
import { mergeGeometries } from 'three/examples/jsm/utils/BufferGeometryUtils.js'
import { NOISE_GLSL } from './noise'
import { LIGHTING_GLSL, type LightingUniforms } from './shading'
import { coastX, groundHeight, ROCKS, roadX, WORLD_Z } from './world'

/**
 * The land: a mesh laid out along the coastline (dense across the cliff face,
 * sparse far inland and far offshore), shaded as layered sandstone cliffs, dry
 * grass and scrub, sand in the coves and dark wet rock at the waterline; pines,
 * cypresses and bushes on the hills; and sea stacks and reefs offshore.
 */
const TERRAIN_VERTEX = /* glsl */ `
varying vec3 vWorld;
varying vec3 vNormal;
void main() {
  vec4 world = modelMatrix * vec4(position, 1.0);
  vWorld = world.xyz;
  vNormal = normalize(mat3(modelMatrix) * normal);
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const TERRAIN_FRAGMENT = /* glsl */ `
varying vec3 vWorld;
varying vec3 vNormal;
${NOISE_GLSL}
${LIGHTING_GLSL}
void main() {
  vec3 n = normalize(vNormal);
  float slope = 1.0 - n.y;
  float y = vWorld.y;
  // Sandstone: irregular layers, weathering streaks running down, darker cracks.
  float warp = snoise(vWorld * 0.012) * 4.0 + snoise(vWorld * 0.004) * 7.0;
  float strata = sin(y * 0.38 + warp) * 0.5 + 0.5;
  // Broad streaks, with the fine ones only up close (from afar they break up into dots).
  float streaks = snoise(vec3(vWorld.x * 0.09, y * 0.012, vWorld.z * 0.09)) * 0.5 + 0.5;
  streaks = clamp(streaks + snoise(vec3(vWorld.x * 0.35, y * 0.018, vWorld.z * 0.35)) * 0.25 * smoothstep(260.0, 60.0, length(cameraPosition - vWorld)), 0.0, 1.0);
  // Vertical fissures rather than spots.
  float cracks = smoothstep(0.66, 0.86, abs(snoise(vec3(vWorld.x * 0.16, y * 0.01, vWorld.z * 0.16))));
  vec3 rock = mix(vec3(0.46, 0.35, 0.24), vec3(0.66, 0.52, 0.37), strata * 0.55 + streaks * 0.45);
  rock *= mix(0.8, 1.08, snoise(vWorld * 0.05) * 0.5 + 0.5) * mix(1.0, 0.72, cracks);
  float patches = snoise(vec3(vWorld.xz * 0.011, 1.0)) * 0.5 + 0.5;
  float fine = snoise(vec3(vWorld.xz * 0.08, 2.0)) * 0.5 + 0.5;
  vec3 grass = mix(vec3(0.34, 0.4, 0.16), vec3(0.12, 0.24, 0.07), smoothstep(0.3, 0.72, patches)) * mix(0.78, 1.14, fine);
  // Darker scrub in clumps (warped and broken up, so they read as scrub rather than round pits)
  // and a scatter of wildflowers.
  vec2 scrubAt = vWorld.xz * 0.045 + vec2(snoise(vec3(vWorld.xz * 0.018, 11.0)), snoise(vec3(vWorld.xz * 0.018, 12.0))) * 1.6;
  float scrubNoise = snoise(vec3(scrubAt, 5.0)) * 0.65 + snoise(vec3(vWorld.xz * 0.16, 6.0)) * 0.35;
  float scrub = smoothstep(0.3, 0.7, scrubNoise * 0.5 + 0.5);
  grass = mix(grass, vec3(0.1, 0.15, 0.06), scrub * 0.5);
  // Sun-dried patches, and up close tufts and clumps so the verge is not a flat carpet.
  float dry = smoothstep(0.58, 0.8, snoise(vec3(vWorld.xz * 0.02, 9.0)) * 0.5 + 0.5);
  grass = mix(grass, vec3(0.42, 0.37, 0.19), dry * 0.45);
  float near = smoothstep(90.0, 12.0, length(cameraPosition - vWorld));
  float tuft = snoise(vec3(vWorld.xz * 1.7, 3.0)) * 0.5 + 0.5;
  float clump = snoise(vec3(vWorld.xz * 0.45, 4.0)) * 0.5 + 0.5;
  grass *= mix(1.0, mix(0.6, 1.2, tuft * 0.5 + clump * 0.5), near);
  float flowers = step(0.965, snoise(vec3(vWorld.xz * 0.7, 7.0)) * 0.5 + 0.5);
  grass = mix(grass, mix(vec3(0.85, 0.75, 0.2), vec3(0.7, 0.35, 0.6), step(0.5, fract(vWorld.x * 0.13))), flowers * 0.8);
  vec3 sand = vec3(0.8, 0.7, 0.52) * mix(0.92, 1.05, fine);
  vec3 albedo = mix(rock, grass, smoothstep(0.34, 0.16, slope));
  albedo = mix(albedo, sand, smoothstep(5.0, 1.8, y) * smoothstep(0.4, 0.15, slope));
  // Wet and darker at the waterline; the seabed is sand fading into the deep.
  albedo *= mix(0.42, 1.0, smoothstep(-0.4, 1.6, y));
  if (y < 0.0) albedo = mix(sand * 0.75, vec3(0.08, 0.11, 0.1), smoothstep(-1.0, -14.0, y));
  float shadow = keyShadow(vWorld + n * 1.2);
  vec3 color = shade(albedo, n, 0.1, shadow);
  gl_FragColor = vec4(atmosphere(color, vWorld), 1.0);
}
`

const PAINTED_VERTEX = /* glsl */ `
varying vec3 vWorld;
varying vec3 vNormal;
varying vec3 vColor;
void main() {
  vec4 world = vec4(position, 1.0);
  vec3 n = normal;
  #ifdef USE_INSTANCING
    world = instanceMatrix * world;
    n = mat3(instanceMatrix) * n;
  #endif
  world = modelMatrix * world;
  vWorld = world.xyz;
  vNormal = normalize(mat3(modelMatrix) * n);
  vColor = color;
  #ifdef USE_INSTANCING_COLOR
    vColor *= instanceColor;
  #endif
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const PAINTED_FRAGMENT = /* glsl */ `
uniform float uWrap;
varying vec3 vWorld;
varying vec3 vNormal;
varying vec3 vColor;
${LIGHTING_GLSL}
void main() {
  vec3 n = normalize(vNormal);
  float shadow = keyShadow(vWorld + n * 1.5);
  vec3 color = shade(vColor, n, uWrap, shadow);
  gl_FragColor = vec4(atmosphere(color, vWorld), 1.0);
}
`

const ROCK_FRAGMENT = /* glsl */ `
varying vec3 vWorld;
varying vec3 vNormal;
varying vec3 vColor;
${NOISE_GLSL}
${LIGHTING_GLSL}
void main() {
  // Faceted, layered sandstone like the cliffs they broke from: flat facets, fine bumps on top.
  vec3 n = normalize(normalize(vNormal) + vec3(snoise(vWorld * 0.7), snoise(vWorld * 0.7 + 7.0), snoise(vWorld * 0.7 + 13.0)) * 0.12
    + vec3(snoise(vWorld * 2.1), snoise(vWorld * 2.1 + 3.0), snoise(vWorld * 2.1 + 9.0)) * 0.06);
  float strata = sin(vWorld.y * 1.3 + snoise(vWorld * 0.05) * 2.0) * 0.5 + 0.5;
  float grain = snoise(vWorld * 0.6) * 0.5 + 0.5;
  vec3 albedo = mix(vec3(0.4, 0.3, 0.21), vec3(0.64, 0.51, 0.37), strata * 0.5 + grain * 0.5) * mix(0.8, 1.06, snoise(vWorld * 0.12) * 0.5 + 0.5);
  float cracks = smoothstep(0.62, 0.86, abs(snoise(vec3(vWorld.x * 0.3, vWorld.y * 0.05, vWorld.z * 0.3))));
  albedo *= mix(1.0, 0.6, cracks);
  // A dark band of weed and wet rock where the waves wash.
  float wet = 1.0 - smoothstep(0.2, 2.8, vWorld.y + snoise(vWorld * 0.3) * 0.6);
  albedo = mix(albedo, vec3(0.07, 0.08, 0.05), wet * 0.85);
  // Pale salt and guano on the ledges and tops.
  albedo = mix(albedo, vec3(0.8, 0.78, 0.72), smoothstep(0.8, 0.97, n.y) * 0.45);
  float shadow = keyShadow(vWorld + n * 1.0);
  gl_FragColor = vec4(atmosphere(shade(albedo, n, 0.15, shadow), vWorld), 1.0);
}
`

/** Offsets across the coast, dense over the cliff face, the shallows and wherever the road runs. */
function crossSamples(): number[] {
  const values: number[] = []
  const band = (from: number, to: number, step: number) => { for (let d = from; d < to; d += step) values.push(d) }
  band(-700, -80, 35)
  band(-80, -4, 4)
  band(-4, 34, 1.6)
  // The road runs 55-130 m in from the edge (furthest across the headland's neck); its bench needs 4 m.
  band(34, 150, 4)
  band(150, 420, 12)
  band(420, 1500, 40)
  values.push(1500)
  return values
}

export function colored(geometry: THREE.BufferGeometry, color: THREE.Color): THREE.BufferGeometry {
  const flat = geometry.index ? geometry.toNonIndexed() : geometry
  const count = flat.getAttribute('position').count
  const colors = new Float32Array(count * 3)
  for (let index = 0; index < count; index++) colors.set([color.r, color.g, color.b], index * 3)
  flat.setAttribute('color', new THREE.BufferAttribute(colors, 3))
  return flat
}

export function paintedMaterial(uniforms: LightingUniforms, wrap: number, fragmentShader = PAINTED_FRAGMENT): THREE.ShaderMaterial {
  return new THREE.ShaderMaterial({
    uniforms: { ...uniforms, uWrap: { value: wrap } },
    vertexShader: PAINTED_VERTEX,
    fragmentShader,
    vertexColors: true,
  })
}

function terrainMesh(uniforms: LightingUniforms): THREE.Mesh {
  const across = crossSamples()
  const rows = Math.round((WORLD_Z * 2) / 6)
  const positions = new Float32Array((rows + 1) * across.length * 3)
  let k = 0
  for (let row = 0; row <= rows; row++) {
    const z = -WORLD_Z + (row / rows) * WORLD_Z * 2
    const coast = coastX(z)
    for (const d of across) {
      const x = coast + d
      positions[k++] = x
      positions[k++] = groundHeight(x, z)
      positions[k++] = z
    }
  }
  const indices: number[] = []
  const columns = across.length
  for (let row = 0; row < rows; row++) {
    for (let column = 0; column < columns - 1; column++) {
      const a = row * columns + column
      const b = a + columns
      indices.push(a, b, a + 1, a + 1, b, b + 1)
    }
  }
  const geometry = new THREE.BufferGeometry()
  geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3))
  geometry.setIndex(indices)
  geometry.computeVertexNormals()
  const mesh = new THREE.Mesh(geometry, new THREE.ShaderMaterial({ uniforms: { ...uniforms }, vertexShader: TERRAIN_VERTEX, fragmentShader: TERRAIN_FRAGMENT }))
  mesh.frustumCulled = false
  return mesh
}

function vegetation(uniforms: LightingUniforms, random: () => number): THREE.InstancedMesh[] {
  const trunk = new THREE.Color().setRGB(0.22, 0.15, 0.1)
  const pineGreen = new THREE.Color().setRGB(0.12, 0.22, 0.08)
  const cypressGreen = new THREE.Color().setRGB(0.07, 0.15, 0.06)
  const shrubGreen = new THREE.Color().setRGB(0.2, 0.26, 0.1)
  const pinePart = (x: number, y: number, z: number, r: number, squash: number) => {
    const puff = new THREE.IcosahedronGeometry(r, 1)
    puff.scale(1, squash, 1)
    puff.translate(x, y, z)
    return colored(puff, pineGreen)
  }
  const pineTrunk = new THREE.CylinderGeometry(0.035, 0.06, 0.8, 6)
  pineTrunk.translate(0, 0.4, 0)
  const pine = mergeGeometries([colored(pineTrunk, trunk), pinePart(0, 0.85, 0, 0.36, 0.38), pinePart(0.2, 0.8, 0.1, 0.24, 0.4), pinePart(-0.18, 0.82, -0.12, 0.26, 0.4)])!
  const cypressBody = new THREE.IcosahedronGeometry(0.5, 2)
  cypressBody.scale(0.22, 1, 0.22)
  cypressBody.translate(0, 0.55, 0)
  const cypress = mergeGeometries([colored(cypressBody, cypressGreen)])!
  const shrubBody = new THREE.IcosahedronGeometry(0.5, 1)
  shrubBody.scale(1, 0.6, 1)
  shrubBody.translate(0, 0.25, 0)
  const shrub = mergeGeometries([colored(shrubBody, shrubGreen)])!
  const kinds = [
    { geometry: pine, count: 360, size: [11, 17] },
    { geometry: cypress, count: 180, size: [11, 18] },
    { geometry: shrub, count: 700, size: [2.5, 5] },
  ] as const
  const material = paintedMaterial(uniforms, 0.5)
  return kinds.map((kind) => {
    const mesh = new THREE.InstancedMesh(kind.geometry, material, kind.count)
    const matrix = new THREE.Matrix4()
    const tint = new THREE.Color()
    let placed = 0
    for (let tries = 0; tries < kind.count * 20 && placed < kind.count; tries++) {
      const z = (random() * 2 - 1) * (WORLD_Z - 200)
      const d = 40 + random() ** 1.5 * 900
      const x = coastX(z) + d
      if (Math.abs(x - roadX(z)) < 13) continue
      // Groves: thin out where a slow noise says so.
      if (Math.sin(x * 0.011) * Math.sin(z * 0.008) + Math.sin(z * 0.023 + x * 0.004) * 0.5 < -0.2 && random() < 0.8) continue
      const y = groundHeight(x, z)
      const size = kind.size[0] + random() * (kind.size[1] - kind.size[0])
      matrix.compose(new THREE.Vector3(x, y - 0.3, z), new THREE.Quaternion().setFromAxisAngle(new THREE.Vector3(0, 1, 0), random() * 6.3), new THREE.Vector3(size * (0.85 + random() * 0.3), size, size * (0.85 + random() * 0.3)))
      mesh.setMatrixAt(placed, matrix)
      mesh.setColorAt(placed, tint.setRGB(0.85 + random() * 0.35, 0.85 + random() * 0.3, 0.8 + random() * 0.3))
      placed++
    }
    mesh.count = placed
    mesh.computeBoundingSphere()
    mesh.frustumCulled = false
    return mesh
  })
}

function rocksMesh(uniforms: LightingUniforms): THREE.Mesh {
  const parts: THREE.BufferGeometry[] = []
  const gray = new THREE.Color(1, 1, 1)
  for (const rock of ROCKS) {
    // Not merged: every face keeps its own normal, so the stone breaks into flat facets.
    const geometry = new THREE.IcosahedronGeometry(1, 3).deleteAttribute('normal').deleteAttribute('uv')
    const position = geometry.getAttribute('position')
    const v = new THREE.Vector3()
    for (let index = 0; index < position.count; index++) {
      v.fromBufferAttribute(position, index)
      const s = rock.seed
      // Jagged, weathered stone: several octaves of lumps and a few sharp facets.
      const lump = 1 + 0.26 * Math.sin(v.x * 3.1 + s) * Math.sin(v.y * 2.7 + s * 1.3) * Math.sin(v.z * 3.3 + s * 0.7)
        + 0.12 * Math.sin(v.x * 7.3 + s * 2) * Math.sin(v.z * 6.1 + s) + 0.06 * Math.sin(v.y * 13 + v.x * 5 + s)
        - 0.1 * Math.abs(Math.sin(v.x * 4.7 + v.z * 3.9 + s * 3))
      // Sea stacks: steep sides stepped into ledges along the layers, a rounded top, a wide foot under the water.
      const top = Math.max(0, v.y)
      const layer = v.y * 3.4 + Math.sin(v.x * 2.3 + s) * 0.45 + Math.sin(v.z * 3.1 + s * 1.7) * 0.3
      const ledge = v.y > -0.2 ? 1 - 0.055 * (layer - Math.floor(layer)) : 1
      v.set(v.x * lump * ledge * (1 - top * 0.25), v.y > 0 ? Math.pow(v.y, 0.7) : v.y * 0.5, v.z * lump * ledge * (1 - top * 0.25))
      position.setXYZ(index, v.x * rock.radius, v.y * rock.height + (v.y > 0 ? 0 : 0), v.z * rock.radius)
    }
    geometry.computeVertexNormals()
    geometry.translate(rock.x, -rock.height * 0.12, rock.z)
    parts.push(colored(geometry, gray))
  }
  const mesh = new THREE.Mesh(mergeGeometries(parts)!, paintedMaterial(uniforms, 0.15, ROCK_FRAGMENT))
  mesh.frustumCulled = false
  return mesh
}

export function createTerrain(uniforms: LightingUniforms, random: () => number): THREE.Group {
  const group = new THREE.Group()
  group.add(terrainMesh(uniforms))
  for (const mesh of vegetation(uniforms, random)) group.add(mesh)
  group.add(rocksMesh(uniforms))
  return group
}
