import * as T from 'three'
import { mergeGeometries } from 'three/examples/jsm/utils/BufferGeometryUtils.js'
import { palace } from './architecture'
import { box, line, random } from './math'

const ivory = new T.MeshStandardMaterial({ color: '#c7c9b8', roughness: .65, metalness: .08 })
const gold = new T.MeshStandardMaterial({ color: '#bd9b58', roughness: .36, metalness: .65 })
const glow = new T.MeshStandardMaterial({ color: '#edc487', emissive: '#ffbd68', emissiveIntensity: 1.1, roughness: .4 })
const wood = new T.MeshStandardMaterial({ color: '#253835', roughness: .95 })
const leaves = new T.MeshStandardMaterial({ color: '#46645d', roughness: .9, map: pineTexture(), alphaTest: .42, side: T.DoubleSide })
const rockMat = new T.MeshStandardMaterial({ color: '#607979', roughness: .96 })

function pineTexture(): T.CanvasTexture {
  const canvas=document.createElement('canvas');canvas.width=128;canvas.height=128
  const ctx=canvas.getContext('2d')
  if(!ctx)throw new Error('Unable to create pine texture')
  const rand=random(288)
  for(let i=0;i<950;i++){
    const angle=rand()*Math.PI*2,r=Math.sqrt(rand()),x=64+Math.cos(angle)*r*57,y=64+Math.sin(angle)*r*45
    const shade=135+Math.floor(rand()*110)
    ctx.strokeStyle='rgb('+shade+','+shade+','+shade+')';ctx.lineWidth=.6+rand()*.9
    ctx.beginPath();ctx.moveTo(x,y);ctx.lineTo(x+Math.cos(angle)*(3+rand()*8),y+Math.sin(angle)*(3+rand()*6));ctx.stroke()
  }
  const texture=new T.CanvasTexture(canvas);texture.colorSpace=T.SRGBColorSpace;return texture
}

// World-space grain keeps assembled cliffs seamless; silhouettes are geometry.
function weather(material: T.MeshStandardMaterial, strength: number): void {
  material.onBeforeCompile = shader => {
    shader.vertexShader = 'varying vec3 stonePosition;\n' + shader.vertexShader
    shader.vertexShader = shader.vertexShader.replace('#include <worldpos_vertex>', '#include <worldpos_vertex>\nstonePosition=(modelMatrix*vec4(transformed,1.)).xyz;')
    shader.fragmentShader = [
      'varying vec3 stonePosition;',
      'float grain(vec3 p){return fract(sin(dot(p,vec3(127.1,311.7,74.7)))*43758.5453);}',
      'float stoneNoise(vec3 p){vec3 i=floor(p),f=fract(p);f=f*f*(3.-2.*f);',
      'return mix(mix(mix(grain(i),grain(i+vec3(1,0,0)),f.x),mix(grain(i+vec3(0,1,0)),grain(i+vec3(1,1,0)),f.x),f.y),',
      'mix(mix(grain(i+vec3(0,0,1)),grain(i+vec3(1,0,1)),f.x),mix(grain(i+vec3(0,1,1)),grain(i+vec3(1,1,1)),f.x),f.y),f.z);}',
    ].join('\n') + '\n' + shader.fragmentShader
    shader.fragmentShader = shader.fragmentShader.replace('#include <color_fragment>', [
      '#include <color_fragment>',
      'float strata=stoneNoise(stonePosition*vec3(.7,2.7,.7));',
      'float veins=stoneNoise(stonePosition*.27+vec3(strata*.3));',
      'float grain0=stoneNoise(stonePosition*8.);',
      'float tone=.70+.24*veins+.16*grain0;',
      'diffuseColor.rgb*=mix(1.,tone,'+strength.toFixed(2)+');',
    ].join('\n'))
  }
  material.customProgramCacheKey = () => 'moon-palace-weather-' + strength
}
weather(ivory, .8); weather(rockMat, 1.8)

