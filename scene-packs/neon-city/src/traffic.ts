import * as THREE from 'three'
import { mergeGeometries } from 'three/examples/jsm/utils/BufferGeometryUtils.js'
import { FOG_GLSL, type AtmosphereUniforms } from './atmosphere'
import { AVENUE_X, EXTENT, HIGHWAYS, ROAD, ROAD_CENTERS, roadWidth } from './layout'

/**
 * Everything that moves on the ground: street traffic on every lane, the two
 * elevated highways with their own traffic, and the street lamps. Cars are
 * animated entirely on the GPU from per-car lane data, so thousands cost one
 * draw call each for lights and bodies.
 */
const PATH_SAMPLES = 1024

// Each car carries two camera-facing lamp sprites just outside its body: headlights
// in front, tail lights behind, each as bright as its end faces the camera.
const LAMP_COMMON = /* glsl */ `
uniform float uTime;
uniform float uPixelAngle;
attribute float aEnd; // +1 headlights, -1 tail lights
varying vec2 vUv;
varying vec3 vColor;
varying float vPair;
varying vec3 vWorld;
void lamps(vec3 center, vec3 dir) {
  vec3 lamp = center + dir * aEnd * 2.35 + vec3(0.0, 0.72, 0.0);
  vec3 toCam = cameraPosition - lamp;
  float dist = length(toCam);
  toCam /= dist;
  vec3 side = normalize(cross(dir, vec3(0.0, 1.0, 0.0)));
  vec3 across = side - toCam * dot(side, toCam);
  float acrossLength = length(across);
  vec3 camRight = vec3(viewMatrix[0][0], viewMatrix[1][0], viewMatrix[2][0]);
  across = acrossLength > 0.05 ? across / acrossLength : camRight;
  vec3 up = normalize(cross(toCam, across));
  float px = uPixelAngle * dist;
  // Far lamps stay a few pixels wide so streets and highways read as streams of light.
  float width = max(2.0, px * 3.4);
  float height = max(0.9, px * 3.0);
  float gain = clamp(sqrt((2.0 * 0.9) / (width * height)), 0.2, 1.0);
  float visible = smoothstep(-0.2, 0.5, dot(dir, toCam) * aEnd);
  vColor = (aEnd > 0.0 ? vec3(1.0, 0.86, 0.68) * 3.2 : vec3(1.0, 0.05, 0.02) * 2.8) * visible * gain;
  // Up close the two lamps separate; far away they merge into one dot.
  vPair = (1.0 - smoothstep(0.2, 0.7, px)) * acrossLength;
  vec3 pos = lamp + across * (position.x * width) + up * (position.y * height);
  vUv = uv;
  vWorld = pos;
  gl_Position = projectionMatrix * viewMatrix * vec4(pos, 1.0);
}
`

const LAMP_FRAGMENT = /* glsl */ `
varying vec2 vUv;
varying vec3 vColor;
varying float vPair;
varying vec3 vWorld;
${FOG_GLSL}
void main() {
  vec2 p = vUv - 0.5;
  float blob = exp(-(p.x * p.x * 14.0 + p.y * p.y * 40.0));
  float pair = exp(-(pow((abs(p.x) - 0.3) * 10.0, 2.0) + pow(p.y * 8.0, 2.0)));
  gl_FragColor = vec4(vColor * mix(blob, pair, vPair) * (1.0 - fogAmount(vWorld)), 1.0);
}
`

const STREET_CAR = /* glsl */ `
attribute vec4 aLane; // origin x, origin z, direction x, direction z
attribute vec4 aMove; // lane length, speed, offset, seed
vec3 carCenter(out vec3 dir) {
  float s = mod(aMove.z + uTime * aMove.y, aMove.x);
  dir = vec3(aLane.z, 0.0, aLane.w);
  return vec3(aLane.x, 0.0, aLane.y) + dir * s;
}
`

