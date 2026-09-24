import * as T from 'three'
import { random } from './math'

const noise = `
float hash(vec3 p){p=fract(p*0.3183099+vec3(.13,.37,.73));p*=17.;return fract(p.x*p.y*p.z*(p.x+p.y+p.z));}
float noise3(vec3 p){vec3 i=floor(p),f=fract(p);f=f*f*(3.-2.*f);
return mix(mix(mix(hash(i),hash(i+vec3(1,0,0)),f.x),mix(hash(i+vec3(0,1,0)),hash(i+vec3(1,1,0)),f.x),f.y),
mix(mix(hash(i+vec3(0,0,1)),hash(i+vec3(1,0,1)),f.x),mix(hash(i+vec3(0,1,1)),hash(i+vec3(1,1,1)),f.x),f.y),f.z);}
float fbm(vec3 p){return .57*noise3(p)+.28*noise3(p*2.07)+.15*noise3(p*4.19);}
`

export function atmosphere(scene: T.Scene): { update: (t: number) => void } {
  const sky = new T.Mesh(new T.SphereGeometry(950, 48, 24), new T.ShaderMaterial({
    side: T.BackSide, depthWrite: false, fog: false,
    vertexShader: `varying vec3 vDir;void main(){vDir=position;gl_Position=projectionMatrix*modelViewMatrix*vec4(position,1.);}`,
    fragmentShader: `${noise}
varying vec3 vDir;
void main(){vec3 d=normalize(vDir);float h=max(d.y,0.);
vec3 col=mix(vec3(.095,.055,.15),vec3(.009,.016,.055),smoothstep(-.03,.65,h));
col=mix(col,vec3(.022,.025,.065),smoothstep(.35,.95,h)*.5);
float haze=pow(max(0.,1.-abs(d.y-.04)),15.);col+=vec3(.065,.018,.028)*haze;
float w=fbm(d*4.+vec3(2.,0.,0.)); col+=vec3(.025,.005,.05)*pow(w,3.);
gl_FragColor=vec4(col,1.);}`,
  })); scene.add(sky)

  const moon = new T.Mesh(new T.SphereGeometry(22, 64, 40), new T.ShaderMaterial({
    vertexShader: `varying vec3 vNormal;varying vec3 vP;void main(){vNormal=normalize(normalMatrix*normal);vP=position;gl_Position=projectionMatrix*modelViewMatrix*vec4(position,1.);}`,
    fragmentShader: `${noise}
varying vec3 vNormal;varying vec3 vP;
void main(){float n=fbm(vP*.19);float detail=noise3(vP*.75);float facing=pow(max(0.,vNormal.z),.4);
vec3 c=mix(vec3(.64,.69,.83),vec3(1.36,1.24,1.12),smoothstep(.2,.85,n));
c*=.78+.22*detail;c*=.6+.4*facing;gl_FragColor=vec4(c,1.);}`,
  })); moon.position.set(-26, 57, -145); scene.add(moon)
  // Soft lunar halo remains behind the architectural astrolabe.
  const halo = new T.Mesh(new T.PlaneGeometry(110, 110), new T.ShaderMaterial({
    transparent: true, depthWrite: false, blending: T.AdditiveBlending,
    vertexShader: `varying vec2 uv0;void main(){uv0=uv;gl_Position=projectionMatrix*modelViewMatrix*vec4(position,1.);}`,
    fragmentShader: `varying vec2 uv0;void main(){float r=length(uv0-.5)*2.;float a=exp(-r*r*5.)*.18;gl_FragColor=vec4(.65,.5,.8,a);}`,
  })); halo.position.copy(moon.position); halo.position.z -= 24; scene.add(halo)

  const rand = random(72), positions: number[] = [], colors: number[] = []
  for (let i = 0; i < 1700; i++) {
    const a = rand() * Math.PI * 2, y = rand() * 0.96 + 0.04, r = Math.sqrt(1 - y * y) * 780
    positions.push(Math.cos(a) * r, y * 780, Math.sin(a) * r)
    const b = .4 + rand() * .65; colors.push(b, b * .84, b * .91)
  }
  const stars = new T.BufferGeometry(); stars.setAttribute('position', new T.Float32BufferAttribute(positions, 3)); stars.setAttribute('color', new T.Float32BufferAttribute(colors, 3))
  scene.add(new T.Points(stars, new T.PointsMaterial({ size: 1.05, transparent: true, opacity: .7, vertexColors: true, depthWrite: false, fog: false })))

  const cloudMaterial = new T.ShaderMaterial({
    uniforms: { time: { value: 0 } }, transparent: true, depthWrite: false, side: T.DoubleSide,
    vertexShader: `varying vec3 vWorld;void main(){vWorld=(modelMatrix*vec4(position,1.)).xyz;gl_Position=projectionMatrix*viewMatrix*vec4(vWorld,1.);}`,
    fragmentShader: `${noise}
uniform float time;varying vec3 vWorld;
float density(vec3 p){vec3 q=vec3(p.x*.032+time*.006,0.,p.z*.032+time*.003);
float top=-30.+fbm(q)*24.+noise3(q*.45)*5.;float body=1.-smoothstep(top-4.,top+2.,p.y);
return body*(.5+.4*noise3(vec3(q.x*2.,p.y*.14,q.z*2.)))*smoothstep(-42.,-35.,p.y);}
void main(){vec3 ray=normalize(vWorld-cameraPosition);float stepSize=2.7;vec3 p=vWorld;
vec3 color=vec3(0.);float alpha=0.;
for(int i=0;i<38;i++){float den=density(p);float lit=clamp(den-density(p+vec3(-2.,5.,-2.)),0.,1.);
vec3 c=mix(vec3(.045,.044,.092),vec3(.21,.18,.30),lit*.9+smoothstep(-32.,-12.,p.y)*.4);
float a=1.-exp(-den*stepSize*.34);color+=(1.-alpha)*c*a;alpha+=(1.-alpha)*a;p+=ray*stepSize;if(alpha>.985||p.y< -41.)break;}
float dist=length(vWorld.xz-cameraPosition.xz);vec3 horizon=vec3(.1302,.0647,.1652);color=mix(color,horizon*alpha,smoothstep(80.,360.,dist));
gl_FragColor=vec4(color,alpha);}`,
  })
  const clouds = new T.Mesh(new T.PlaneGeometry(1600, 1600), cloudMaterial)
  clouds.rotation.x = -Math.PI / 2; clouds.position.y = -31; clouds.renderOrder = 1; scene.add(clouds)
  const lowerSea = new T.Mesh(new T.PlaneGeometry(1800, 1800), new T.MeshBasicMaterial({ color: '#292544', fog: true }))
  lowerSea.rotation.x = -Math.PI / 2; lowerSea.position.y = -42; scene.add(lowerSea)
  // Soft cloud lobes fade at their silhouettes; opaque spheres would read as
  // boulders. The ray-marched layer below closes the gaps between these wisps.
  const puffMaterial = new T.ShaderMaterial({ transparent: true, depthWrite: false,
    vertexShader: `varying vec3 vN;varying vec3 vW;varying vec3 vP;void main(){vP=position;vec4 world=modelMatrix*instanceMatrix*vec4(position,1.);vW=world.xyz;vN=normalize(mat3(modelMatrix)*mat3(instanceMatrix)*normal);gl_Position=projectionMatrix*viewMatrix*world;}`,
    fragmentShader: `${noise} varying vec3 vN;varying vec3 vW;varying vec3 vP;
void main(){vec3 N=normalize(vN);float facing=max(0.,dot(N,normalize(cameraPosition-vW)));float n=fbm(vP*4.+vW*.03);
float alpha=pow(facing,1.15)*smoothstep(-.8,.15,vP.y)*(.38+n*.28);
float light=clamp(dot(N,normalize(vec3(-.4,1.,.3))),0.,1.);vec3 col=mix(vec3(.10,.09,.18),vec3(.34,.29,.42),light*.65+n*.2);
col=mix(col,vec3(.1302,.0647,.1652),smoothstep(80.,260.,length(vW-cameraPosition)));gl_FragColor=vec4(col,alpha);}`,
  })
  const puffs = new T.InstancedMesh(new T.SphereGeometry(1, 20, 12), puffMaterial, 361)
  const dummy = new T.Object3D()
  let instance = 0
  for (let x = -9; x <= 9; x++) for (let z = -9; z <= 9; z++) {
    const r = 11 + rand() * 9
    dummy.position.set(x * 21 + (rand() - .5) * 15, -28 + rand() * 5, z * 21 + (rand() - .5) * 15)
    dummy.scale.set(r * (1 + rand() * .4), r * (.42 + rand() * .18), r)
    dummy.rotation.set(0, rand() * Math.PI, .1 * (rand() - .5)); dummy.updateMatrix()
    puffs.setMatrixAt(instance++, dummy.matrix)
  }
  puffs.instanceMatrix.needsUpdate = true; puffs.renderOrder = 2; scene.add(puffs)
  return { update(t) { cloudMaterial.uniforms.time!.value = t; puffs.position.x = Math.sin(t * .018) * 2; puffs.position.y = Math.sin(t * .06) * .25 } }
}

