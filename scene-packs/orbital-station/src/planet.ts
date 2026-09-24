import * as THREE from 'three'
import { NOISE_GLSL } from './noise'

export const PLANET_RADIUS = 100

const SPHERE_VERTEX = /* glsl */ `
varying vec3 vObject;
varying vec3 vWorldNormal;
varying vec3 vWorldPosition;
void main() {
  vObject = normalize(position);
  vec4 world = modelMatrix * vec4(position, 1.0);
  vWorldPosition = world.xyz;
  vWorldNormal = normalize(mat3(modelMatrix) * normal);
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const SURFACE_FRAGMENT = /* glsl */ `
uniform vec3 uSunDirection;
uniform float uCityLights;
varying vec3 vObject;
varying vec3 vWorldNormal;
varying vec3 vWorldPosition;
${NOISE_GLSL}
void main() {
  vec3 p = vObject;
  float continents = fbm(p * 1.45 + vec3(3.1, 0.0, 1.7));
  float height = continents + fbm(p * 7.0) * 0.2;
  float land = smoothstep(0.11, 0.14, height);
  float latitude = abs(p.y);

  vec3 ocean = mix(vec3(0.004, 0.022, 0.08), vec3(0.02, 0.2, 0.33), smoothstep(-0.05, 0.13, height));
  float dryness = smoothstep(0.05, 0.6, fbm3(p * 3.0 + 7.0) + (0.45 - latitude) * 0.5);
  vec3 ground = mix(vec3(0.07, 0.2, 0.06), vec3(0.6, 0.44, 0.24), dryness);
  ground = mix(ground, vec3(0.36, 0.33, 0.31), smoothstep(0.34, 0.52, height));
  vec3 albedo = mix(ocean, ground, land);
  float ice = smoothstep(0.76, 0.86, latitude + fbm3(p * 4.0) * 0.1);
  albedo = mix(albedo, vec3(0.9, 0.94, 1.0), ice);

  vec3 N = normalize(vWorldNormal);
  vec3 L = normalize(uSunDirection);
  vec3 V = normalize(cameraPosition - vWorldPosition);
  float ndl = dot(N, L);
  float daylight = smoothstep(-0.1, 0.3, ndl);
  vec3 color = albedo * (0.015 + 1.45 * max(ndl, 0.0));
  // Warm light along the terminator.
  color += albedo * vec3(1.0, 0.38, 0.12) * smoothstep(0.28, 0.0, abs(ndl)) * 0.4;

  vec3 H = normalize(L + V);
  float glint = pow(max(dot(N, H), 0.0), 160.0) * (1.0 - land) * (1.0 - ice);
  color += vec3(1.0, 0.88, 0.72) * glint * 1.1 * daylight;

  // City lights gather on temperate coasts of the night side.
  float cities = smoothstep(0.5, 0.8, fbm3(p * 26.0)) * smoothstep(0.62, 0.3, dryness);
  cities *= land * (1.0 - ice) * (1.0 - smoothstep(0.03, 0.34, height - 0.05));
  float night = 1.0 - smoothstep(-0.22, 0.04, ndl);
  color += vec3(1.0, 0.6, 0.26) * cities * night * uCityLights * 2.4;

  float rim = pow(1.0 - max(dot(N, V), 0.0), 3.0);
  color = mix(color, vec3(0.25, 0.5, 1.0) * (0.04 + daylight * 0.8), rim * 0.45);
  gl_FragColor = vec4(color, 1.0);
  #include <tonemapping_fragment>
  #include <colorspace_fragment>
}
`

const CLOUD_FRAGMENT = /* glsl */ `
uniform vec3 uSunDirection;
uniform float uTime;
varying vec3 vObject;
varying vec3 vWorldNormal;
varying vec3 vWorldPosition;
${NOISE_GLSL}
void main() {
  vec3 p = vObject;
  float drift = uTime * 0.012;
  // Stretched along latitude so weather reads as bands and swirls.
  float cover = fbm(p * vec3(2.2, 4.6, 2.2) + vec3(drift, drift * 0.3, -drift * 0.6));
  cover += fbm3(p * 8.0 - vec3(drift * 2.0)) * 0.3;
  float alpha = smoothstep(0.16, 0.5, cover) * 0.88;
  vec3 N = normalize(vWorldNormal);
  float ndl = dot(N, normalize(uSunDirection));
  float lit = smoothstep(-0.12, 0.35, ndl);
  vec3 color = mix(vec3(0.02, 0.025, 0.04), vec3(1.0), lit);
  color += vec3(1.0, 0.42, 0.18) * smoothstep(0.22, 0.0, abs(ndl)) * 0.5;
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

export function createPlanet(sunDirection: THREE.Vector3): Planet {
  const group = new THREE.Group()
  const sun = { value: sunDirection }

  const surfaceUniforms = { uSunDirection: sun, uCityLights: { value: 1 } }
  const surface = new THREE.Mesh(
    new THREE.SphereGeometry(PLANET_RADIUS, 192, 128),
    new THREE.ShaderMaterial({ vertexShader: SPHERE_VERTEX, fragmentShader: SURFACE_FRAGMENT, uniforms: surfaceUniforms }),
  )
  group.add(surface)

  const cloudUniforms = { uSunDirection: sun, uTime: { value: 0 } }
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
      cloudUniforms.uTime.value = time
    },
    setCityLights(value) {
      surfaceUniforms.uCityLights.value = value
    },
  }
}