const HIGHWAY_CAR = /* glsl */ `
uniform sampler2D uPath;
attribute vec4 aPath; // path row, lateral offset, direction sign, seed
attribute vec4 aMove; // path length, speed, offset, unused
vec3 pathAt(float row, float u) {
  float x = clamp(u, 0.0, 1.0) * ${(PATH_SAMPLES - 1).toFixed(1)};
  float i0 = floor(x);
  vec3 a = texelFetch(uPath, ivec2(int(i0), int(row)), 0).xyz;
  vec3 b = texelFetch(uPath, ivec2(int(min(i0 + 1.0, ${(PATH_SAMPLES - 1).toFixed(1)})), int(row)), 0).xyz;
  return mix(a, b, x - i0);
}
vec3 carCenter(out vec3 dir) {
  float s = mod(aMove.z + uTime * aMove.y, aMove.x) / aMove.x;
  float u = aPath.z > 0.0 ? s : 1.0 - s;
  vec3 p = pathAt(aPath.x, u);
  vec3 t = normalize(pathAt(aPath.x + 1.0, u));
  vec3 side = normalize(cross(t, vec3(0.0, 1.0, 0.0)));
  dir = t * aPath.z;
  return p + side * aPath.y;
}
`

const BODY_VERTEX = /* glsl */ `
varying vec3 vWorld;
varying vec3 vNormal;
varying float vSeed;
void body(vec3 center, vec3 dir, float seed) {
  vec3 side = vec3(-dir.z, 0.0, dir.x);
  vec3 pos = center + dir * position.x + vec3(0.0, position.y, 0.0) + side * position.z;
  vNormal = dir * normal.x + vec3(0.0, normal.y, 0.0) + side * normal.z;
  vWorld = pos;
  vSeed = seed;
  gl_Position = projectionMatrix * viewMatrix * vec4(pos, 1.0);
}
`

const BODY_FRAGMENT = /* glsl */ `
uniform vec3 uSkyTop;
uniform vec3 uHorizon;
uniform vec3 uGlow;
uniform float uAmbient;
varying vec3 vWorld;
varying vec3 vNormal;
varying float vSeed;
${FOG_GLSL}
void main() {
  vec3 n = normalize(vNormal);
  vec3 paint = vSeed < 0.35 ? vec3(0.006) : vSeed < 0.55 ? vec3(0.045, 0.006, 0.008) : vSeed < 0.75 ? vec3(0.008, 0.016, 0.04) : vec3(0.07, 0.07, 0.075);
  vec3 v = normalize(cameraPosition - vWorld);
  float fresnel = pow(1.0 - max(dot(n, v), 0.0), 4.0);
  vec3 env = mix(uHorizon * 0.4 + uGlow * 0.6, uSkyTop, n.y * 0.5 + 0.5);
  vec3 color = paint * (0.05 + env * (0.6 + uAmbient * 3.0)) + env * fresnel * 0.25;
  gl_FragColor = vec4(applyFog(color, vWorld), 1.0);
}
`

const RAIL_VERTEX = /* glsl */ `
uniform float uPixelAngle;
attribute vec3 aTangent;
attribute float aSide;
varying float vGain;
varying vec3 vWorld;
void main() {
  vec3 toCam = normalize(cameraPosition - position);
  vec3 perp = normalize(cross(aTangent, toCam));
  float px = uPixelAngle * length(cameraPosition - position);
  float width = max(0.35, px * 1.6);
  vGain = clamp(0.35 / width, 0.12, 1.0);
  vec3 pos = position + perp * aSide * width * 0.5;
  vWorld = pos;
  gl_Position = projectionMatrix * viewMatrix * vec4(pos, 1.0);
}
`

const RAIL_FRAGMENT = /* glsl */ `
uniform vec3 uColor;
uniform float uNeon;
varying float vGain;
varying vec3 vWorld;
${FOG_GLSL}
void main() {
  gl_FragColor = vec4(uColor * 1.1 * vGain * (0.5 + 0.5 * uNeon) * (1.0 - fogAmount(vWorld)), 1.0);
}
`

