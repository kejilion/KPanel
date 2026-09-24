import * as THREE from 'three'
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js'
import { LIGHTING_GLSL, type LightingUniforms } from './shading'

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
varying vec2 vUv;
varying vec3 vWorld;
${LIGHTING_GLSL}
void main() {
  vec4 surface = texture2D(uSurface, vUv);
  // Baked in Blender's object space (z up); the stacks are only moved, never turned, so after
  // swapping to three.js axes this is the world normal.
  vec3 baked = surface.xyz * 2.0 - 1.0;
  vec3 n = normalize(vec3(baked.x, baked.z, -baked.y));
  vec3 albedo = texture2D(uColour, vUv).rgb;
  float shadow = keyShadow(vWorld + n * 1.0);
  vec3 color = shade(albedo, n, 0.15, shadow) * mix(0.35, 1.0, surface.a);
  gl_FragColor = vec4(atmosphere(color, vWorld), 1.0);
}
`

export async function createRocks(uniforms: LightingUniforms): Promise<THREE.Object3D> {
  const textures = new THREE.TextureLoader()
  const load = async (path: string, colour: boolean) => {
    const texture = await textures.loadAsync(path)
    // The maps are stored top row first, as glTF expects.
    texture.flipY = false
    texture.colorSpace = colour ? THREE.SRGBColorSpace : THREE.NoColorSpace
    texture.anisotropy = 8
    return texture
  }
  const gltf = await new GLTFLoader().loadAsync('assets/rocks.glb')
  const meshes: THREE.Mesh[] = []
  gltf.scene.traverse((object) => { if ((object as THREE.Mesh).isMesh) meshes.push(object as THREE.Mesh) })
  await Promise.all(meshes.map(async (mesh) => {
    const index = /rock-(\d+)/.exec(mesh.name)?.[1] ?? /rock-(\d+)/.exec(mesh.parent?.name ?? '')?.[1]
    const [colour, surface] = await Promise.all([load(`assets/rock-${index}-colour.webp`, true), load(`assets/rock-${index}-surface.webp`, false)])
    mesh.material = new THREE.ShaderMaterial({
      uniforms: { ...uniforms, uColour: { value: colour }, uSurface: { value: surface } },
      vertexShader: VERTEX,
      fragmentShader: FRAGMENT,
    })
    mesh.frustumCulled = false
  }))
  return gltf.scene
}
