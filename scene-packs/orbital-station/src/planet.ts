import * as THREE from 'three'
import { NOISE_GLSL } from './noise'

export const PLANET_RADIUS = 100

const SPHERE_VERTEX = /* glsl */ `
varying vec2 vUv;
varying vec3 vObject;
varying vec3 vWorldNormal;
varying vec3 vWorldPosition;
void main() {
  vUv = uv;
  vObject = normalize(position);
  vec4 world = modelMatrix * vec4(position, 1.0);
  vWorldPosition = world.xyz;
  vWorldNormal = normalize(mat3(modelMatrix) * normal);
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

/**
 * The surface, painted offline in detail (tools/planet.py): colour, relief (an object-space
 * normal map) and masks for water and city lights. Clouds (their own sphere, turning a little
 * faster) cast soft shadows on it.
 */
const SURFACE_FRAGMENT = /* glsl */ `
uniform vec3 uSunDirection;
uniform float uCityLights;
uniform float uRotation;
uniform float uCloudShift;
uniform sampler2D uSurface;
uniform sampler2D uRelief;
uniform sampler2D uMasks;
uniform sampler2D uClouds;
varying vec2 vUv;
varying vec3 vObject;
varying vec3 vWorldNormal;
varying vec3 vWorldPosition;
void main() {
  vec2 masks = texture2D(uMasks, vUv).rg;
  float water = masks.r;
  float cities = masks.g;
  // Open water is dark, but not black: it takes a little of the sky's blue.
  vec3 albedo = texture2D(uSurface, vUv).rgb;
  albedo = mix(albedo, albedo * 1.6 + vec3(0.004, 0.014, 0.036), water);
  // The relief is in the sphere's own axes: turn it with the planet (it only spins about y).
  vec3 relief = texture2D(uRelief, vUv).rgb * 2.0 - 1.0;
  float c = cos(uRotation);
  float s = sin(uRotation);
  vec3 bumped = normalize(vec3(c * relief.x + s * relief.z, relief.y, -s * relief.x + c * relief.z));
  vec3 sphere = normalize(vWorldNormal);
  vec3 N = normalize(mix(bumped, sphere, water));
  vec3 L = normalize(uSunDirection);
  vec3 V = normalize(cameraPosition - vWorldPosition);
  float ndl = dot(N, L);
  float sphereNdl = dot(sphere, L);
  float daylight = smoothstep(-0.1, 0.3, sphereNdl);
  // Soft shadows of the clouds overhead.
  float overhead = texture2D(uClouds, vec2(vUv.x + uCloudShift, vUv.y)).r;
  float shade = 1.0 - 0.5 * overhead;
  vec3 color = albedo * (0.012 + 1.45 * max(ndl, 0.0) * smoothstep(-0.05, 0.1, sphereNdl) * shade);
  // Warm light along the terminator.
  color += albedo * vec3(1.0, 0.38, 0.12) * smoothstep(0.28, 0.0, abs(sphereNdl)) * 0.35 * shade;

  // The sun's glint on open water: a sharp core and a broad sheen.
  vec3 H = normalize(L + V);
  float nh = max(dot(sphere, H), 0.0);
  float glint = (pow(nh, 500.0) * 0.9 + pow(nh, 60.0) * 0.025) * water * shade;
  color += vec3(1.0, 0.88, 0.72) * glint * daylight;

  // City lights on the night side, dimmed under cloud.
  float night = 1.0 - smoothstep(-0.22, 0.04, sphereNdl);
  color += vec3(1.0, 0.62, 0.28) * cities * night * uCityLights * 3.0 * (1.0 - 0.75 * overhead);

  // The air over the day side scatters blue light back out, more so towards the limb.
  float rim = pow(1.0 - max(dot(sphere, V), 0.0), 3.0);
  float air = sqrt(max(sphereNdl, 0.0)) * (0.35 + 1.2 * rim);
  color += vec3(0.045, 0.1, 0.24) * air * 0.35;
  color = mix(color, vec3(0.25, 0.5, 1.0) * (0.04 + daylight * 0.8), rim * 0.4);
  gl_FragColor = vec4(color, 1.0);
  #include <tonemapping_fragment>
  #include <colorspace_fragment>
}
`

const CLOUD_FRAGMENT = /* glsl */ `
uniform vec3 uSunDirection;
uniform sampler2D uClouds;
uniform sampler2D uMasks;
uniform float uCloudShift;
varying vec2 vUv;
varying vec3 vObject;
varying vec3 vWorldNormal;
varying vec3 vWorldPosition;
void main() {
  // The painted cover as it is: thick cores, and edges and veils you can see the ground through.
  float cover = texture2D(uClouds, vUv).r;
  float alpha = cover * 0.92;
  vec3 N = normalize(vWorldNormal);
  float ndl = dot(N, normalize(uSunDirection));
  float lit = smoothstep(-0.12, 0.35, ndl);
  // Thick cloud is brighter on top. On the night side a cloud is all but black: only the faintest
  // sky light, and the orange glow of the cities underneath, spread and softened by the cloud.
  float night = 1.0 - smoothstep(-0.22, 0.04, ndl);
  float below = textureLod(uMasks, vec2(vUv.x - uCloudShift, vUv.y), 5.0).g;
  vec3 dark = vec3(0.0012, 0.0015, 0.0025) + vec3(1.0, 0.55, 0.22) * below * night * 0.9;
  // Thin cloud scatters a little blue from the air under it; thick cloud is bright white on top.
  vec3 day = mix(vec3(0.62, 0.7, 0.82), vec3(0.9), smoothstep(0.1, 0.8, cover));
  vec3 color = mix(dark, day, lit);
  color += vec3(1.0, 0.42, 0.18) * smoothstep(0.16, 0.0, abs(ndl)) * 0.35;
  gl_FragColor = vec4(color, alpha);
  #include <tonemapping_fragment>
  #include <colorspace_fragment>
}
`

const ATMOSPHERE_FRAGMENT = /* glsl */ `
uniform vec3 uSunDirection;
uniform float uIntensity;
varying vec3 vObject;
varying vec3 vWorldNormal;
varying vec3 vWorldPosition;
void main() {
  vec3 N = normalize(vWorldNormal);
  vec3 V = normalize(cameraPosition - vWorldPosition);
  vec3 L = normalize(uSunDirection);
  // Back faces: 0 at the outer edge of the halo, growing towards the planet limb.
  float depth = clamp(-dot(N, V), 0.0, 1.0);
  float halo = pow(smoothstep(0.0, 0.34, depth), 2.6);
  float sunSide = dot(N, L);
  vec3 sky = mix(vec3(1.0, 0.42, 0.16), vec3(0.28, 0.58, 1.0), smoothstep(-0.1, 0.45, sunSide));
  float lit = smoothstep(-0.35, 0.35, sunSide);
  // Forward scattering: the limb blazes when the sun sits just behind the planet.
  float forward = pow(max(dot(-V, L), 0.0), 10.0) * smoothstep(-0.4, 0.1, sunSide);
  vec3 color = sky * halo * (lit * 1.25 + 0.012) + vec3(1.0, 0.7, 0.45) * halo * forward * 3.0;
  gl_FragColor = vec4(color * uIntensity, 1.0);
}
`

const MOON_FRAGMENT = /* glsl */ `
uniform vec3 uSunDirection;
varying vec3 vObject;
varying vec3 vWorldNormal;
varying vec3 vWorldPosition;
${NOISE_GLSL}
void main() {
  float craters = fbm(vObject * 5.0);
  float maria = smoothstep(0.0, 0.3, fbm3(vObject * 1.6 + 2.0));
  vec3 albedo = mix(vec3(0.55, 0.53, 0.5), vec3(0.28, 0.27, 0.27), maria) * (0.8 + craters * 0.35);
  float ndl = max(dot(normalize(vWorldNormal), normalize(uSunDirection)), 0.0);
  gl_FragColor = vec4(albedo * (0.01 + ndl * 1.4), 1.0);
  #include <tonemapping_fragment>
  #include <colorspace_fragment>
}
`

export interface Planet {
  group: THREE.Group
  update(time: number, dt: number): void
  setCityLights(value: number): void
}

async function loadMap(path: string, colour: boolean): Promise<THREE.Texture> {
  const texture = await new THREE.TextureLoader().loadAsync(path)
  texture.colorSpace = colour ? THREE.SRGBColorSpace : THREE.NoColorSpace
  texture.anisotropy = 8
  texture.wrapS = THREE.RepeatWrapping
  return texture
}

export async function createPlanet(sunDirection: THREE.Vector3): Promise<Planet> {
  const group = new THREE.Group()
  const sun = { value: sunDirection }
  const [surfaceMap, reliefMap, masksMap, cloudsMap] = await Promise.all([
    loadMap('assets/planet-surface.webp', true),
    loadMap('assets/planet-relief.webp', false),
    loadMap('assets/planet-masks.webp', false),
    loadMap('assets/planet-clouds.webp', false),
  ])

  const surfaceUniforms = {
    uSunDirection: sun,
    uCityLights: { value: 1 },
    uRotation: { value: 0 },
    uCloudShift: { value: 0 },
    uSurface: { value: surfaceMap },
    uRelief: { value: reliefMap },
    uMasks: { value: masksMap },
    uClouds: { value: cloudsMap },
  }
  const surface = new THREE.Mesh(
    new THREE.SphereGeometry(PLANET_RADIUS, 192, 128),
    new THREE.ShaderMaterial({ vertexShader: SPHERE_VERTEX, fragmentShader: SURFACE_FRAGMENT, uniforms: surfaceUniforms }),
  )
  group.add(surface)

  const cloudUniforms = { uSunDirection: sun, uClouds: { value: cloudsMap }, uMasks: { value: masksMap }, uCloudShift: surfaceUniforms.uCloudShift }
  const clouds = new THREE.Mesh(
    new THREE.SphereGeometry(PLANET_RADIUS * 1.012, 160, 108),
    new THREE.ShaderMaterial({
      vertexShader: SPHERE_VERTEX,
      fragmentShader: CLOUD_FRAGMENT,
      uniforms: cloudUniforms,
      transparent: true,
      depthWrite: false,
    }),
  )
  group.add(clouds)
  // Turned so the largest continent (about 217 degrees round the painted map) faces the sunlit
  // side of the opening view, rather than open ocean.
  surface.rotation.y = THREE.MathUtils.degToRad(45 - 217)
  clouds.rotation.y = surface.rotation.y

  const atmosphere = new THREE.Mesh(
    new THREE.SphereGeometry(PLANET_RADIUS * 1.075, 128, 96),
    new THREE.ShaderMaterial({
      vertexShader: SPHERE_VERTEX,
      fragmentShader: ATMOSPHERE_FRAGMENT,
      uniforms: { uSunDirection: sun, uIntensity: { value: 0.8 } },
      side: THREE.BackSide,
      blending: THREE.AdditiveBlending,
      transparent: true,
      depthWrite: false,
    }),
  )
  group.add(atmosphere)

  const moonPivot = new THREE.Group()
  const moon = new THREE.Mesh(
    new THREE.SphereGeometry(11, 64, 48),
    new THREE.ShaderMaterial({ vertexShader: SPHERE_VERTEX, fragmentShader: MOON_FRAGMENT, uniforms: { uSunDirection: sun } }),
  )
  moon.position.set(-330, 70, -210)
  moonPivot.add(moon)
  moonPivot.rotation.z = 0.12
  group.add(moonPivot)

  return {
    group,
    update(time, dt) {
      surface.rotation.y += dt * 0.006
      clouds.rotation.y += dt * 0.0085
      moonPivot.rotation.y += dt * 0.004
      moon.rotation.y += dt * 0.02
      surfaceUniforms.uRotation.value = surface.rotation.y
      surfaceUniforms.uCloudShift.value = (surface.rotation.y - clouds.rotation.y) / (Math.PI * 2)
      void time
    },
    setCityLights(value) {
      surfaceUniforms.uCityLights.value = value
    },
  }
}
