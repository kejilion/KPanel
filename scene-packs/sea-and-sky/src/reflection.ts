import * as THREE from 'three'

/**
 * The stacks' reflection in the sea. Every frame the rocks (layer 1) are drawn
 * from the camera's mirror image below the water, looking up; seen from there,
 * each rock is exactly where its reflection lies. The sea reads this image at
 * its own position (through uReflectionMatrix), shifted by the waves.
 */
export interface Reflection {
  texture: THREE.Texture
  matrix: THREE.Matrix4
  render(camera: THREE.PerspectiveCamera): void
  resize(width: number, height: number): void
}

export function createReflection(renderer: THREE.WebGLRenderer, scene: THREE.Scene, mirrorPass: THREE.IUniform<number>): Reflection {
  const target = new THREE.WebGLRenderTarget(1, 1, { type: THREE.HalfFloatType })
  const mirror = new THREE.PerspectiveCamera()
  const direction = new THREE.Vector3()
  const look = new THREE.Vector3()
  const matrix = new THREE.Matrix4()
  const bias = new THREE.Matrix4().set(
    0.5, 0, 0, 0.5,
    0, 0.5, 0, 0.5,
    0, 0, 0.5, 0.5,
    0, 0, 0, 1,
  )
  const clear = new THREE.Color()
  return {
    texture: target.texture,
    matrix,
    render(camera) {
      mirror.copy(camera)
      mirror.layers.set(1)
      camera.getWorldDirection(direction)
      mirror.position.set(camera.position.x, -camera.position.y, camera.position.z)
      look.copy(mirror.position).add(direction.set(direction.x, -direction.y, direction.z))
      mirror.up.set(0, 1, 0)
      mirror.lookAt(look)
      mirror.updateMatrixWorld()
      matrix.copy(bias).multiply(mirror.projectionMatrix).multiply(mirror.matrixWorldInverse)
      const previousTarget = renderer.getRenderTarget()
      renderer.getClearColor(clear)
      const previousAlpha = renderer.getClearAlpha()
      renderer.setRenderTarget(target)
      renderer.setClearColor(0x000000, 0)
      renderer.clear()
      mirrorPass.value = 1
      renderer.render(scene, mirror)
      mirrorPass.value = 0
      renderer.setClearColor(clear, previousAlpha)
      renderer.setRenderTarget(previousTarget)
    },
    resize(width, height) {
      target.setSize(Math.max(1, Math.round(width)), Math.max(1, Math.round(height)))
    },
  }
}
