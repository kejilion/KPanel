import type { ShaderScene } from '../types'

// A mountain valley seen from a high meadow: five ridges recede into aerial
// haze, snow catches the light on the far peaks, pines crown the near hills and
// mist drifts through the valleys while the sky follows the local clock.

export const scene: ShaderScene = {
  fragment: /* glsl */ `
const float FOCAL = 1.4;
const int LAYERS = 5;
// Far to near: distance, base height, relief, frequency, pine density.
const float DIST[5] = float[5](3400.0, 1900.0, 1050.0, 520.0, 210.0);
const float BASE[5] = float[5](-40.0, -120.0, -110.0, -70.0, -40.0);
const float RELIEF[5] = float[5](640.0, 380.0, 190.0, 95.0, 38.0);
const float FREQ[5] = float[5](0.00075, 0.0013, 0.0024, 0.0048, 0.011);
const float PINES[5] = float[5](0.0, 0.0, 0.0, 0.0, 0.32);
const vec3 SNOW = pow(vec3(0.93, 0.95, 1.0), vec3(2.2));
const vec3 ROCK = pow(vec3(0.42, 0.44, 0.5), vec3(2.2));
const vec3 FOREST = pow(vec3(0.12, 0.22, 0.15), vec3(2.2));
const vec3 MEADOW = pow(vec3(0.3, 0.42, 0.16), vec3(2.2));
const vec3 SPRUCE = pow(vec3(0.06, 0.13, 0.1), vec3(2.2));

// Conifer crowns along a ridge: overlapping narrow triangles.
float pines(float x, float density, float height) {
  if (density <= 0.0) return 0.0;
  float u = x * density;
  float cell = floor(u);
  float tallest = 0.0;
  for (int k = -1; k <= 1; k++) {
    float c = cell + float(k);
    float h = hash11(c * 3.71);
    if (h < 0.3) continue;
    float center = c + 0.5 + (hash11(c * 1.37) - 0.5) * 0.7;
    float along = abs(u - center) / 0.55;
    // Slightly concave flanks read as spruce rather than sawtooth.
    float tree = height * (0.5 + 0.5 * h) * pow(max(0.0, 1.0 - along), 1.35);
    tallest = max(tallest, tree);
  }
  return tallest;
}

float ridgeShape(int i, float x) {
  float f = x * FREQ[i] + float(i) * 13.7;
  int count = octaves(i == 0 ? 7.0 : 6.0);
  return i < 2 ? ridged1(f, count) : fbm1(f, count);
}

// The bare ground, which also drives the lighting so trees never streak it.
float terrainHeight(int i, float x) { return BASE[i] + RELIEF[i] * ridgeShape(i, x); }

float ridgeHeight(int i, float x) {
  float shape = ridgeShape(i, x);
  float h = BASE[i] + RELIEF[i] * shape;
  // Distant forest reads as a rough canopy rather than single trees.
  if (i == 3) h += (noise1(x * 0.35) * 0.6 + noise1(x * 1.1) * 0.4) * 7.0;
  // Forest thins out towards the crests.
  return h + pines(x, PINES[i], 15.0) * smoothstep(0.75, 0.35, shape);
}

vec3 sky(vec3 ro, vec3 rd, float px) {
  vec3 col = skyGradient(rd);
  col += stars(rd, px) * uStars;
  col += sunAndMoon(rd, px);
  vec4 deck = cloudDeck(ro, rd, 2200.0, 0.0011, 0.4, 0.012);
  return mix(col, deck.rgb, deck.a);
}

vec3 render(vec2 fragCoord) {
  vec3 ro = vec3(uPointer.x * 6.0, -uPointer.y * 2.0, 0.0);
  vec3 rd = cameraRay(fragCoord, FOCAL, 0.02, 0.01);
  float px = pixelAngle(FOCAL);
  vec3 sunFlat = normalize(vec3(uSunDir.x, max(uSunDir.y, 0.04), 0.0) + vec3(0.0001));
  vec3 moonFlat = normalize(vec3(uMoonDir.x, max(uMoonDir.y, 0.04), 0.0) + vec3(0.0001));
  vec3 ambient = mix(uZenith, uHorizon, 0.4) * 0.8 + vec3(0.004, 0.006, 0.012);

  // Nothing rises above ~11 degrees, so the upper sky skips the ridge tests.
  if (rd.y / rd.z < 0.2) {
    for (int j = 0; j < LAYERS; j++) {
      int i = LAYERS - 1 - j;
      float lambda = DIST[i] / rd.z;
      float x = ro.x + rd.x * lambda;
      float y = ro.y + rd.y * lambda;
      float top = ridgeHeight(i, x);
      if (y > top) continue;

      float near = float(i) / float(LAYERS - 1);
      float dx = 6.0 + 30.0 * (1.0 - near);
      float ground = terrainHeight(i, x);
      float slope = (terrainHeight(i, x + dx) - ground) / dx;
      // Depth below the bare ground, so trees never cast streaks down the slope.
      float depth = max(ground - y, 0.0);
      // The rock face has its own relief so light varies down the slope too.
      float scale = 0.012 + 0.03 * near;
      vec2 face = vec2(x, y) * scale;
      float f0 = fbm(face, 4);
      vec2 g = vec2(fbm(face + vec2(0.35, 0.0), 4) - f0, fbm(face + vec2(0.0, 0.35), 4) - f0) * 3.0;
      vec3 normal = normalize(vec3(-slope * 0.7 - g.x, 0.75 + g.y, -0.65));
      // Terrain is lit from above and beside the viewer, so faces are readable at any hour.
      vec3 sunLight = normalize(vec3(uSunDir.x * 1.6, max(uSunDir.y, 0.02) + 0.3, -0.25));
      vec3 moonLight = normalize(vec3(uMoonDir.x * 1.6, max(uMoonDir.y, 0.02) + 0.3, -0.25));

      // Albedo: snowy rock far away, forest and meadow up close.
      vec3 albedo = mix(ROCK, FOREST, smoothstep(0.5, 2.0, float(i)));
      if (i == 4) {
        // Spruces on the crest, meadow on the slope below them.
        albedo = y > ground - 1.0 ? SPRUCE : mix(FOREST, MEADOW, smoothstep(4.0, 30.0, depth) * 0.8);
      }
      float snowLine = i == 0 ? 170.0 : 300.0;
      float snow = i < 2 ? smoothstep(snowLine - 50.0, snowLine + 50.0, y + (f0 - 0.5) * 220.0) : 0.0;
      snow *= smoothstep(0.35, 0.75, normal.y);
      albedo = mix(albedo, SNOW, snow);

      float sun = max(dot(normal, sunLight), 0.0);
      float moon = max(dot(normal, moonLight), 0.0);
      // Upper slopes see more sky; hollows stay darker.
      float occlusion = mix(0.5, 1.0, exp(-depth / (40.0 + 200.0 * (1.0 - near)))) * mix(0.7, 1.0, normal.y);
      vec3 col = albedo * (ambient * occlusion + uSunColor * sun * 1.15 + uMoonColor * moon * 0.9);

      // Backlit crests glow when the sun or moon sits behind the ridge.
      float rim = exp(-(top - y) / (3.0 + 10.0 * (1.0 - near)));
      col += (uSunColor * pow(max(dot(rd, uSunDir), 0.0), 5.0) + uMoonColor * pow(max(dot(rd, uMoonDir), 0.0), 6.0) * 0.6) * rim * 0.7;

      // Aerial perspective towards the horizon, warmer on the sun's side.
      vec3 hazeColor = mix(uHorizon, uGlow, pow(max(dot(rd, uSunDir), 0.0), 3.0) * 0.6) * 0.95;
      col = mix(col, hazeColor, 1.0 - exp(-lambda * 0.0003));

      // Valley fog: thick below the fog top, thinning with height, broken into drifting banks.
      float fogTop = BASE[i] + RELIEF[i] * 0.12;
      float heightFog = exp(-max(y - fogTop, 0.0) / (10.0 + 40.0 * (1.0 - near)));
      float banks = fbm(vec2(x * 0.004 + uTime * 0.008, y * 0.025 + float(i) * 3.0), 4);
      float mist = clamp(heightFog * smoothstep(0.35, 0.75, banks) * (1.0 - 0.6 * near), 0.0, 0.7);
      vec3 mistColor = hazeColor * mix(0.85, 1.1, uLight) + uSunColor * 0.08;
      return mix(col, mistColor, mist);
    }
  }
  return sky(ro, rd, px);
}
`,
}
