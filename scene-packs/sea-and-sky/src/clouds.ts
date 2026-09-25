import * as THREE from 'three'
import { track } from './loading'
import { packURL } from './runtime'

/**
 * Volumetric clouds: a layer of cumulus 1.4 to 3 km up, ray-marched through two
 * tileable noise volumes (tools/cloud-noise.mjs) at half resolution before the
 * scene is drawn. Each sample looks back towards the sun or moon through the
 * cloud above it, so the clouds have lit tops and shaded undersides, bright
 * silver edges against the light, and at sunset glow orange from below. The sky
 * dome lays the result over itself; the sea reflects a cheaper version of the
 * same cloud field (cloudCover below), so the two match.
 */
export const CLOUD_GLSL = /* glsl */ `
precision highp sampler3D;
uniform sampler3D uShape;
uniform sampler3D uDetail;
uniform float uCloudCover;
const float CLOUD_BOTTOM = 1400.0;
const float CLOUD_TOP = 3000.0;

float cloudRemap(float value, float low, float high) {
  return clamp((value - low) / (high - low), 0.0, 1.0);
}
vec3 cloudWind() {
  return vec3(uCloudTime * 6.0, 0.0, uCloudTime * 2.5);
}
/** Which parts of the sky have clouds at all: a very large, slow slice of the same noise. */
float cloudWeather(vec2 xz) {
  return textureLod(uShape, vec3((xz + cloudWind().xz * 0.4) / 42000.0, 0.37).xzy, 0.0).r;
}
/** Cloud density at p (0 outside a cloud). detailed: eat wisps out of the edges. The volumes have no
 * mipmaps, and this runs inside loops, so it samples level 0 explicitly. */
float cloudDensity(vec3 p, bool detailed) {
  float h = (p.y - CLOUD_BOTTOM) / (CLOUD_TOP - CLOUD_BOTTOM);
  if (h < 0.0 || h > 1.0) return 0.0;
  vec3 wind = cloudWind();
  float coverage = uCloudCover * smoothstep(0.42, 0.72, cloudWeather(p.xz));
  // Flat-ish bases, rounded tops.
  float profile = smoothstep(0.0, 0.08, h) * smoothstep(1.0, 0.45, h);
  float shape = textureLod(uShape, vec3(p.x + wind.x, p.y * 2.2, p.z + wind.z) / 9000.0, 0.0).r;
  float mass = cloudRemap(shape * profile, 1.0 - coverage, 1.0);
  if (mass <= 0.0 || !detailed) return mass;
  float wisps = textureLod(uDetail, (p + wind * 1.4) / 1500.0, 0.0).r;
  return cloudRemap(mass, wisps * 0.35, 1.0);
}
/** The cloud field seen along direction d from the camera, cheaply: cover and brightness, for reflections. */
vec2 cloudCover(vec3 d) {
  if (d.y < 0.015) return vec2(0.0);
  vec3 p = cameraPosition + d * ((2000.0 - cameraPosition.y) / d.y);
  float cover = 0.0;
  for (int i = 0; i < 3; i++) cover += cloudDensity(p + d * float(i) * 280.0 / d.y, false);
  cover = clamp(cover * 0.9, 0.0, 1.0) * smoothstep(0.015, 0.12, d.y);
  // Darker where more cloud stands between this one and the light.
  float lit = 1.05 - 0.6 * cloudDensity(p + uLightDir * 600.0, false);
  return vec2(cover, clamp(lit, 0.25, 1.05));
}
`

const MARCH_VERTEX = /* glsl */ `
varying vec2 vUv;
void main() {
  vUv = uv;
  gl_Position = vec4(position.xy, 0.0, 1.0);
}
`