function shard(parent: T.Object3D, x: number, y: number, z: number, w: number, h: number, seed: number): T.Mesh {
  const geometry = new T.IcosahedronGeometry(1, 2)
  const p = geometry.getAttribute('position')
  for (let i=0;i<p.count;i++) {
    const a=p.getX(i), b=p.getY(i), c=p.getZ(i)
    const n=1+.17*Math.sin(a*9+b*5+seed)*Math.sin(c*8-b*4)+.1*Math.sin(b*21+a*4)
    p.setXYZ(i,a*n*w,b*h*(1+.09*Math.sin(a*11+c*5)),c*n*w*.8)
  }
  geometry.computeVertexNormals()
  const mesh = new T.Mesh(geometry,rockMat); mesh.position.set(x,y,z); mesh.rotation.y=seed
  parent.add(mesh); return mesh
}

function cliff(parent: T.Object3D, x: number, y: number, z: number, radius: number, depth: number, seed: number): void {
  const rand=random(seed), vertices:number[]=[],indices:number[]=[]
  const sides=96, levels=15
  // A lobed, vertically stratified mass with a fractured shoulder.
  for(let j=0;j<=levels;j++) for(let i=0;i<=sides;i++){
    const a=i/sides*Math.PI*2,t=j/levels
    const outline=1+.10*Math.sin(a*5+seed)+.055*Math.sin(a*11+.2)+.035*Math.sin(a*19)
    const profile=Math.pow(Math.max(.001,1-t),.46)
    const ribs=1+.08*Math.sin(a*17+t*3)+.045*Math.sin(a*29-t*9)
    const r=radius*outline*profile*ribs
    const yy=-depth*t+(j===0?0:Math.sin(a*8+t*15)*.45)
    vertices.push(x+Math.cos(a)*r,y+yy,z+Math.sin(a)*r*.79)
  }
  for(let j=0;j<levels;j++)for(let i=0;i<sides;i++){
    const a=j*(sides+1)+i,b=a+sides+1; indices.push(a,a+1,b,b,a+1,b+1)
  }
  const center=vertices.length/3; vertices.push(x,y,z)
  for(let i=0;i<sides;i++) indices.push(center,i+1,i)
  const geo=new T.BufferGeometry();geo.setAttribute('position',new T.Float32BufferAttribute(vertices,3));geo.setIndex(indices)
  geo.computeVertexNormals();geo.setAttribute('uv',new T.Float32BufferAttribute(new Float32Array(vertices.length/3*2),2))
  parent.add(new T.Mesh(geo,rockMat))
  for(let i=0;i<32;i++){
    const a=i/32*Math.PI*2,r=radius*(.73+rand()*.19),h=2.5+rand()*depth*.3
    const rock=shard(parent,x+Math.cos(a)*r,y-h*.78-1,z+Math.sin(a)*r*.79,1.4+rand()*2.1,h,seed+i)
    rock.rotation.z=(rand()-.5)*.25
  }
}

function pine(parent:T.Object3D,x:number,y:number,z:number,size:number,seed:number):void {
  const rand=random(seed),g=new T.Group();g.position.set(x,y,z);g.scale.setScalar(size);parent.add(g)
  const trunk=[new T.Vector3(),new T.Vector3(.5,2,0),new T.Vector3(-.3,4.5,.2),new T.Vector3(1,7,0)]
  line(trunk,.22,wood,g)
  for(let b=0;b<9;b++){
    const a=b*2.4,yy=3+b*.42,r=2.2+rand()*1.5
    const end=new T.Vector3(Math.cos(a)*r,yy+1,Math.sin(a)*r)
    line([new T.Vector3(0,yy-1,0),new T.Vector3(end.x*.65,yy+.6,end.z*.65),end],.07+rand()*.05,wood,g)
    for(let k=0;k<12;k++){
      const needle=new T.Mesh(new T.PlaneGeometry(2.4+rand()*.7,2.1+rand()*.7),leaves)
      needle.position.copy(end).add(new T.Vector3((rand()-.5)*2.8,rand()*.5,(rand()-.5)*2.6))
      needle.rotation.set(-Math.PI*.25-rand()*.5,rand()*Math.PI,rand()*Math.PI);g.add(needle)
    }
  }
}

