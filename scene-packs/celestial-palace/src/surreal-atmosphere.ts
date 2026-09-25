import * as T from 'three'
import { Reflector } from 'three/examples/jsm/objects/Reflector.js'
import { random } from './math'

const noise = `
float hash3(vec3 p){p=fract(p*.3183099+vec3(.13,.37,.73));p*=17.;return fract(p.x*p.y*p.z*(p.x+p.y+p.z));}
float noise3(vec3 p){vec3 i=floor(p),f=fract(p);f=f*f*(3.-2.*f);return mix(mix(mix(hash3(i),hash3(i+vec3(1,0,0)),f.x),mix(hash3(i+vec3(0,1,0)),hash3(i+vec3(1,1,0)),f.x),f.y),mix(mix(hash3(i+vec3(0,0,1)),hash3(i+vec3(1,0,1)),f.x),mix(hash3(i+vec3(0,1,1)),hash3(i+vec3(1,1,1)),f.x),f.y),f.z);}
float fbm(vec3 p){return .55*noise3(p)+.29*noise3(p*2.03)+.16*noise3(p*4.13);}
`

function waveTexture(): T.DataTexture {
  const size = 256, data = new Uint8Array(size * size * 4)
  for (let y = 0; y < size; y++) for (let x = 0; x < size; x++) {
    const u = x / size * Math.PI * 2, v = y / size * Math.PI * 2
    let dx = 0, dy = 0
    for (let j = 0; j < 7; j++) {
      const kx = [3, 5, 8, 13, 19, 29, 41][j]!, ky = [2, -3, 5, -8, 13, -19, 29][j]!
      const q = Math.cos(u * kx + v * ky + j * 1.7) * .13 / (1 + j * .8)
      dx += q; dy += q * ky / kx
    }
    const n = new T.Vector3(-dx, -dy, 1).normalize(), p = (y * size + x) * 4
    data[p] = (n.x * .5 + .5) * 255; data[p + 1] = (n.y * .5 + .5) * 255
    data[p + 2] = (n.z * .5 + .5) * 255; data[p + 3] = 255
  }
  const texture = new T.DataTexture(data, size, size, T.RGBAFormat)
  texture.wrapS = texture.wrapT = T.RepeatWrapping
  texture.magFilter = T.LinearFilter; texture.minFilter = T.LinearMipmapLinearFilter
  texture.generateMipmaps = true; texture.needsUpdate = true
  return texture
}

