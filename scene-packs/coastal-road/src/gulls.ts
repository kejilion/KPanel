import * as THREE from 'three'
import { LIGHTING_GLSL, type LightingUniforms } from './shading'
import { HEADLAND_Z, coastX } from './world'

/**
 * Seagulls riding the wind off the cliffs: wide lazy circles, mostly gliding,
 * with a few slow wingbeats now and then. They go to roost at night.
 */
const VERTEX = /* glsl */ `
uniform float uTime;
attribute float aPhase;
varying vec3 vWorld;
varying vec3 vNormal;
void main() {
  vec3 p = position;
  // Wings bend at the shoulder: mostly held out, a few beats every so often.
  float beating = smoothstep(0.6, 0.8, sin(uTime * 0.45 + aPhase * 5.0));
  float flap = sin(uTime * 7.0 + aPhase * 11.0) * beating * 0.45 + 0.08;
  p.y += abs(p.z) * flap;
  vec4 world = modelMatrix * instanceMatrix * vec4(p, 1.0);
  vWorld = world.xyz;
  vNormal = normalize(mat3(modelMatrix) * mat3(instanceMatrix) * normal);
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const FRAGMENT = /* glsl */ `
varying vec3 vWorld;
varying vec3 vNormal;
${LIGHTING_GLSL}
void main() {
  vec3 n = normalize(vNormal);
  if (!gl_FrontFacing) n = -n;
  vec3 color = shade(vec3(0.85, 0.86, 0.86), n, 0.6, 1.0);
  gl_FragColor = vec4(atmosphere(color, vWorld), 1.0);
}
`

function gullGeometry(): THREE.BufferGeometry {
  // A flat gull: a slim body and two swept wings (the wings bend in the shader).
  const positions = [
    0.45, 0, 0, -0.35, 0, 0.08, -0.35, 0, -0.08, // body
    0.12, 0, 0, -0.18, 0, 0, 0.0, 0, 0.9, // right wing
    0.12, 0, 0, 0.0, 0, -0.9, -0.18, 0, 0, // left wing
  ]
  const geometry = new THREE.BufferGeometry()
  geometry.setAttribute('position', new THREE.Float32BufferAttribute(positions, 3))
  geometry.computeVertexNormals()
  return geometry
}

interface Gull {
  centre: THREE.Vector3
  radius: number
  speed: number
  phase: number
  height: number
}

export function createGulls(uniforms: LightingUniforms, random: () => number, count = 16): THREE.InstancedMesh & { update(time: number, night: number): void } {
  const geometry = gullGeometry()
  const phases = new Float32Array(count)
  const gulls: Gull[] = []
  for (let index = 0; index < count; index++) {
    phases[index] = random()
    const z = HEADLAND_Z + (random() - 0.6) * 700
    gulls.push({
      centre: new THREE.Vector3(coastX(z) - 40 - random() * 160, 0, z),
      radius: 25 + random() * 70,
      speed: (0.12 + random() * 0.1) * (random() < 0.5 ? -1 : 1),
      phase: random() * Math.PI * 2,
      height: 30 + random() * 45,
    })
  }
  geometry.setAttribute('aPhase', new THREE.InstancedBufferAttribute(phases, 1))
  const mesh = new THREE.InstancedMesh(geometry, new THREE.ShaderMaterial({
    uniforms: { ...uniforms },
    vertexShader: VERTEX,
    fragmentShader: FRAGMENT,
    side: THREE.DoubleSide,
  }), count)
  mesh.frustumCulled = false
  const matrix = new THREE.Matrix4()
  const position = new THREE.Vector3()
  const quaternion = new THREE.Quaternion()
  const euler = new THREE.Euler()
  const scale = new THREE.Vector3(1.1, 1.1, 1.1)
  return Object.assign(mesh, {
    update(time: number, night: number) {
      mesh.visible = night < 0.9
      gulls.forEach((gull, index) => {
        const angle = gull.phase + time * gull.speed
        position.set(gull.centre.x + Math.cos(angle) * gull.radius, gull.height + Math.sin(time * 0.3 + gull.phase) * 4, gull.centre.z + Math.sin(angle) * gull.radius)
        // Heading along the circle, banked into the turn.
        const heading = Math.atan2(-Math.cos(angle) * Math.sign(gull.speed), -Math.sin(angle) * Math.sign(gull.speed))
        quaternion.setFromEuler(euler.set(-0.35 * Math.sign(gull.speed), heading, 0, 'YXZ'))
        mesh.setMatrixAt(index, matrix.compose(position, quaternion, scale))
      })
      mesh.instanceMatrix.needsUpdate = true
    },
  })
}