function lantern(parent:T.Object3D,p:T.Vector3,scale=.75):void {
  const g=new T.Group();g.position.copy(p);g.scale.setScalar(scale);parent.add(g)
  box(g,[.85,.3,.85],[0,.15,0],ivory);box(g,[.35,.85,.35],[0,.65,0],ivory)
  box(g,[.86,.18,.86],[0,1.13,0],gold);box(g,[.57,.8,.57],[0,1.62,0],glow)
  for(const x of [-.36,.36])for(const z of [-.36,.36])box(g,[.11,.96,.11],[x,1.63,z],gold)
  const cap=new T.Mesh(new T.ConeGeometry(.85,.6,4),gold);cap.position.y=2.32;cap.rotation.y=Math.PI/4;g.add(cap)
  const bead=new T.Mesh(new T.SphereGeometry(.13,8,6),glow);bead.position.y=2.77;g.add(bead)
}

function bridge(parent:T.Object3D):void {
  const path=new T.CatmullRomCurve3([new T.Vector3(-22,-2,83),new T.Vector3(-20,3.5,62),new T.Vector3(2,8,37),new T.Vector3(0,12.3,11.4)])
  const n=102
  for(let i=0;i<=n;i++){
    const t=i/n,p=path.getPoint(t),tangent=path.getTangent(t),yaw=Math.atan2(tangent.x,tangent.z)
    // Stepped groups separated by small gravity-defying breaks.
    const group=new T.Group();group.position.copy(p);group.rotation.y=yaw;parent.add(group)
    const isGap=i>12&&i<n&&i%17===0
    if(isGap) continue
    box(group,[5.7,.4,.91],[0,0,0],ivory)
    if(i%17===1&&i>10)box(group,[5.95,.28,1.2],[0,-.35,0],gold)
    for(const sign of [-1,1]){
      box(group,[.075,.055,.88],[sign*2.77,.24,0],glow)
      if(i%8===0)lantern(group,new T.Vector3(sign*2.64,.24,0),.48)
    }
    if(i%17===9){
      box(group,[3.8,.5,2],[0,-.6,0],ivory)
      const support=new T.Mesh(new T.CylinderGeometry(.9,.25,3,6),ivory)
      support.position.y=-2.2;group.add(support)
    }
  }
}

function ringArc(parent:T.Object3D,start:number,end:number):void{
  const shape=new T.Shape(),r=34,inner=30.6
  shape.absarc(0,0,r,start,end,false)
  shape.lineTo(Math.cos(end)*inner,Math.sin(end)*inner)
  shape.absarc(0,0,inner,end,start,true);shape.closePath()
  const geo=new T.ExtrudeGeometry(shape,{depth:2.7,bevelEnabled:true,bevelSize:.12,bevelThickness:.12,bevelSegments:2,steps:1,curveSegments:90})
  parent.add(new T.Mesh(geo,ivory))
  for(const radius of [30.7,31.05,33.55]){
    const points=Array.from({length:90},(_,i)=>{const a=start+(end-start)*i/89;return new T.Vector3(Math.cos(a)*radius,Math.sin(a)*radius,2.86)})
    line(points,radius===30.7?.06:.035,radius===30.7?glow:gold,parent)
  }
  // Shallow scrollwork stays lower contrast than the rim light.
  for(let a=start+.09;a<end-.09;a+=.15){
    const pts:T.Vector3[]=[]
    for(let k=0;k<=24;k++){
      const t=k/24*Math.PI*2,r0=32.25+Math.sin(t)*.56,ang=a+Math.cos(t)*.025
      pts.push(new T.Vector3(Math.cos(ang)*r0,Math.sin(ang)*r0,2.82))
    }
    line(pts,.027,gold,parent)
  }
}

