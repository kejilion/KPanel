import * as T from 'three';

type WaterEffects = {
  update: (time: number, camera: T.Camera) => void;
  dispose: () => void;
};

// The pack owns these effects; the runtime owns fog, lighting and Blender assets.
// Each effect consumes the host's animation time so pause/resume remains exact.
export function createUnderwater(scene: T.Scene, renderer: T.WebGLRenderer): WaterEffects {
  const group = new T.Group();
  group.name = 'Water column';
  scene.add(group);
  const geometries: T.BufferGeometry[] = [];
  const materials: T.Material[] = [];
  const timeUniform = { value: 0 };
  const cameraUniform = { value: new T.Vector3() };
  let seed = 910731;
  const random = (): number => {
    seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0;
    return seed / 4294967296;
  };
  const ownGeometry = <G extends T.BufferGeometry>(geometry: G): G => {
    geometries.push(geometry);
    return geometry;
  };
  const ownMaterial = <M extends T.Material>(material: M): M => {
    materials.push(material);
    return material;
  };

  // Camera-facing slices approximate scattered light through roof openings.
  // Gaussian cross sections and long endpoint fades prevent visible cone edges.
  const beamGeometry = ownGeometry(new T.PlaneGeometry(2, 1, 1, 24));
  const beamSpecs = [
    { top: [14, 35, 1], bottom: [-5, -1, -13], width: 3.8, strength: 0.084 },
    { top: [19, 34, -7], bottom: [0, -1, -21], width: 2.4, strength: 0.072 },
    { top: [7, 32, 9], bottom: [-11, -1, -4], width: 4.6, strength: 0.043 },
    { top: [21, 37, 14], bottom: [0, -1, 0], width: 2.8, strength: 0.064 },
    { top: [5, 36, -12], bottom: [-15, -1, -26], width: 5.2, strength: 0.036 },
  ];
  for (let i = 0; i < beamSpecs.length; i++) {
    const spec = beamSpecs[i]!;
    const material = ownMaterial(new T.ShaderMaterial({
      uniforms: {
        uTime: timeUniform,
        uCamera: cameraUniform,
        uTop: { value: new T.Vector3(spec.top[0], spec.top[1], spec.top[2]) },
        uBottom: { value: new T.Vector3(spec.bottom[0], spec.bottom[1], spec.bottom[2]) },
        uWidth: { value: spec.width },
        uStrength: { value: spec.strength },
        uPhase: { value: i * 3.7 },
      },
      transparent: true,
      depthWrite: false,
      side: T.DoubleSide,
      blending: T.AdditiveBlending,
      toneMapped: false,
      vertexShader: `
        uniform float uTime, uWidth, uPhase;
        uniform vec3 uTop, uBottom, uCamera;
        varying vec2 vBeam;
        varying vec3 vWorld;
        void main() {
          float along = uv.y;
          vec3 axis = normalize(uTop - uBottom);
          vec3 center = mix(uBottom, uTop, along);
          vec3 side = normalize(cross(axis, uCamera - center));
          float width = uWidth * mix(1.35, 0.55, along);
          float drift = sin(along * 4.0 + uTime * 0.12 + uPhase) * 0.1;
          vWorld = center + side * (position.x + drift) * width;
          vBeam = vec2(position.x, along);
          gl_Position = projectionMatrix * viewMatrix * vec4(vWorld, 1.0);
        }
      `,
      fragmentShader: `
        uniform float uTime, uStrength, uPhase;
        varying vec2 vBeam;
        varying vec3 vWorld;
        float hash(vec3 p) {
          p = fract(p * 0.1031);
          p += dot(p, p.yzx + 33.33);
          return fract((p.x + p.y) * p.z);
        }
        float noise(vec3 p) {
          vec3 i = floor(p), f = fract(p);
          f = f * f * (3.0 - 2.0 * f);
          return mix(mix(mix(hash(i), hash(i + vec3(1,0,0)), f.x),
                         mix(hash(i + vec3(0,1,0)), hash(i + vec3(1,1,0)), f.x), f.y),
                     mix(mix(hash(i + vec3(0,0,1)), hash(i + vec3(1,0,1)), f.x),
                         mix(hash(i + vec3(0,1,1)), hash(i + vec3(1,1,1)), f.x), f.y), f.z);
        }
        void main() {
          float crossSection = exp(-vBeam.x * vBeam.x * 5.0);
          crossSection *= 1.0 - smoothstep(0.72, 1.0, abs(vBeam.x));
          float ends = smoothstep(0.0, 0.2, vBeam.y) * (1.0 - smoothstep(0.72, 1.0, vBeam.y));
          float cloudShadow = noise(vWorld * vec3(0.2, 0.055, 0.2) + vec3(uTime * 0.024, uPhase, 0));
          float threads = 0.84 + 0.16 * sin(vBeam.x * 14.0 + cloudShadow * 5.0 + uTime * 0.16);
          float alpha = crossSection * ends * mix(0.42, 1.0, cloudShadow) * threads * uStrength;
          gl_FragColor = vec4(vec3(0.46, 0.72, 0.66), alpha);
          #include <colorspace_fragment>
        }
      `,
    }));
    const beam = new T.Mesh(beamGeometry, material);
    beam.name = `Scattered sunlight ${i + 1}`;
    beam.frustumCulled = false;
    beam.renderOrder = 3;
    group.add(beam);
  }

  const dustCount = 940;
  const dustPositions = new Float32Array(dustCount * 3);
  const dustSeeds = new Float32Array(dustCount * 2);
  for (let i = 0; i < dustCount; i++) {
    dustPositions[i * 3] = (random() - 0.5) * 95;
    dustPositions[i * 3 + 1] = random() * 32;
    dustPositions[i * 3 + 2] = (random() - 0.5) * 105 - 8;
    dustSeeds[i * 2] = random();
    dustSeeds[i * 2 + 1] = random();
  }
  const dustGeometry = ownGeometry(new T.BufferGeometry());
  dustGeometry.setAttribute('position', new T.BufferAttribute(dustPositions, 3));
  dustGeometry.setAttribute('aSeed', new T.BufferAttribute(dustSeeds, 2));
  const dustMaterial = ownMaterial(new T.ShaderMaterial({
    uniforms: T.UniformsUtils.merge([
      T.UniformsLib.fog,
      { uPixelRatio: { value: renderer.getPixelRatio() } },
    ]),
    fog: true,
    transparent: true,
    depthWrite: false,
    blending: T.AdditiveBlending,
    toneMapped: false,
    vertexShader: `
      uniform float uTime, uPixelRatio;
      attribute vec2 aSeed;
      varying float vOpacity;
      #include <fog_pars_vertex>
      void main() {
        vec3 p = position;
        p.x += sin(uTime * 0.055 + aSeed.x * 24.0) * 0.7;
        p.z += cos(uTime * 0.044 + aSeed.y * 27.0) * 0.8;
        p.y = mod(p.y + uTime * (0.017 + aSeed.y * 0.018), 32.0);
        vec4 mvPosition = modelViewMatrix * vec4(p, 1.0);
        gl_Position = projectionMatrix * mvPosition;
        gl_PointSize = clamp((24.0 + aSeed.x * 20.0) / max(1.0, -mvPosition.z), 0.55, 2.5) * uPixelRatio;
        vOpacity = (0.14 + aSeed.y * 0.15) * smoothstep(1.0, 5.0, -mvPosition.z) * exp(mvPosition.z * 0.021);
        #include <fog_vertex>
      }
    `,
    fragmentShader: `
      varying float vOpacity;
      #include <fog_pars_fragment>
      void main() {
        float r = length(gl_PointCoord - 0.5) * 2.0;
        float alpha = exp(-r * r * 3.5) * (1.0 - smoothstep(0.6, 1.0, r)) * vOpacity;
        gl_FragColor = vec4(vec3(0.3, 0.59, 0.59), alpha);
        #include <fog_fragment>
        #include <colorspace_fragment>
      }
    `,
  }));
  dustMaterial.uniforms['uTime'] = timeUniform;
  const dust = new T.Points(dustGeometry, dustMaterial);
  dust.name = 'Suspended sediment';
  dust.frustumCulled = false;
  group.add(dust);

  // A thin film on the seabed suggests refracted surface sunlight. It is not
  // attached to imported materials and intentionally does not illuminate walls.
  const causticGeometry = ownGeometry(new T.PlaneGeometry(52, 72));
  const causticMaterial = ownMaterial(new T.ShaderMaterial({
    uniforms: { uTime: timeUniform },
    transparent: true,
    depthWrite: false,
    blending: T.AdditiveBlending,
    toneMapped: false,
    polygonOffset: true,
    polygonOffsetFactor: -1,
    polygonOffsetUnits: -1,
    vertexShader: `
      varying vec3 vWorld;
      varying vec2 vUv;
      void main() {
        vUv = uv;
        vWorld = (modelMatrix * vec4(position, 1.0)).xyz;
        gl_Position = projectionMatrix * viewMatrix * vec4(vWorld, 1.0);
      }
    `,
    fragmentShader: `
      uniform float uTime;
      varying vec3 vWorld;
      varying vec2 vUv;
      vec2 hash(vec2 p) {
        return fract(sin(vec2(dot(p, vec2(127.1, 311.7)), dot(p, vec2(269.5, 183.3)))) * 43758.5453);
      }
      float cellEdge(vec2 p) {
        vec2 base = floor(p), local = fract(p);
        float first = 8.0, second = 8.0;
        for (int y = -1; y <= 1; y++) {
          for (int x = -1; x <= 1; x++) {
            vec2 grid = vec2(float(x), float(y));
            vec2 h = hash(base + grid);
            vec2 center = 0.5 + 0.29 * sin(h * 6.283185 + uTime * 0.24);
            vec2 r = grid + center - local;
            float d = dot(r, r);
            if (d < first) { second = first; first = d; }
            else { second = min(second, d); }
          }
        }
        return exp(-abs(sqrt(second) - sqrt(first)) * 40.0);
      }
      void main() {
        vec2 p = vWorld.xz * 0.43;
        p += 0.23 * vec2(sin(p.y * 1.7 + uTime * 0.11), cos(p.x * 1.6 - uTime * 0.13));
        float light = cellEdge(p) * 0.67 + cellEdge(p * 1.51 + vec2(6.3, -3.9)) * 0.33;
        vec2 edge = smoothstep(vec2(0.0), vec2(0.17), vUv) * (1.0 - smoothstep(vec2(0.83), vec2(1.0), vUv));
        float lightPatch = exp(-dot(vWorld.xz - vec2(-1.0, -6.0), vWorld.xz - vec2(-1.0, -6.0)) * 0.0018);
        gl_FragColor = vec4(vec3(0.25, 0.64, 0.55), light * edge.x * edge.y * lightPatch * 0.14);
        #include <colorspace_fragment>
      }
    `,
  }));
  const caustics = new T.Mesh(causticGeometry, causticMaterial);
  caustics.name = 'Surface refraction on seabed';
  caustics.rotation.x = -Math.PI / 2;
  caustics.position.set(0, 0.045, -6);
  caustics.renderOrder = 1;
  group.add(caustics);

  // Low polygon solid fish with a forked tail, rather than point sprites.
  const fishVertices: number[] = [];
  const fishIndices: number[] = [];
  const sections: ReadonlyArray<readonly [number, number]> = [
    [-0.49, 0.028], [-0.26, 0.11], [0.12, 0.145], [0.4, 0.093], [0.58, 0.009],
  ];
  const radialSegments = 10;
  for (const [x, radius] of sections) {
    for (let j = 0; j < radialSegments; j++) {
      const angle = j / radialSegments * Math.PI * 2;
      fishVertices.push(x, Math.cos(angle) * radius, Math.sin(angle) * radius * 0.53);
    }
  }
  for (let ring = 0; ring < sections.length - 1; ring++) {
    for (let j = 0; j < radialSegments; j++) {
      const a = ring * radialSegments + j;
      const b = ring * radialSegments + (j + 1) % radialSegments;
      const c = a + radialSegments;
      const d = b + radialSegments;
      fishIndices.push(a, b, c, b, d, c);
    }
  }
  const tail = fishVertices.length / 3;
  fishVertices.push(-0.43, 0, 0, -0.79, 0.23, 0, -0.68, 0, 0, -0.79, -0.23, 0);
  fishIndices.push(tail, tail + 1, tail + 2, tail, tail + 2, tail + 3);
  const dorsal = fishVertices.length / 3;
  fishVertices.push(0.15, 0.11, 0, -0.17, 0.24, 0, -0.3, 0.07, 0);
  fishIndices.push(dorsal, dorsal + 1, dorsal + 2);
  const fishGeometry = ownGeometry(new T.BufferGeometry());
  fishGeometry.setAttribute('position', new T.Float32BufferAttribute(fishVertices, 3));
  fishGeometry.setIndex(fishIndices);
  fishGeometry.computeVertexNormals();
  const fishMaterial = ownMaterial(new T.MeshStandardMaterial({
    color: 0x385963,
    roughness: 0.52,
    metalness: 0.2,
    side: T.DoubleSide,
  }));
  fishMaterial.onBeforeCompile = (shader) => {
    shader.uniforms['uWaterTime'] = timeUniform;
    shader.vertexShader = `uniform float uWaterTime;\n${shader.vertexShader}`;
    shader.vertexShader = shader.vertexShader.replace('#include <begin_vertex>', `
      #include <begin_vertex>
      float tailWeight = pow(clamp(-position.x + 0.2, 0.0, 1.0), 2.0);
      transformed.z += sin(uWaterTime * 3.6 + instanceMatrix[3].x * 0.9 + position.x * 7.0) * tailWeight * 0.08;
    `);
  };
  fishMaterial.customProgramCacheKey = () => 'abyssal-fish-tail-v1';
  const schools = [
    { count: 34, center: new T.Vector3(0, 12, -6), rx: 13, rz: 8, speed: 0.048, offset: 0.7 },
    { count: 22, center: new T.Vector3(-17, 6, 3), rx: 6, rz: 12, speed: 0.043, offset: 2.1 },
    { count: 26, center: new T.Vector3(6, 6.8, -23), rx: 11, rz: 4.5, speed: 0.053, offset: 4.0 },
  ];
  const fishData = schools.flatMap((school) => Array.from({ length: school.count }, () => ({
    school,
    phase: (random() - 0.5) * 0.88,
    height: (random() - 0.5) * 2.4,
    radius: (random() - 0.5) * 2,
    size: 0.38 + random() * 0.36,
  })));
  const fish = new T.InstancedMesh(fishGeometry, fishMaterial, fishData.length);
  fish.name = 'Quiet shoals';
  fish.instanceMatrix.setUsage(T.DynamicDrawUsage);
  fish.frustumCulled = false;
  group.add(fish);
  const transform = new T.Object3D();
  const forward = new T.Vector3(1, 0, 0);
  const tangent = new T.Vector3();

  let disposed = false;
  const update = (time: number, camera: T.Camera): void => {
    if (disposed) return;
    timeUniform.value = time;
    camera.getWorldPosition(cameraUniform.value);
    dustMaterial.uniforms['uPixelRatio']!.value = renderer.getPixelRatio();
    for (let i = 0; i < fishData.length; i++) {
      const data = fishData[i]!;
      const school = data.school;
      const angle = time * school.speed + school.offset + data.phase;
      const radiusX = school.rx + data.radius;
      const radiusZ = school.rz + data.radius * 0.6;
      transform.position.set(
        school.center.x + Math.cos(angle) * radiusX,
        school.center.y + data.height + Math.sin(time * 0.15 + data.phase * 3) * 0.35,
        school.center.z + Math.sin(angle) * radiusZ,
      );
      tangent.set(-Math.sin(angle) * radiusX, Math.cos(time * 0.15 + data.phase * 3) * 0.12, Math.cos(angle) * radiusZ).normalize();
      transform.quaternion.setFromUnitVectors(forward, tangent);
      transform.scale.setScalar(data.size);
      transform.updateMatrix();
      fish.setMatrixAt(i, transform.matrix);
    }
    fish.instanceMatrix.needsUpdate = true;
  };
  return {
    update,
    dispose: () => {
      if (disposed) return;
      disposed = true;
      scene.remove(group);
      fish.dispose();
      for (const geometry of geometries) geometry.dispose();
      for (const material of materials) material.dispose();
      group.clear();
    },
  };
}