const CONCRETE_VERTEX = /* glsl */ `
varying vec3 vWorld;
varying vec3 vNormal;
varying vec2 vUv;
void main() {
  vec4 world = modelMatrix * instanceMatrix * vec4(position, 1.0);
  vWorld = world.xyz;
  vNormal = normalize(mat3(modelMatrix) * mat3(instanceMatrix) * normal);
  vUv = uv;
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const CONCRETE_FRAGMENT = /* glsl */ `
uniform vec3 uSkyTop;
uniform vec3 uHorizon;
uniform vec3 uGlow;
uniform float uAmbient;
uniform float uLamps;
uniform float uDeck;
varying vec3 vWorld;
varying vec3 vNormal;
varying vec2 vUv;
${FOG_GLSL}
void main() {
  vec3 n = normalize(vNormal);
  vec3 base = vec3(0.03, 0.03, 0.034);
  vec3 env = mix(uHorizon * 0.5 + uGlow * 0.6, uSkyTop, n.y * 0.5 + 0.5);
  vec3 color = base * (0.2 + env * uAmbient * 6.0);
  // City light bouncing up onto the undersides.
  color += base * uGlow * 2.0 * max(-n.y, 0.0);
  if (uDeck > 0.5 && n.y > 0.5) {
    // Deck surface: lamp pools and faint lane dashes.
    float pool = exp(-pow((fract(vUv.x / 36.0) - 0.5) * 36.0 / 9.0, 2.0));
    color += vec3(1.0, 0.62, 0.3) * pool * 0.05 * uLamps;
    float lane = abs(fract(vUv.y * 3.0) - 0.5);
    float dash = step(0.5, fract(vUv.x / 9.0));
    color += vec3(0.05) * (1.0 - smoothstep(0.02, 0.04, lane)) * dash * uLamps;
  }
  gl_FragColor = vec4(applyFog(color, vWorld), 1.0);
}
`

const STREET_LAMP_VERTEX = /* glsl */ `
uniform float uPointScale;
attribute vec3 aColor;
varying vec3 vColor;
varying float vGain;
varying vec3 vWorld;
void main() {
  vec4 view = viewMatrix * vec4(position, 1.0);
  float size = 1.6 * uPointScale / max(-view.z, 1.0);
  gl_PointSize = clamp(size, 2.2, 42.0);
  vGain = clamp(size / 2.2, 0.12, 1.0);
  vColor = aColor;
  vWorld = position;
  gl_Position = projectionMatrix * view;
}
`

const STREET_LAMP_FRAGMENT = /* glsl */ `
uniform float uLamps;
varying vec3 vColor;
varying float vGain;
varying vec3 vWorld;
${FOG_GLSL}
void main() {
  vec2 p = gl_PointCoord - 0.5;
  float glow = exp(-dot(p, p) * 22.0);
  gl_FragColor = vec4(vColor * glow * 2.6 * vGain * uLamps * (1.0 - fogAmount(vWorld)), 1.0);
}
`

/** Two small quads per car: aEnd +1 for the headlights, -1 for the tail lights. */
function lampQuads(): THREE.BufferGeometry {
  const geometry = new THREE.BufferGeometry()
  const positions: number[] = []
  const uvs: number[] = []
  const ends: number[] = []
  const indices: number[] = []
  for (const end of [1, -1]) {
    const base = positions.length / 3
    for (const [x, y] of [[-0.5, -0.5], [0.5, -0.5], [0.5, 0.5], [-0.5, 0.5]] as const) {
      positions.push(x, y, 0)
      uvs.push(x + 0.5, y + 0.5)
      ends.push(end)
    }
    indices.push(base, base + 1, base + 2, base, base + 2, base + 3)
  }
  geometry.setAttribute('position', new THREE.Float32BufferAttribute(positions, 3))
  geometry.setAttribute('uv', new THREE.Float32BufferAttribute(uvs, 2))
  geometry.setAttribute('aEnd', new THREE.Float32BufferAttribute(ends, 1))
  geometry.setIndex(indices)
  return geometry
}

/** Shares a plain geometry's vertices with a per-car instanced geometry. */
function instanced(source: THREE.BufferGeometry, count: number): THREE.InstancedBufferGeometry {
  const geometry = new THREE.InstancedBufferGeometry()
  geometry.setIndex(source.index)
  for (const [name, attribute] of Object.entries(source.attributes)) geometry.setAttribute(name, attribute)
  geometry.instanceCount = count
  return geometry
}

function additive(material: THREE.ShaderMaterial): THREE.ShaderMaterial {
  material.transparent = true
  material.depthWrite = false
  material.blending = THREE.AdditiveBlending
  return material
}

function fog(uniforms: AtmosphereUniforms) {
  return { uFogColor: uniforms.uFogColor, uFogDensity: uniforms.uFogDensity }
}

function light(uniforms: AtmosphereUniforms) {
  return { uSkyTop: uniforms.uSkyTop, uHorizon: uniforms.uHorizon, uGlow: uniforms.uGlow, uAmbient: uniforms.uAmbient, ...fog(uniforms) }
}

function streetLanes(random: () => number): { lane: number[], move: number[] }[] {
  const cars: { lane: number[], move: number[] }[] = []
  const length = EXTENT * 2
  for (const alongZ of [true, false]) {
    for (const center of ROAD_CENTERS) {
      const avenue = alongZ && Math.abs(center - AVENUE_X) < 1
      const lanesPerSide = avenue ? 3 : 2
      for (let lane = 0; lane < lanesPerSide; lane++) {
        const offset = 2.1 + lane * 3.4
        for (const sign of [1, -1]) {
          // Right-hand traffic: heading +z (or +x) keeps to the lower side of the road.
          const heading = sign
          const lateral = center - sign * offset * (alongZ ? 1 : -1)
          const ox = alongZ ? lateral : -EXTENT * heading
          const oz = alongZ ? -EXTENT * heading : lateral
          const dx = alongZ ? 0 : heading
          const dz = alongZ ? heading : 0
          // Night traffic: a car now and then, not a queue.
          const spacing = avenue ? 130 + random() * 190 : 70 + random() * 120
          const speed = avenue ? 11 + random() * 4 : 8 + random() * 8
          for (let s = random() * spacing; s < length; s += spacing * (0.75 + random() * 0.5)) {
            cars.push({ lane: [ox, oz, dx, dz], move: [length, speed, s, random()] })
          }
        }
      }
    }
  }
  return cars
}

function pathTexture(): { texture: THREE.DataTexture, lengths: number[] } {
  const data = new Float32Array(PATH_SAMPLES * HIGHWAYS.length * 2 * 4)
  const lengths: number[] = []
  const tangent = new THREE.Vector3()
  HIGHWAYS.forEach((highway, index) => {
    const points = highway.curve.getSpacedPoints(PATH_SAMPLES - 1)
    lengths.push(highway.curve.getLength())
    points.forEach((point, sample) => {
      highway.curve.getTangentAt(sample / (PATH_SAMPLES - 1), tangent)
      data.set([point.x, point.y + 0.6, point.z, 1], ((index * 2) * PATH_SAMPLES + sample) * 4)
      data.set([tangent.x, tangent.y, tangent.z, 0], ((index * 2 + 1) * PATH_SAMPLES + sample) * 4)
    })
  })
  const texture = new THREE.DataTexture(data, PATH_SAMPLES, HIGHWAYS.length * 2, THREE.RGBAFormat, THREE.FloatType)
  texture.needsUpdate = true
  return { texture, lengths }
}

function highwayDeck(): { deck: THREE.BufferGeometry, rails: THREE.BufferGeometry[], pillars: THREE.Matrix4[] } {
  const positions: number[] = []
  const normals: number[] = []
  const uvs: number[] = []
  const indices: number[] = []
  const rails: THREE.BufferGeometry[] = []
  const pillars: THREE.Matrix4[] = []
  const up = new THREE.Vector3(0, 1, 0)
  const tangent = new THREE.Vector3()
  const side = new THREE.Vector3()
  for (const highway of HIGHWAYS) {
    const count = 520
    const points = highway.curve.getSpacedPoints(count)
    const length = highway.curve.getLength()
    const half = highway.width / 2
    const railPositions: number[] = []
    const railTangents: number[] = []
    const railSides: number[] = []
    const railIndices: number[] = []
    // Cross-section: top surface, two barrier walls and the underside, as four strips.
    const strips = [
      { a: [-half, 0], b: [half, 0], normal: 'up' },
      { a: [half, 0], b: [half, -1.6], normal: 'right' },
      { a: [-half, -1.6], b: [-half, 0], normal: 'left' },
      { a: [half, -1.6], b: [-half, -1.6], normal: 'down' },
    ] as const
    for (const strip of strips) {
      const start = positions.length / 3
      points.forEach((point, index) => {
        highway.curve.getTangentAt(index / count, tangent)
        side.crossVectors(tangent, up).normalize()
        const distance = (index / count) * length
        for (const [offset, y, v] of [[strip.a[0], strip.a[1], 0], [strip.b[0], strip.b[1], 1]] as const) {
          positions.push(point.x + side.x * offset, point.y + y, point.z + side.z * offset)
          const normal = strip.normal === 'up' ? up : strip.normal === 'down' ? up.clone().negate() : strip.normal === 'right' ? side : side.clone().negate()
          normals.push(normal.x, normal.y, normal.z)
          uvs.push(distance, v)
        }
        if (index > 0) {
          const a = start + (index - 1) * 2
          indices.push(a, a + 2, a + 1, a + 1, a + 2, a + 3)
        }
      })
    }
    // Light rails along both barrier tops, and a pillar every 40 m.
    for (const offset of [-half + 0.3, half - 0.3]) {
      const start = railPositions.length / 3
      points.forEach((point, index) => {
        highway.curve.getTangentAt(index / count, tangent)
        side.crossVectors(tangent, up).normalize()
        for (const s of [-1, 1]) {
          railPositions.push(point.x + side.x * offset, point.y + 1.05, point.z + side.z * offset)
          railTangents.push(tangent.x, tangent.y, tangent.z)
          railSides.push(s)
        }
        if (index > 0) {
          const a = start + (index - 1) * 2
          railIndices.push(a, a + 2, a + 1, a + 1, a + 2, a + 3)
        }
      })
    }
    const rail = new THREE.BufferGeometry()
    rail.setAttribute('position', new THREE.Float32BufferAttribute(railPositions, 3))
    rail.setAttribute('aTangent', new THREE.Float32BufferAttribute(railTangents, 3))
    rail.setAttribute('aSide', new THREE.Float32BufferAttribute(railSides, 1))
    rail.setIndex(railIndices)
    rails.push(rail)
    for (let distance = 20; distance < length; distance += 40) {
      const point = highway.curve.getPointAt(distance / length)
      const height = point.y - 1.6
      pillars.push(new THREE.Matrix4().compose(new THREE.Vector3(point.x, 0, point.z), new THREE.Quaternion(), new THREE.Vector3(3.2, height, 3.2)))
    }
  }
  const deck = new THREE.BufferGeometry()
  deck.setAttribute('position', new THREE.Float32BufferAttribute(positions, 3))
  deck.setAttribute('normal', new THREE.Float32BufferAttribute(normals, 3))
  deck.setAttribute('uv', new THREE.Float32BufferAttribute(uvs, 2))
  deck.setIndex(indices)
  return { deck, rails, pillars }
}

function lampPositions(random: () => number): { positions: number[], colors: number[], poles: THREE.Matrix4[] } {
  const positions: number[] = []
  const colors: number[] = []
  const poles: THREE.Matrix4[] = []
  const sodium = [1, 0.62, 0.3]
  const white = [0.85, 0.9, 1]
  for (const alongZ of [true, false]) {
    for (const center of ROAD_CENTERS) {
      const half = roadWidth(center, alongZ) / 2
      const tint = random() < 0.35 ? white : sodium
      for (let s = -EXTENT + 16; s < EXTENT; s += 32) {
        // Skip lamps standing in a crossing.
        const cross = ROAD_CENTERS.some((other) => Math.abs(other - s) < ROAD / 2 + 3)
        if (cross) continue
        for (const sign of [-1, 1]) {
          const lateral = center + sign * (half - 0.8)
          const x = alongZ ? lateral : s
          const z = alongZ ? s : lateral
          positions.push(x, 9, z)
          colors.push(...tint)
          poles.push(new THREE.Matrix4().compose(new THREE.Vector3(x, 0, z), new THREE.Quaternion(), new THREE.Vector3(0.22, 9, 0.22)))
        }
      }
    }
  }
  // Tall sodium lamps along the highways.
  for (const highway of HIGHWAYS) {
    const length = highway.curve.getLength()
    const tangent = new THREE.Vector3()
    const side = new THREE.Vector3()
    for (let distance = 18; distance < length; distance += 36) {
      const u = distance / length
      const point = highway.curve.getPointAt(u)
      highway.curve.getTangentAt(u, tangent)
      side.crossVectors(tangent, new THREE.Vector3(0, 1, 0)).normalize()
      for (const sign of [-1, 1]) {
        positions.push(point.x + side.x * sign * (highway.width / 2 - 0.6), point.y + 8, point.z + side.z * sign * (highway.width / 2 - 0.6))
        colors.push(...sodium)
      }
    }
  }
  return { positions, colors, poles }
}

export function createTraffic(uniforms: AtmosphereUniforms, random: () => number): THREE.Group {
  const group = new THREE.Group()
  const time = { uTime: uniforms.uTime, uPixelAngle: uniforms.uPixelAngle }

  // Street traffic: one instanced quad (lights) and one box (body) per car.
  const cars = streetLanes(random)
  const laneData = new Float32Array(cars.length * 4)
  const moveData = new Float32Array(cars.length * 4)
  cars.forEach((car, index) => {
    laneData.set(car.lane, index * 4)
    moveData.set(car.move, index * 4)
  })
  const laneAttribute = new THREE.InstancedBufferAttribute(laneData, 4)
  const moveAttribute = new THREE.InstancedBufferAttribute(moveData, 4)

  const lightGeometry = instanced(lampQuads(), cars.length)
  lightGeometry.setAttribute('aLane', laneAttribute)
  lightGeometry.setAttribute('aMove', moveAttribute)
  const streetLights = new THREE.Mesh(lightGeometry, additive(new THREE.ShaderMaterial({
    uniforms: { ...time, ...fog(uniforms) },
    vertexShader: `${LAMP_COMMON}${STREET_CAR}void main() { vec3 dir; vec3 c = carCenter(dir); lamps(c, dir); }`,
    fragmentShader: LAMP_FRAGMENT,
  })))
  streetLights.frustumCulled = false
  group.add(streetLights)

  // A low chassis and a cabin set slightly back: enough to read as a car from the street.
  const chassis = new THREE.BoxGeometry(4.4, 0.75, 1.85)
  chassis.translate(0, 0.62, 0)
  const cabin = new THREE.BoxGeometry(2.3, 0.6, 1.6)
  cabin.translate(-0.25, 1.28, 0)
  const bodyBox = mergeGeometries([chassis, cabin])!
  const bodyGeometry = instanced(bodyBox, cars.length)
  bodyGeometry.setAttribute('aLane', laneAttribute)
  bodyGeometry.setAttribute('aMove', moveAttribute)
  const bodies = new THREE.Mesh(bodyGeometry, new THREE.ShaderMaterial({
    uniforms: { ...time, ...light(uniforms) },
    vertexShader: `uniform float uTime;\n${STREET_CAR}${BODY_VERTEX}void main() { vec3 dir; vec3 c = carCenter(dir); body(c, dir, aMove.w); }`,
    fragmentShader: BODY_FRAGMENT,
  }))
  bodies.frustumCulled = false
  group.add(bodies)

  // Elevated highways: deck, glowing rails, pillars and dense traffic along the curves.
  const { deck, rails, pillars } = highwayDeck()
  const deckMaterial = new THREE.ShaderMaterial({
    uniforms: { ...light(uniforms), uLamps: uniforms.uLamps, uDeck: { value: 1 } },
    vertexShader: CONCRETE_VERTEX.replace('modelMatrix * instanceMatrix', 'modelMatrix').replace('mat3(modelMatrix) * mat3(instanceMatrix)', 'mat3(modelMatrix)'),
    fragmentShader: CONCRETE_FRAGMENT,
    side: THREE.DoubleSide,
  })
  group.add(new THREE.Mesh(deck, deckMaterial))
  rails.forEach((rail, index) => {
    const mesh = new THREE.Mesh(rail, additive(new THREE.ShaderMaterial({
      uniforms: { uPixelAngle: uniforms.uPixelAngle, uNeon: uniforms.uNeon, uColor: { value: HIGHWAYS[index]!.rail }, ...fog(uniforms) },
      vertexShader: RAIL_VERTEX,
      fragmentShader: RAIL_FRAGMENT,
      side: THREE.DoubleSide,
    })))
    mesh.frustumCulled = false
    group.add(mesh)
  })
  const pillarBox = new THREE.BoxGeometry(1, 1, 1)
  pillarBox.translate(0, 0.5, 0)
  const pillarMesh = new THREE.InstancedMesh(pillarBox, new THREE.ShaderMaterial({
    uniforms: { ...light(uniforms), uLamps: uniforms.uLamps, uDeck: { value: 0 } },
    vertexShader: CONCRETE_VERTEX,
    fragmentShader: CONCRETE_FRAGMENT,
  }), pillars.length)
  pillars.forEach((matrix, index) => pillarMesh.setMatrixAt(index, matrix))
  pillarMesh.computeBoundingSphere()
  group.add(pillarMesh)

  const { texture, lengths } = pathTexture()
  const highwayCars: { path: number[], move: number[] }[] = []
  HIGHWAYS.forEach((highway, index) => {
    const length = lengths[index]!
    for (let lane = 0; lane < highway.lanes; lane++) {
      for (const direction of [1, -1]) {
        // Right-hand traffic: the side vector points to the right of travel along the curve.
        const lateral = direction * (2.2 + lane * 3.5)
        const spacing = 22 + random() * 30
        const speed = 17 + random() * 7 - lane * 1.5
        for (let s = random() * spacing; s < length; s += spacing * (0.7 + random() * 0.6)) {
          highwayCars.push({ path: [index * 2, lateral, direction, random()], move: [length, speed, s, 0] })
        }
      }
    }
  })
  const pathData = new Float32Array(highwayCars.length * 4)
  const pathMove = new Float32Array(highwayCars.length * 4)
  highwayCars.forEach((car, index) => {
    pathData.set(car.path, index * 4)
    pathMove.set(car.move, index * 4)
  })
  const pathAttribute = new THREE.InstancedBufferAttribute(pathData, 4)
  const pathMoveAttribute = new THREE.InstancedBufferAttribute(pathMove, 4)
  const highwayLightGeometry = instanced(lampQuads(), highwayCars.length)
  highwayLightGeometry.setAttribute('aPath', pathAttribute)
  highwayLightGeometry.setAttribute('aMove', pathMoveAttribute)
  const highwayLights = new THREE.Mesh(highwayLightGeometry, additive(new THREE.ShaderMaterial({
    uniforms: { ...time, ...fog(uniforms), uPath: { value: texture } },
    vertexShader: `${LAMP_COMMON}${HIGHWAY_CAR}void main() { vec3 dir; vec3 c = carCenter(dir); lamps(c, normalize(vec3(dir.x, 0.0, dir.z))); }`,
    fragmentShader: LAMP_FRAGMENT,
  })))
  highwayLights.frustumCulled = false
  group.add(highwayLights)
  const highwayBodyGeometry = instanced(bodyBox, highwayCars.length)
  highwayBodyGeometry.setAttribute('aPath', pathAttribute)
  highwayBodyGeometry.setAttribute('aMove', pathMoveAttribute)
  const highwayBodies = new THREE.Mesh(highwayBodyGeometry, new THREE.ShaderMaterial({
    uniforms: { ...time, ...light(uniforms), uPath: { value: texture } },
    vertexShader: `uniform float uTime;\n${HIGHWAY_CAR}${BODY_VERTEX}void main() { vec3 dir; vec3 c = carCenter(dir); body(c, normalize(vec3(dir.x, 0.0, dir.z)), aPath.w); }`,
    fragmentShader: BODY_FRAGMENT,
  }))
  highwayBodies.frustumCulled = false
  group.add(highwayBodies)

  // Street lamps: glowing heads (points) on thin dark poles.
  const lamps = lampPositions(random)
  const lampGeometry = new THREE.BufferGeometry()
  lampGeometry.setAttribute('position', new THREE.Float32BufferAttribute(lamps.positions, 3))
  lampGeometry.setAttribute('aColor', new THREE.Float32BufferAttribute(lamps.colors, 3))
  const lampPoints = new THREE.Points(lampGeometry, additive(new THREE.ShaderMaterial({
    uniforms: { uPointScale: uniforms.uPointScale, uLamps: uniforms.uLamps, ...fog(uniforms) },
    vertexShader: STREET_LAMP_VERTEX,
    fragmentShader: STREET_LAMP_FRAGMENT,
  })))
  lampPoints.frustumCulled = false
  group.add(lampPoints)
  const poleMesh = new THREE.InstancedMesh(pillarBox, new THREE.ShaderMaterial({
    uniforms: { ...light(uniforms), uLamps: uniforms.uLamps, uDeck: { value: 0 } },
    vertexShader: CONCRETE_VERTEX,
    fragmentShader: CONCRETE_FRAGMENT,
  }), lamps.poles.length)
  lamps.poles.forEach((matrix, index) => poleMesh.setMatrixAt(index, matrix))
  poleMesh.computeBoundingSphere()
  group.add(poleMesh)
  return group
}