export function waterfalls(scene: T.Scene): { update: (t: number) => void } {
  const material = new T.ShaderMaterial({
    uniforms: { time: { value: 0 } }, transparent: true, side: T.DoubleSide, depthWrite: false,
    blending: T.AdditiveBlending,
    vertexShader: `varying vec2 uv0;void main(){uv0=uv;vec3 p=position;p.x+=sin(uv.y*13.+position.z*.4)*pow(1.-uv.y,2.)*.45;gl_Position=projectionMatrix*modelViewMatrix*vec4(p,1.);}`,
    fragmentShader: `uniform float time;varying vec2 uv0;
void main(){float edge=pow(sin(uv0.x*3.14159),.6);float streams=.25+.75*pow(.5+.5*sin(uv0.x*71.+sin(uv0.y*24.+time*1.7)*.9),3.);
float flow=.65+.35*sin(uv0.y*62.+time*3.);float fade=smoothstep(0.,.27,uv0.y);float a=edge*streams*flow*fade*.6;
gl_FragColor=vec4(mix(vec3(.35,.6,.9),vec3(.7,.88,1.),streams)*1.25,a);}`,
  })
  for (const [x, y, z, width, length] of [[12.5, 1.6, 5.5, 2.1, 37], [-26, -.2, -10, 1.4, 29], [36.5, 5.6, -21, 2.8, 42]]) {
    const ribbon = new T.Mesh(new T.PlaneGeometry(width!, length!, 10, 40), material)
    ribbon.position.set(x!, y! - length! / 2, z!); ribbon.rotation.y = Math.PI / 4; ribbon.renderOrder = 3; scene.add(ribbon)
    const crossing = ribbon.clone(); crossing.rotation.y -= Math.PI / 2; scene.add(crossing)
  }
  return { update(t) { material.uniforms.time!.value = t } }
}

