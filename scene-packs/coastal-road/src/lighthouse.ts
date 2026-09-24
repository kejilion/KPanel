import * as THREE from 'three'
import { mergeGeometries } from 'three/examples/jsm/utils/BufferGeometryUtils.js'
import { glowPoints } from './road'
import type { LightingUniforms } from './shading'
import { colored, paintedMaterial } from './terrain'
import { LIGHTHOUSE } from './world'

/**
 * The lighthouse on the headland: a banded tower with a gallery and a lantern,
 * a keeper's cottage, and at night two soft beams turning slowly over the sea.
 * The lantern glows steadily; nothing flashes.
 */
const BEAM_VERTEX = /* glsl */ `
varying vec3 vWorld;
varying vec3 vNormal;
varying float vAlong;
void main() {
  vec4 world = modelMatrix * vec4(position, 1.0);
  vWorld = world.xyz;
  vNormal = normalize(mat3(modelMatrix) * normal);
  vAlong = position.y;
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const BEAM_FRAGMENT = /* glsl */ `
uniform float uNight;
varying vec3 vWorld;
varying vec3 vNormal;
varying float vAlong;
void main() {
  vec3 view = normalize(cameraPosition - vWorld);
  float core = pow(abs(dot(normalize(vNormal), view)), 2.5);
  float fade = (1.0 - vAlong) * smoothstep(0.0, 0.03, vAlong);
  gl_FragColor = vec4(vec3(1.0, 0.92, 0.75) * core * fade * fade * uNight * 0.09, 1.0);
}
`

export function createLighthouse(uniforms: LightingUniforms): THREE.Group & { update(time: number, night: number): void } {
  const group = new THREE.Group()
  group.position.copy(LIGHTHOUSE)
  const white = new THREE.Color().setRGB(0.82, 0.8, 0.74)
  const red = new THREE.Color().setRGB(0.55, 0.08, 0.06)
  const iron = new THREE.Color().setRGB(0.12, 0.13, 0.14)
  const parts: THREE.BufferGeometry[] = []
  const bands = 5
  for (let band = 0; band < bands; band++) {
    const bottom = 4.4 - band * 0.28
    const top = 4.4 - (band + 1) * 0.28
    const segment = new THREE.CylinderGeometry(top, bottom, 4.8, 20)
    segment.translate(0, 2.4 + band * 4.8, 0)
    parts.push(colored(segment, band % 2 === 0 ? white : red))
  }
  const gallery = new THREE.CylinderGeometry(3.6, 3.6, 0.4, 20)
  gallery.translate(0, 24.2, 0)
  parts.push(colored(gallery, iron))
  const lantern = new THREE.CylinderGeometry(2.1, 2.1, 3, 12)
  lantern.translate(0, 25.9, 0)
  parts.push(colored(lantern, new THREE.Color().setRGB(0.35, 0.33, 0.24)))
  const cap = new THREE.ConeGeometry(2.6, 2.4, 12)
  cap.translate(0, 28.6, 0)
  parts.push(colored(cap, red))
  const cottage = new THREE.BoxGeometry(9, 4, 6)
  cottage.translate(-9, 2, 6)
  parts.push(colored(cottage, white))
  const roof = new THREE.CylinderGeometry(0.01, 5.4, 3, 4, 1)
  roof.rotateY(Math.PI / 4)
  roof.scale(1.3, 1, 0.9)
  roof.translate(-9, 5.5, 6)
  parts.push(colored(roof, red))
  group.add(new THREE.Mesh(mergeGeometries(parts)!, paintedMaterial(uniforms, 0.25)))

  const lamp = glowPoints(uniforms, 3)
  lamp.positions.set([0, 25.9, 0, -9, 2.2, 9.1, -6, 2.2, 9.1])
  lamp.sizes.set([6, 1.2, 1.2])
  group.add(lamp.points)

  const beamGeometry = new THREE.CylinderGeometry(13, 0.8, 1, 24, 1, true)
  beamGeometry.translate(0, 0.5, 0)
  beamGeometry.rotateZ(-Math.PI / 2) // along +x
  const beamMaterial = new THREE.ShaderMaterial({
    uniforms: { uNight: uniforms.uNight },
    vertexShader: BEAM_VERTEX.replace('vAlong = position.y;', 'vAlong = position.x;'),
    fragmentShader: BEAM_FRAGMENT,
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
    side: THREE.DoubleSide,
  })
  const beams = new THREE.Group()
  beams.position.set(0, 25.9, 0)
  for (const angle of [0, Math.PI]) {
    const beam = new THREE.Mesh(beamGeometry, beamMaterial)
    beam.scale.set(560, 1, 1)
    beam.rotation.y = angle
    beam.rotation.z = 0.025 // level or a touch upward, so it sweeps the sea and sky rather than the ground
    beam.frustumCulled = false
    beams.add(beam)
  }
  group.add(beams)

  return Object.assign(group, {
    update(time: number, night: number) {
      beams.rotation.y = time * 0.3
      beams.visible = night > 0.02
      const glow = 2.6 * night
      lamp.colors.set([glow * 1.1, glow * 0.95, glow * 0.7, night * 2, night * 1.4, night * 0.8, night * 2, night * 1.4, night * 0.8])
      lamp.update()
    },
  })
}
