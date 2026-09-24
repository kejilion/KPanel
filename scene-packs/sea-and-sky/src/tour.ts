import * as THREE from 'three'
import type { Daylight } from './daylight'
import type { Shot } from './director'
import { createSubjects, Framing } from './framing'
import { CITY_AZIMUTH } from './skyline'

/** The middle of the rock group, which every shot is framed around. */
export const CENTRE = new THREE.Vector3(20, 0, 0)

/**
 * The camera tour: four shots, each with one thing to say, and the framings
 * that keep their subjects in view as the day turns. follow() moves the
 * framings on; call it every frame (with snap for the first).
 */
export function createTour(daylight: Daylight): { shots: Shot[], follow(dt: number, snap?: boolean): void } {
  // The subjects, and the framings that follow them (see framing.ts).
  const subjects = createSubjects(daylight)
  const main = new Framing()
  const second = new Framing()
  const followSubjects = (dt: number, snap = false) => {
    subjects.update()
    main.follow(subjects.primary, dt, snap)
    second.follow(subjects.secondary, dt, snap)
    light.fov = main.fov
    sky.fov = softMax(second.fov, 62, 4)
  }
  const degrees = THREE.MathUtils.degToRad
  // Rounded max and min: a hard corner in a limit would be a sudden change of speed on screen.
  const softMax = (a: number, b: number, round: number) => (a + b + Math.sqrt((a - b) ** 2 + round * round)) / 2
  const softMin = (a: number, b: number, round: number) => (a + b - Math.sqrt((a - b) ** 2 + round * round)) / 2
  const up = new THREE.Vector3(0, 1, 0)
  const sideways = (heading: THREE.Vector3) => new THREE.Vector3(-heading.z, 0, heading.x)
  const look = (position: THREE.Vector3, direction: THREE.Vector3, pitch: number, target: THREE.Vector3) =>
    target.copy(direction).multiplyScalar(Math.cos(pitch) * 500).add(position).setY(position.y + Math.sin(pitch) * 500)
  const city = new THREE.Vector3(Math.sin(CITY_AZIMUTH), 0, -Math.cos(CITY_AZIMUTH))

  // Four shots, each with one thing to say. Like a camera on a tripod, a shot is placed once, when
  // the camera sets off for it, on the far side of the stacks from the light at that moment (all
  // four a hundred-odd metres apart); while it holds, only the view turns to follow the sun or
  // moon, with a slight drift. So when the subject changes, the camera turns its head rather than
  // swinging round the stacks, and the next move puts it back on the right side.
  const anchored = () => {
    const anchor = new THREE.Vector3(0, 0, -1)
    return { anchor, set: (heading: THREE.Vector3) => anchor.copy(heading) }
  }
  // 1. The light: low over the water, facing the sun or moon and the path it lays on the sea.
  const lightAt = anchored()
  const light: Shot = {
    id: 'light', fov: 50,
    enter: () => lightAt.set(main.heading),
    track(elapsed, position, target) {
      position.copy(CENTRE).addScaledVector(sideways(lightAt.anchor), 100 + Math.sin(elapsed * 0.05) * 8).addScaledVector(lightAt.anchor, -140)
      position.y = 6 + Math.sin(elapsed * 0.3) * 0.3
      look(position, main.heading, main.pitch, target)
    },
  }
  // 2. The stacks: dark against the light, which starts off clear of them on the right third.
  const rocksAt = anchored()
  const rocks: Shot = {
    id: 'rocks', fov: 55,
    enter: () => rocksAt.set(main.heading),
    track(elapsed, position, target) {
      const direction = rocksAt.anchor.clone().applyAxisAngle(up, degrees(26) + Math.sin(elapsed * 0.035) * 0.06)
      position.copy(CENTRE).addScaledVector(direction, -180)
      position.y = 5 + Math.sin(elapsed * 0.27) * 0.25
      look(position, direction, softMin(softMax(main.elevation * 0.4, degrees(2), degrees(1.5)), degrees(9), degrees(1.5)), target)
    },
  }
  // 3. Sea and sky: higher and further back, wide, the stacks off to the left of the light;
  //    turned to a moon rising opposite the sunset.
  const skyAt = anchored()
  const sky: Shot = {
    id: 'sky', fov: 62,
    enter: () => skyAt.set(second.heading),
    track(elapsed, position, target) {
      position.copy(CENTRE).addScaledVector(sideways(skyAt.anchor), 100 + Math.sin(elapsed * 0.04) * 10).addScaledVector(skyAt.anchor, -230)
      position.y = 40 + Math.sin(elapsed * 0.2) * 1
      look(position, second.heading.clone().applyAxisAngle(up, degrees(-6)), softMax(second.pitch, degrees(5), degrees(2)), target)
    },
  }
  // 4. The far city across the water: the sunrise just left of it, the full moon rising behind it
  //    at sunset, its lights at night. When the stacks stand between the camera and the city, the
  //    camera steps aside so they sit well to the right of it.
  const cityAt = anchored()
  let citySide = 0
  const cityShot: Shot = {
    id: 'city', fov: 44,
    enter() {
      cityAt.set(main.heading)
      const toRocks = CENTRE.clone().addScaledVector(cityAt.anchor, 160).sub(CENTRE)
      const ahead = toRocks.dot(city)
      const across = toRocks.dot(sideways(city))
      citySide = ahead > 0 ? across - ahead * Math.tan(degrees(26)) : 0
    },
    track(elapsed, position, target) {
      position.copy(CENTRE).addScaledVector(cityAt.anchor, -160).addScaledVector(sideways(city), citySide + Math.sin(elapsed * 0.04) * 8)
      position.y = 6 + Math.sin(elapsed * 0.3) * 0.25
      look(position, city.clone().applyAxisAngle(up, degrees(6)), degrees(2.5), target)
    },
  }
  const shots: Shot[] = [light, rocks, sky, cityShot]
  return { shots, follow: followSubjects }
}
