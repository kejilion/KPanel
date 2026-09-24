import * as THREE from 'three'
import { mergeGeometries } from 'three/examples/jsm/utils/BufferGeometryUtils.js'
import { NOISE_GLSL } from './noise'
import { LIGHTING_GLSL, type LightingUniforms } from './shading'
import { ROCKS } from './world'

/**
 * The sea stacks: faceted, layered sandstone, dark and weedy where the waves
 * wash, pale with salt on the ledges. They catch the low sun warmly and stand
 * as silhouettes against the sunset and the moonlit sea.
 */
const VERTEX = /* glsl */ `
varying vec3 vWorld;
varying vec3 vNormal;
void main() {
  vec4 world = modelMatrix * vec4(position, 1.0);
  vWorld = world.xyz;
  vNormal = normalize(mat3(modelMatrix) * normal);
  gl_Position = projectionMatrix * viewMatrix * world;
}
`

const FRAGMENT = /* glsl */ `
varying vec3 vWorld;
varying vec3 vNormal;
${NOISE_GLSL}
${LIGHTING_GLSL}
void main() {
  // Flat facets from the geometry, fine bumps on top.
  vec3 n = normalize(normalize(vNormal) + vec3(snoise(vWorld * 0.7), snoise(vWorld * 0.7 + 7.0), snoise(vWorld * 0.7 + 13.0)) * 0.12
    + vec3(snoise(vWorld * 2.1), snoise(vWorld * 2.1 + 3.0), snoise(vWorld * 2.1 + 9.0)) * 0.06);
  float strata = sin(vWorld.y * 1.3 + snoise(vWorld * 0.05) * 2.0) * 0.5 + 0.5;
  float grain = snoise(vWorld * 0.6) * 0.5 + 0.5;
  float rough = snoise(vWorld * 0.18) * 0.5 + 0.5;
  vec3 albedo = mix(vec3(0.3, 0.24, 0.19), vec3(0.55, 0.45, 0.34), strata * 0.22 + grain * 0.38 + rough * 0.4) * mix(0.72, 1.08, snoise(vWorld * 0.07) * 0.5 + 0.5);
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

export function createRocks(uniforms: LightingUniforms): THREE.Mesh {
  const parts: THREE.BufferGeometry[] = []
  const v = new THREE.Vector3()
  for (const rock of ROCKS) {
    // Not merged: every face keeps its own normal, so the stone breaks into flat facets.
    const geometry = new THREE.IcosahedronGeometry(1, rock.height > 10 ? 4 : 2).deleteAttribute('normal').deleteAttribute('uv')
    const position = geometry.getAttribute('position')
    for (let index = 0; index < position.count; index++) {
      v.fromBufferAttribute(position, index)
      const s = rock.seed
      // Jagged, weathered stone: several octaves of lumps and a few sharp facets.
      const lump = 1 + 0.26 * Math.sin(v.x * 3.1 + s) * Math.sin(v.y * 2.7 + s * 1.3) * Math.sin(v.z * 3.3 + s * 0.7)
        + 0.12 * Math.sin(v.x * 7.3 + s * 2) * Math.sin(v.z * 6.1 + s) + 0.06 * Math.sin(v.y * 13 + v.x * 5 + s)
        - 0.1 * Math.abs(Math.sin(v.x * 4.7 + v.z * 3.9 + s * 3))
      // Steep sides stepped into ledges along the layers, a rounded top, a wide foot under the water.
      const top = Math.max(0, v.y)
      const layer = v.y * 3.4 + Math.sin(v.x * 2.3 + s) * 0.45 + Math.sin(v.z * 3.1 + s * 1.7) * 0.3
      const ledge = v.y > -0.2 ? 1 - 0.055 * (layer - Math.floor(layer)) : 1
      const across = lump * ledge * (1 - top * 0.25)
      position.setXYZ(index, v.x * across * rock.radius, (v.y > 0 ? Math.pow(v.y, 0.7) : v.y * 0.5) * rock.height, v.z * across * rock.radius)
    }
    geometry.computeVertexNormals()
    geometry.translate(rock.x, -rock.height * 0.12, rock.z)
    parts.push(geometry)
  }
  const mesh = new THREE.Mesh(mergeGeometries(parts)!, new THREE.ShaderMaterial({
    uniforms: { ...uniforms },
    vertexShader: VERTEX,
    fragmentShader: FRAGMENT,
  }))
  mesh.frustumCulled = false
  return mesh
}
