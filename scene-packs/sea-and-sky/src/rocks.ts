import * as THREE from 'three'
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js'
import { dracoLoader, loadTexture } from './loading'
import { LIGHTING_GLSL, type LightingUniforms } from './shading'
import { ROCKS } from './world'

/** Directions round each rock in which its waterline is measured, for the surf (see ocean.ts). */
export const WATERLINE_BINS = 32

/**
 * The sea stacks, sculpted in Blender (see blender/rocks.py): layered sandstone
 * weathered into ledges, joints and a wave-cut notch, dark and weedy where the
 * waves wash, pale with salt on the ledges. Each stack is a light mesh carrying
 * the sculpture as baked maps: colour, an object-space normal map and ambient
 * occlusion. They catch the low sun warmly and stand as silhouettes against the
 * sunset and the moonlit sea.
 */
const VERTEX = /* glsl */ `
varying vec2 vUv;
varying vec3 vWorld;
void main() {
  vUv = uv;
  vec4 world = modelMatrix * vec4(position, 1.0);
  vWorld = world.xyz;
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const FRAGMENT = /* glsl */ `
uniform sampler2D uColour;
uniform sampler2D uSurface;
uniform float uTime;
uniform float uMirrorPass;
varying vec2 vUv;
varying vec3 vWorld;
${LIGHTING_GLSL}
void main() {
  // Seen from below the water for the reflection: only what stands above it.
  if (uMirrorPass > 0.5 && vWorld.y < 0.0) discard;
  vec4 surface = texture2D(uSurface, vUv);
  // Baked in Blender's object space (z up); the stacks are only moved, never turned, so after
  // swapping to three.js axes this is the world normal.
  vec3 baked = surface.xyz * 2.0 - 1.0;
  vec3 n = normalize(vec3(baked.x, baked.z, -baked.y));
  vec3 albedo = texture2D(uColour, vUv).rgb;
  // Freshly wet where the waves wash up the foot, rising and falling with each set: darker, and
  // glossy in the light.
  float swash = 0.6 + 0.8 * (sin(uTime * 0.85 + dot(vWorld.xz, vec2(0.07, 0.05))) * 0.5 + 0.5);
  swash += 0.35 * sin(vWorld.x * 0.9 + vWorld.z * 0.4) * sin(vWorld.z * 1.3 - vWorld.x * 0.3);
  float wet = 1.0 - smoothstep(0.0, swash, vWorld.y);
  albedo *= mix(1.0, 0.7, wet);
  float shadow = keyShadow(vWorld + n * 1.0);
  vec3 color = shade(albedo, n, 0.15, shadow) * mix(0.35, 1.0, surface.a);
  vec3 view = normalize(cameraPosition - vWorld);
  float gloss = pow(max(dot(n, normalize(view + uLightDir)), 0.0), 70.0);
  color += uLight * gloss * wet * shadow * 0.9;
  color = atmosphere(color, vWorld);
  // In the reflection, the higher up the rock the further out on the water its image lies, and the
  // more the waves break it up: so it fades with height, and clings to the foot of the rock.
  float mirrorFade = uMirrorPass > 0.5 ? exp(-max(vWorld.y, 0.0) / 18.0) : 1.0;
  gl_FragColor = vec4(color * mirrorFade, mirrorFade);
}
`

/**
 * How far the sculpted rock reaches at the waterline in each direction round it: one row per rock,
 * WATERLINE_BINS columns from angle -pi to pi (atan2 of z and x from its centre), in metres. The
 * surf breaks against this outline, which is far from round.
 */
function measureWaterlines(meshes: THREE.Mesh[]): THREE.DataTexture {
  const radii = new Float32Array(WATERLINE_BINS * ROCKS.length)
  const point = new THREE.Vector3()
  for (const mesh of meshes) {
    const index = Number(/rock-(d+)/.exec(mesh.name)?.[1] ?? /rock-(d+)/.exec(mesh.parent?.name ?? '')?.[1])
    const rock = ROCKS[index]
    if (!rock) continue
    mesh.updateWorldMatrix(true, false)
    const positions = mesh.geometry.getAttribute('position')
    const row = radii.subarray(index * WATERLINE_BINS, (index + 1) * WATERLINE_BINS)
    for (let vertex = 0; vertex < positions.count; vertex++) {
      point.fromBufferAttribute(positions, vertex).applyMatrix4(mesh.matrixWorld)
      if (Math.abs(point.y) > 0.8) continue
      const dx = point.x - rock.x
      const dz = point.z - rock.z
      const bin = Math.min(WATERLINE_BINS - 1, Math.floor(((Math.atan2(dz, dx) + Math.PI) / (Math.PI * 2)) * WATERLINE_BINS))
      row[bin] = Math.max(row[bin]!, Math.hypot(dx, dz))
    }
    // Any direction without a vertex takes its neighbours' reach.
    for (let bin = 0; bin < WATERLINE_BINS; bin++) {
      if (row[bin]) continue
      row[bin] = Math.max(row[(bin + WATERLINE_BINS - 1) % WATERLINE_BINS]!, row[(bin + 1) % WATERLINE_BINS]!, rock.radius * 0.8)
    }
  }
  const data = new Uint16Array(radii.length)
  radii.forEach((radius, index) => { data[index] = THREE.DataUtils.toHalfFloat(radius) })
  const texture = new THREE.DataTexture(data, WATERLINE_BINS, ROCKS.length, THREE.RedFormat, THREE.HalfFloatType)
  texture.wrapS = THREE.RepeatWrapping
  texture.minFilter = THREE.LinearFilter
  texture.magFilter = THREE.LinearFilter
  texture.needsUpdate = true
  return texture
}

export function rockMaterial(uniforms: LightingUniforms, colour: THREE.Texture | null = null, surface: THREE.Texture | null = null): THREE.ShaderMaterial {
  return new THREE.ShaderMaterial({
    uniforms: { ...uniforms, uColour: { value: colour }, uSurface: { value: surface } },
    vertexShader: VERTEX,
    fragmentShader: FRAGMENT,
  })
}

/**
 * Loads the stacks: the meshes (Draco-compressed) and every map at once, rather than the maps only
 * once the meshes are in. Each map is sent to the graphics card as soon as it arrives.
 */
export async function createRocks(uniforms: LightingUniforms, renderer: THREE.WebGLRenderer): Promise<{ object: THREE.Object3D, waterlines: THREE.DataTexture }> {
  const load = async (path: string, colour: boolean) => {
    // The maps are stored top row first, as glTF expects.
    const texture = await loadTexture(path, false)
    texture.colorSpace = colour ? THREE.SRGBColorSpace : THREE.NoColorSpace
    texture.anisotropy = 8
    renderer.initTexture(texture)
    return texture
  }
  const maps = ROCKS.map((_, index) => Promise.all([load(`assets/rock-${index}-colour.webp`, true), load(`assets/rock-${index}-surface.webp`, false)]))
  const gltf = await new GLTFLoader().setDRACOLoader(dracoLoader()).loadAsync('assets/rocks.glb')
  const meshes: THREE.Mesh[] = []
  gltf.scene.traverse((object) => { if ((object as THREE.Mesh).isMesh) meshes.push(object as THREE.Mesh) })
  await Promise.all(meshes.map(async (mesh) => {
    const index = Number(/rock-(\d+)/.exec(mesh.name)?.[1] ?? /rock-(\d+)/.exec(mesh.parent?.name ?? '')?.[1])
    const [colour, surface] = await maps[index]!
    mesh.material = rockMaterial(uniforms, colour, surface)
    mesh.frustumCulled = false
    // Layer 1: drawn again, from below the water, for the sea's reflection.
    mesh.layers.enable(1)
  }))
  return { object: gltf.scene, waterlines: measureWaterlines(meshes) }
}