/** Procedural environment; the caller owns time, rendering, and pause state. */
export function createAtmosphere(scene: T.Scene, renderer: T.WebGLRenderer): { update: (time: number, camera: T.Camera) => void; dispose: () => void } {
  const group = new T.Group(); group.name = 'Surreal moon environment'; scene.add(group)
  const animated: T.ShaderMaterial[] = []
  const sky = new T.Mesh(new T.SphereGeometry(2600, 40, 20), new T.ShaderMaterial({
    side: T.BackSide, depthWrite: false, fog: false,
    vertexShader: `varying vec3 vDirection;void main(){vDirection=position;gl_Position=projectionMatrix*modelViewMatrix*vec4(position,1.);}`,
    fragmentShader: `${noise} varying vec3 vDirection;void main(){vec3 d=normalize(vDirection);float h=max(d.y,0.);vec3 c=mix(vec3(.021,.062,.080),vec3(.003,.012,.025),smoothstep(0.,.82,h));float haze=pow(1.-abs(d.y),9.);c+=vec3(.003,.006,.009)*haze;float veil=fbm(d*5.3+vec3(3.,1.,0.));c+=vec3(.004,.008,.010)*pow(veil,3.);gl_FragColor=vec4(c,1.);}`,
  })); sky.renderOrder = -20; group.add(sky)

  const moon = new T.Mesh(new T.SphereGeometry(36, 96, 64), new T.ShaderMaterial({
    fog: false,
    vertexShader: `varying vec3 vP;varying vec3 vN;varying vec3 vW;void main(){vP=position;vN=normalize(mat3(modelMatrix)*normal);vW=(modelMatrix*vec4(position,1.)).xyz;gl_Position=projectionMatrix*viewMatrix*vec4(vW,1.);}`,
    fragmentShader: `${noise} varying vec3 vP;varying vec3 vN;varying vec3 vW;void main(){vec3 p=vP*.078;float sea=fbm(p*1.8+vec3(4.,2.,7.));float crust=fbm(p*8.);float micro=noise3(p*35.);float maria=smoothstep(.47,.69,sea);float facing=max(0.,dot(normalize(vN),normalize(cameraPosition-vW)));float relief=.90+.10*crust+.035*micro;vec3 c=mix(vec3(.91,.97,1.00),vec3(.37,.47,.53),maria*.73);c*=relief*(.64+.36*pow(facing,.28));gl_FragColor=vec4(c,1.);}`,
  })); moon.position.set(28, 76, -125); group.add(moon)

  const halo = new T.Mesh(new T.PlaneGeometry(155, 155), new T.ShaderMaterial({
    transparent: true, blending: T.AdditiveBlending, depthWrite: false, side: T.DoubleSide,
    vertexShader: `varying vec2 vUv;void main(){vUv=uv;gl_Position=projectionMatrix*modelViewMatrix*vec4(position,1.);}`,
    fragmentShader: `varying vec2 vUv;void main(){float r=length(vUv-.5)*2.;float a=exp(-r*r*5.5)*.13;gl_FragColor=vec4(.58,.72,.78,a);}`,
  })); halo.position.copy(moon.position).add(new T.Vector3(0, 0, -38)); group.add(halo)

  const rng = random(845), starPositions: number[] = []
  for (let i = 0; i < 240; i++) {
    const a = rng() * Math.PI * 2, y = .2 + rng() * .75, r = Math.sqrt(1 - y * y) * 1000
    starPositions.push(Math.cos(a) * r, y * 1000, Math.sin(a) * r)
  }
  const stars = new T.BufferGeometry(); stars.setAttribute('position', new T.Float32BufferAttribute(starPositions, 3))
  group.add(new T.Points(stars, new T.PointsMaterial({ color: '#b7d1d4', size: .55, transparent: true, opacity: .45, fog: false, depthWrite: false })))

  // Finite cloud banks are ray-marched inside their volume: no opaque sphere lobes.
  const cloudFragment = `${noise}
uniform float time;uniform vec3 center;uniform vec3 extent;varying vec3 vWorld;
float density(vec3 p){vec3 q=(p-center)/extent;float edge=1.-smoothstep(.35,1.,length(q*vec3(.85,1.1,.85)));vec3 n=p*.048+vec3(time*.004,0.,time*.001);float billow=fbm(n);return edge*smoothstep(.32,.72,billow)*1.8;}
void main(){vec3 rd=normalize(vWorld-cameraPosition);vec3 p=vWorld+rd*.12;vec3 lo=center-extent,hi=center+extent;vec3 a=(lo-p)/rd,b=(hi-p)/rd;vec3 tmax=max(a,b);float limit=min(tmax.x,min(tmax.y,tmax.z));float stepSize=max(limit,0.)/24.;vec3 col=vec3(0.);float opacity=0.;for(int i=0;i<24;i++){float d=density(p);if(d>.01){float lit=clamp((d-density(p+vec3(2.,5.,-3.)))*2.3,0.,1.);float top=clamp((p.y-center.y)/extent.y*.5+.5,0.,1.);vec3 c=mix(vec3(.025,.055,.075),vec3(.20,.31,.36),lit*.73+top*.27);float alpha=1.-exp(-d*stepSize*.16);col+=(1.-opacity)*c*alpha;opacity+=(1.-opacity)*alpha;}p+=rd*stepSize;if(opacity>.98)break;}gl_FragColor=vec4(col/max(opacity,.0001),opacity);}`
  const banks = [
    [-115, 1, -100, 65, 22, 52], [112, 6, -118, 65, 26, 55], [-205, 23, -195, 88, 43, 70],
    [200, 27, -195, 83, 46, 65], [0, -8, -260, 110, 13, 60],
  ]
  for (const values of banks) {
    const [x, y, z, sx, sy, sz] = values as [number, number, number, number, number, number]
    const material = new T.ShaderMaterial({
      uniforms: { time: { value: 0 }, center: { value: new T.Vector3(x, y, z) }, extent: { value: new T.Vector3(sx, sy, sz) } },
      transparent: true, depthWrite: false, side: T.FrontSide,
      vertexShader: `varying vec3 vWorld;void main(){vWorld=(modelMatrix*vec4(position,1.)).xyz;gl_Position=projectionMatrix*viewMatrix*vec4(vWorld,1.);}`,
      fragmentShader: cloudFragment,
    })
    const cloud = new T.Mesh(new T.BoxGeometry(sx * 2, sy * 2, sz * 2), material)
    cloud.position.set(x, y, z); group.add(cloud); animated.push(material)
  }

  const normals = waveTexture()
  const reflectionSize = Math.min(1024, renderer.capabilities.maxTextureSize)
  const ocean = new Reflector(new T.PlaneGeometry(2400, 2400), {
    textureWidth: reflectionSize, textureHeight: reflectionSize, clipBias: .003, multisample: 0,
    shader: {
      name: 'Moon ocean reflection',
      uniforms: { tDiffuse: { value: null }, color: { value: new T.Color('#356571') }, textureMatrix: { value: new T.Matrix4() }, normalMap: { value: normals }, time: { value: 0 } },
      vertexShader: `uniform mat4 textureMatrix;varying vec4 vMirror;varying vec3 vWorld;void main(){vec4 w=modelMatrix*vec4(position,1.);vWorld=w.xyz;vMirror=textureMatrix*vec4(position,1.);gl_Position=projectionMatrix*viewMatrix*w;}`,
      fragmentShader: `uniform sampler2D tDiffuse;uniform sampler2D normalMap;uniform float time;varying vec4 vMirror;varying vec3 vWorld;
void main(){vec2 p=vWorld.xz*.025;vec3 na=texture2D(normalMap,p+vec2(time*.006,time*.002)).rgb*2.-1.;vec3 nb=texture2D(normalMap,p*.63+vec2(-time*.003,time*.004)).rgb*2.-1.;vec3 n=normalize(vec3((na.x+nb.x)*.45,1.,(na.y+nb.y)*.45));vec3 eye=normalize(cameraPosition-vWorld);float distanceToEye=length(cameraPosition-vWorld);vec2 distortion=n.xz*(.002+.05/max(distanceToEye,1.));vec2 reflectionUv=vMirror.xy/vMirror.w+distortion;vec3 reflected=texture2D(tDiffuse,reflectionUv).rgb;float edge=smoothstep(0.,.045,reflectionUv.x)*smoothstep(0.,.045,reflectionUv.y)*(1.-smoothstep(.955,1.,reflectionUv.x))*(1.-smoothstep(.955,1.,reflectionUv.y));reflected=mix(vec3(.021,.062,.080),reflected,edge);float fresnel=.45+.50*pow(1.-max(dot(n,eye),0.),3.);vec3 base=vec3(.011,.041,.048);vec3 light=normalize(vec3(28.,101.,-125.)-vec3(vWorld.x,0.,vWorld.z));float gleam=pow(max(dot(reflect(-light,n),eye),0.),160.)*.38;vec3 c=mix(base,reflected,fresnel)+vec3(.53,.68,.73)*gleam;gl_FragColor=vec4(c,1.);}`,
    },
  })
  ocean.rotation.x = -Math.PI / 2; ocean.position.y = -25; group.add(ocean)
  // Reflector clones uniforms, including textures; bind the owned normal texture explicitly.
  const oceanMaterial = ocean.material as T.ShaderMaterial
  oceanMaterial.uniforms.normalMap!.value = normals; animated.push(oceanMaterial)

  const fallMaterial = new T.ShaderMaterial({
    uniforms: { time: { value: 0 } }, transparent: true, side: T.DoubleSide, depthWrite: false,
    vertexShader: `uniform float time;varying vec2 vUv;void main(){vUv=uv;vec3 p=position;p.x+=sin(uv.y*12.+time*.4)*.1*pow(sin(uv.y*3.14159),2.);gl_Position=projectionMatrix*modelViewMatrix*vec4(p,1.);}`,
    fragmentShader: `${noise} uniform float time;varying vec2 vUv;void main(){float edge=pow(max(sin(vUv.x*3.14159),0.),.65);float strands=.32+.68*pow(.5+.5*sin(vUv.x*65.+noise3(vec3(vUv.x*9.,vUv.y*8.-time*.65,0.))*4.),2.);float surge=.65+.35*noise3(vec3(vUv.x*19.,vUv.y*37.-time*2.2,2.));float ends=smoothstep(0.,.06,vUv.y)*(1.-smoothstep(.96,1.,vUv.y));float a=edge*strands*surge*ends*.68;gl_FragColor=vec4(mix(vec3(.26,.53,.60),vec3(.85,.96,1.0),strands),a);}`,
  }); animated.push(fallMaterial)
  const centers = [new T.Vector3(-13, 0, 5), new T.Vector3(13, 0, 4)]
  for (const center of centers) {
    for (let i = 0; i < 3; i++) {
      const ribbon = new T.Mesh(new T.PlaneGeometry(1.4 + i * .28, 37, 6, 40), fallMaterial)
      ribbon.position.set(center.x, -6.5, center.z); ribbon.rotation.y = i * Math.PI / 3
      group.add(ribbon)
    }
    const rippleMat = new T.ShaderMaterial({
      uniforms: { time: { value: 0 } }, transparent: true, depthWrite: false, side: T.DoubleSide,
      vertexShader: `varying vec2 vUv;void main(){vUv=uv;gl_Position=projectionMatrix*modelViewMatrix*vec4(position,1.);}`,
      fragmentShader: `uniform float time;varying vec2 vUv;void main(){float r=length(vUv-.5)*2.;float rings=pow(.5+.5*sin(r*66.+time*2.0),9.);float envelope=smoothstep(0.,.13,r)*(1.-smoothstep(.25,1.,r));float mist=exp(-r*r*32.);gl_FragColor=vec4(.52,.80,.84,(rings*envelope*.25+mist*.25));}`,
    }); animated.push(rippleMat)
    const ripple = new T.Mesh(new T.PlaneGeometry(17, 17), rippleMat)
    ripple.rotation.x = -Math.PI / 2; ripple.position.set(center.x, -24.91, center.z); group.add(ripple)
  }
  const drops: number[] = [], seeds: number[] = []
  for (let i = 0; i < 650; i++) {
    const c = centers[i % 2]!, a = rng() * Math.PI * 2, r = .4 + rng() * 1.9
    drops.push(c.x + Math.cos(a) * r, rng() * 37, c.z + Math.sin(a) * r); seeds.push(rng() * 6.28)
  }
  const sprayGeo = new T.BufferGeometry(); sprayGeo.setAttribute('position', new T.Float32BufferAttribute(drops, 3)); sprayGeo.setAttribute('phase', new T.Float32BufferAttribute(seeds, 1))
  const sprayMat = new T.ShaderMaterial({
    uniforms: { time: { value: 0 } }, transparent: true, depthWrite: false,
    vertexShader: `uniform float time;attribute float phase;varying float alpha;void main(){vec3 p=position;float h=mod(p.y+time*(1.0+phase*.12),37.);p.y=h-25.;p.x+=sin(h*.4+phase)*.25;p.z+=cos(h*.3+phase)*.22;vec4 mv=modelViewMatrix*vec4(p,1.);gl_Position=projectionMatrix*mv;gl_PointSize=clamp(46./-mv.z,1.,2.8);alpha=sin(h/37.*3.14159)*.55;}`,
    fragmentShader: `varying float alpha;void main(){float r=length(gl_PointCoord-.5);gl_FragColor=vec4(.75,.94,1.,(1.-smoothstep(.08,.5,r))*alpha);}`,
  }); animated.push(sprayMat)
  const spray = new T.Points(sprayGeo, sprayMat); spray.frustumCulled = false; group.add(spray)
  const motes: number[] = [], moteSeeds: number[] = []
  for (let i = 0; i < 85; i++) { motes.push((rng() - .5) * 85, rng() * 48 - 8, (rng() - .5) * 80); moteSeeds.push(rng() * 6.28) }
  const moteGeo = new T.BufferGeometry(); moteGeo.setAttribute('position', new T.Float32BufferAttribute(motes, 3)); moteGeo.setAttribute('phase', new T.Float32BufferAttribute(moteSeeds, 1))
  const moteMat = new T.ShaderMaterial({
    uniforms: { time: { value: 0 } }, transparent: true, depthWrite: false, blending: T.AdditiveBlending,
    vertexShader: `uniform float time;attribute float phase;varying float alpha;void main(){vec3 p=position;p.y+=sin(time*.14+phase)*.6;p.x+=sin(time*.08+phase)*.8;vec4 mv=modelViewMatrix*vec4(p,1.);gl_Position=projectionMatrix*mv;gl_PointSize=clamp(100./-mv.z,1.,3.5);alpha=.20+.30*pow(sin(time*.5+phase),2.);}`,
    fragmentShader: `varying float alpha;void main(){float r=length(gl_PointCoord-.5);gl_FragColor=vec4(1.,.71,.36,(1.-smoothstep(.05,.5,r))*alpha);}`,
  }); animated.push(moteMat)
  const points = new T.Points(moteGeo, moteMat); points.frustumCulled = false; group.add(points)

  return {
    update(time, camera) {
      for (const material of animated) material.uniforms.time!.value = time
      halo.quaternion.copy(camera.quaternion)
    },
    dispose() {
      scene.remove(group)
      const geometries = new Set<T.BufferGeometry>(), materials = new Set<T.Material>()
      group.traverse(object => { if (object instanceof T.Mesh || object instanceof T.Points) { geometries.add(object.geometry); const source = object.material; for (const material of Array.isArray(source) ? source : [source]) materials.add(material) } })
      for (const geometry of geometries) geometry.dispose()
      for (const material of materials) material.dispose()
      ocean.getRenderTarget().dispose(); normals.dispose()
    },
  }
}