function batchStatic(root:T.Group):void {
  root.updateMatrixWorld(true)
  const batches=new Map<T.Material,T.BufferGeometry[]>(),original:T.Mesh[]=[]
  root.traverse(object=>{
    if(!(object instanceof T.Mesh)||Array.isArray(object.material))return
    const geo=object.geometry.index?object.geometry.toNonIndexed():object.geometry.clone()
    geo.applyMatrix4(object.matrixWorld)
    if(!geo.getAttribute('uv'))geo.setAttribute('uv',new T.Float32BufferAttribute(new Float32Array(geo.getAttribute('position').count*2),2))
    const batch=batches.get(object.material)||[];batch.push(geo);batches.set(object.material,batch);original.push(object)
  })
  for(const [material,geometries]of batches){
    const geo=mergeGeometries(geometries)
    if(!geo)throw new Error('Failed to merge moon-palace geometry')
    const mesh=new T.Mesh(geo,material);mesh.castShadow=true;mesh.receiveShadow=true;root.add(mesh)
    geometries.forEach(g=>g.dispose())
  }
  original.forEach(mesh=>{mesh.removeFromParent();mesh.geometry.dispose()})
}

export function createWorld(scene:T.Scene):{update:(t:number)=>void}{
  const root=new T.Group();scene.add(root)
  cliff(root,0,12,0,16,23,811)
  palace(root,0,12.7,-1.5,.84)
  palace(root,-9.1,12.1,-2.5,.34);palace(root,9.1,12.1,-2.5,.34)
  const inverted=palace(root,0,-8,2,.42);inverted.rotation.z=Math.PI
  for(let i=0;i<6;i++)box(root,[6.5+i*.3,.26,1.15],[0,13.3-i*.21,7+i*.85],ivory)
  const ring=new T.Group();ring.position.set(0,32,-12);root.add(ring)
  for(const [a,b]of [[.03,.79],[1.32,3.79],[3.86,5.13],[5.2,6.22]])ringArc(ring,a!,b!)
  pine(root,-12,12.5,1.5,.95,93);pine(root,11.5,12.2,3,.76,14);pine(root,-8,12.1,-7,.55,28)
  bridge(root)
  cliff(root,-22,-2.5,84,9,13,819);pine(root,-28,-2.2,82,1.3,528)
  const rand=random(7821)
  for(let i=0;i<23;i++){
    const side=i%2===0?-1:1,x=side*(48+rand()*160),z=-45-rand()*245,y=-14+rand()*42,r=2+rand()*4.7
    cliff(root,x,y,z,r,15+rand()*32,80+i)
    if(i%3===0)pine(root,x,y,z,.4+rand()*.25,10+i)
  }
  batchStatic(root)
  const moving=new T.Group();scene.add(moving)
  const fragments:{mesh:T.Mesh;base:T.Vector3;phase:number}[]=[]
  for(let i=0;i<16;i++){
    const a=.85+rand()*.39,r=32+rand()*10
    const mesh=shard(moving,Math.cos(a)*r,32+Math.sin(a)*r,-9+rand()*8,.3+rand()*.9,1+rand()*2,128+i)
    mesh.material=ivory;mesh.castShadow=true
    fragments.push({mesh,base:mesh.position.clone(),phase:rand()*6.28})
  }
  for(let i=0;i<12;i++){
    const sign=i%2?-1:1,mesh=shard(moving,sign*(21+rand()*28),12+rand()*37,-8-rand()*36,.4+rand()*.8,1+rand()*2.5,223+i)
    fragments.push({mesh,base:mesh.position.clone(),phase:rand()*6.28})
  }
  return {update(t){for(const f of fragments){f.mesh.position.y=f.base.y+Math.sin(t*.15+f.phase)*.42;f.mesh.rotation.z=Math.sin(t*.06+f.phase)*.09}}}
}