export function lanterns(scene: T.Scene): { update: (t: number) => void } {
  const count = 64, rand = random(999), dummy = new T.Object3D()
  const bodies = new T.InstancedMesh(new T.CylinderGeometry(.36, .29, .75, 8), new T.MeshStandardMaterial({ color: '#fbc887', emissive: '#ffa848', emissiveIntensity: 2.1, roughness: .8 }), count)
  const tops = new T.InstancedMesh(new T.CylinderGeometry(.39, .39, .065, 8), new T.MeshStandardMaterial({ color: '#79543d', metalness: .5, roughness: .5 }), count * 2)
  const seeds = Array.from({ length: count }, () => ({ x: (rand() - .5) * 130, y: rand() * 70, z: (rand() - .63) * 140, s: .4 + rand() * .85, a: rand() * Math.PI * 2 }))
  bodies.frustumCulled = false; tops.frustumCulled = false; scene.add(bodies, tops)
  return { update(t) {
    seeds.forEach((p, i) => {
      dummy.position.set(p.x + Math.sin(t * .075 + p.a) * 2, ((p.y + t * .32) % 80) - 12, p.z + Math.sin(t * .05 + p.a) * 1.5)
      dummy.rotation.set(Math.sin(t * .22 + p.a) * .1, p.a + t * .025, Math.sin(t * .19 + p.a) * .07)
      dummy.scale.setScalar(p.s); dummy.updateMatrix(); bodies.setMatrixAt(i, dummy.matrix)
      dummy.position.y -= .37 * p.s; dummy.updateMatrix(); tops.setMatrixAt(i * 2, dummy.matrix)
      dummy.position.y += .74 * p.s; dummy.updateMatrix(); tops.setMatrixAt(i * 2 + 1, dummy.matrix)
    })
    bodies.instanceMatrix.needsUpdate = true; tops.instanceMatrix.needsUpdate = true
  } }
}

export function petals(scene: T.Scene): { update: (t: number) => void } {
  const rand = random(567), pos: number[] = [], phases: number[] = []
  for (let i = 0; i < 200; i++) { pos.push((rand() - .5) * 85, rand() * 35 - 4, (rand() - .5) * 85); phases.push(rand() * 6.28) }
  const geo = new T.BufferGeometry(); geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); geo.setAttribute('phase', new T.Float32BufferAttribute(phases, 1))
  const mat = new T.ShaderMaterial({
    uniforms: { time: { value: 0 } }, transparent: true, depthWrite: false,
    vertexShader: `uniform float time;attribute float phase;varying float a;void main(){vec3 p=position;p.x+=sin(time*.14+phase)*4.;p.y=mod(p.y-time*.18+40.,40.)-6.;p.z+=cos(time*.13+phase)*3.;vec4 v=modelViewMatrix*vec4(p,1.);gl_Position=projectionMatrix*v;gl_PointSize=clamp(85./-v.z,1.,4.);a=.3+.45*pow(sin(time*.6+phase),2.);}`,
    fragmentShader: `varying float a;void main(){vec2 p=gl_PointCoord-.5;float d=length(p*vec2(1.,1.5));gl_FragColor=vec4(1.,.61,.78,(1.-smoothstep(.15,.5,d))*a);}`,
  }); const points = new T.Points(geo, mat); points.frustumCulled = false; scene.add(points)
  return { update(t) { mat.uniforms.time!.value = t } }
}