const MARCH_FRAGMENT = /* glsl */ `
uniform mat4 uInverseProjection;
uniform mat4 uCameraWorld;
uniform vec3 uCameraPosition;
uniform vec3 uLightDir;
uniform vec3 uLight;
uniform vec3 uAmbientTop;
uniform vec3 uSkyHorizon;
uniform float uNight;
uniform float uCloudTime;
varying vec2 vUv;
#define cameraPosition uCameraPosition
${CLOUD_GLSL}
const float FAR = 36000.0;
const int STEPS = 56;
const float EXTINCTION = 0.0055;

float henyeyGreenstein(float cosine, float g) {
  float g2 = g * g;
  return (1.0 - g2) / (12.566 * pow(1.0 + g2 - 2.0 * g * cosine, 1.5));
}
/** How much cloud lies between p and the light. */
float towardsLight(vec3 p) {
  float depth = 0.0;
  float stepLength = 90.0;
  for (int i = 0; i < 5; i++) {
    p += uLightDir * stepLength;
    depth += cloudDensity(p, false) * stepLength;
    stepLength *= 1.7;
  }
  return depth;
}
void main() {
  vec4 view = uInverseProjection * vec4(vUv * 2.0 - 1.0, 1.0, 1.0);
  vec3 dir = normalize((uCameraWorld * vec4(view.xyz / view.w, 0.0)).xyz);
  float enter = (CLOUD_BOTTOM - uCameraPosition.y) / dir.y;
  if (dir.y < 0.01 || enter > FAR) {
    gl_FragColor = vec4(0.0, 0.0, 0.0, 1.0);
    return;
  }
  float leave = min((CLOUD_TOP - uCameraPosition.y) / dir.y, FAR);
  float stepLength = (leave - enter) / float(STEPS);
  // A fixed per-pixel offset (interleaved gradient noise) breaks up banding without flicker.
  float t = enter + stepLength * fract(52.9829189 * fract(dot(gl_FragCoord.xy, vec2(0.06711056, 0.00583715))));
  float cosine = dot(dir, uLightDir);
  // Bright forward scattering round the light (the silver edges), a little back scattering too.
  float phase = mix(henyeyGreenstein(cosine, 0.6), henyeyGreenstein(cosine, -0.2), 0.3) * 12.566;
  float transmittance = 1.0;
  vec3 light = vec3(0.0);
  for (int i = 0; i < STEPS; i++) {
    vec3 p = uCameraPosition + dir * t;
    float density = cloudDensity(p, true);
    if (density > 0.003) {
      float h = (p.y - CLOUD_BOTTOM) / (CLOUD_TOP - CLOUD_BOTTOM);
      float absorb = exp(-density * stepLength * EXTINCTION);
      // Sunlight through the cloud above, brighter where the cloud is thin at the edge.
      float lightPath = exp(-towardsLight(p) * EXTINCTION);
      float powder = 1.0 - exp(-density * 120.0 * EXTINCTION * 2.0);
      vec3 sunlit = uLight * lightPath * phase * mix(0.6, 1.0, powder) * 0.55;
      vec3 skylight = mix(uSkyHorizon * 0.55, uAmbientTop * 1.1, h);
      light += transmittance * (1.0 - absorb) * (sunlit + skylight);
      transmittance *= absorb;
      if (transmittance < 0.02) break;
    }
    t += stepLength;
  }
  // Far clouds melt into the haze on the horizon.
  float haze = smoothstep(9000.0, FAR, enter);
  light = mix(light, uSkyHorizon * 0.9 * (1.0 - transmittance), haze * 0.7);
  gl_FragColor = vec4(light, transmittance);
}
`

async function loadVolume(path: string, size: number): Promise<THREE.Data3DTexture> {
  const bytes = await track(path, fetch(packURL(path)).then((response) => {
    if (!response.ok) throw new Error(`${path}: ${response.status}`)
    return response.arrayBuffer()
  }))
  const texture = new THREE.Data3DTexture(new Uint8Array(bytes), size, size, size)
  texture.format = THREE.RedFormat
  texture.type = THREE.UnsignedByteType
  texture.minFilter = THREE.LinearFilter
  texture.magFilter = THREE.LinearFilter
  texture.wrapS = THREE.RepeatWrapping
  texture.wrapT = THREE.RepeatWrapping
  texture.wrapR = THREE.RepeatWrapping
  texture.unpackAlignment = 1
  texture.needsUpdate = true
  return texture
}

export interface Clouds {
  /** The clouds for the current view: colour, and in alpha how much of the sky shows through. */
  texture: THREE.Texture
  /** The two noise volumes, once downloaded (the shader is compiled without waiting for them). */
  loaded: Promise<{ shape: THREE.Data3DTexture, detail: THREE.Data3DTexture }>
  /** Compiles the marching shader, without blocking the page where the browser allows. */
  compile(): Promise<unknown>
  render(camera: THREE.PerspectiveCamera): void
  /** scale: the fraction of the screen's resolution the clouds are marched at. */
  resize(width: number, height: number, scale: number): void
}

export function createClouds(renderer: THREE.WebGLRenderer, uniforms: Record<string, THREE.IUniform>): Clouds {
  const target = new THREE.WebGLRenderTarget(1, 1, { type: THREE.HalfFloatType, depthBuffer: false })
  const material = new THREE.ShaderMaterial({
    uniforms: {
      ...uniforms,
      uShape: { value: null },
      uDetail: { value: null },
      uInverseProjection: { value: new THREE.Matrix4() },
      uCameraWorld: { value: new THREE.Matrix4() },
      uCameraPosition: { value: new THREE.Vector3() },
    },
    vertexShader: MARCH_VERTEX,
    fragmentShader: MARCH_FRAGMENT,
    depthTest: false,
    depthWrite: false,
  })
  const quad = new THREE.Mesh(new THREE.PlaneGeometry(2, 2), material)
  quad.frustumCulled = false
  const scene = new THREE.Scene()
  scene.add(quad)
  const still = new THREE.OrthographicCamera(-1, 1, 1, -1, 0, 1)
  const loaded = Promise.all([loadVolume('assets/cloud-shape.bin', 96), loadVolume('assets/cloud-detail.bin', 32)]).then(([shape, detail]) => {
    material.uniforms.uShape!.value = shape
    material.uniforms.uDetail!.value = detail
    return { shape, detail }
  })
  return {
    texture: target.texture,
    loaded,
    compile: () => renderer.compileAsync(scene, still),
    render(camera) {
      material.uniforms.uInverseProjection!.value.copy(camera.projectionMatrixInverse)
      material.uniforms.uCameraWorld!.value.copy(camera.matrixWorld)
      material.uniforms.uCameraPosition!.value.copy(camera.position)
      const previous = renderer.getRenderTarget()
      renderer.setRenderTarget(target)
      renderer.render(scene, still)
      renderer.setRenderTarget(previous)
    },
    resize(width, height, scale) {
      target.setSize(Math.max(1, Math.round(width * scale)), Math.max(1, Math.round(height * scale)))
    },
  }
}
