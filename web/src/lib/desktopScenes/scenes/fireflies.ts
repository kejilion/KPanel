import type { ShaderScene } from '../types'

// A moonlit pine forest: five rows of tiered pines fade into blue mist, a
// clearing opens onto the moon and its light shafts, and fireflies drift and
// blink between the trunks as an additive point pass.

const CAMERA = /* glsl */ `
const float FOCAL = 1.4;
const float PITCH = 0.1;
vec3 cameraOrigin() { return vec3(uPointer.x * 1.4, 1.6 - uPointer.y * 0.4, 0.0); }
`

export const scene: ShaderScene = {
  fragment: /* glsl */ `
${CAMERA}
const int ROWS = 5;
// Far to near: distance, spacing, height, clearing half-width in view units.
const float DIST[5] = float[5](170.0, 95.0, 52.0, 27.0, 12.0);
const float SPACING[5] = float[5](7.0, 6.5, 6.0, 5.5, 6.5);
const float HEIGHT[5] = float[5](34.0, 30.0, 26.0, 22.0, 20.0);
const float CLEARING[5] = float[5](0.0, 0.06, 0.14, 0.22, 0.3);
const vec3 MOON_DIR = vec3(0.27, 0.29, 0.918);
const vec3 SKY_TOP = pow(vec3(0.02, 0.05, 0.09), vec3(2.2));
const vec3 SKY_LOW = pow(vec3(0.1, 0.2, 0.26), vec3(2.2));
const vec3 MIST = pow(vec3(0.2, 0.31, 0.36), vec3(2.2));
const vec3 MOONLIGHT = pow(vec3(0.72, 0.82, 0.95), vec3(2.2));
const vec3 NEEDLES = pow(vec3(0.02, 0.045, 0.045), vec3(2.2));

// Signed horizontal distance inside the nearest spruce (positive = inside).
float pine(float x, float y, int row, out float side) {
  float s = SPACING[row];
  float cell = floor(x / s);
  float near = float(row) / float(ROWS - 1);
  float best = -1e3;
  side = 0.0;
  for (int k = -1; k <= 1; k++) {
    float c = cell + float(k);
    vec3 h = hash32(vec2(c, float(row) * 7.0));
    float cx = (c + 0.5 + (h.x - 0.5) * 0.7) * s;
    if (h.z < 0.15 || abs(cx) / DIST[row] < CLEARING[row] * (0.7 + 0.6 * h.z)) continue;
    float height = HEIGHT[row] * (0.7 + 0.45 * h.y);
    // Near trees lift their crowns to show the trunk.
    float crownBase = height * mix(0.03, 0.22, near);
    float up = (y - crownBase) / (height - crownBase);
    float inside = -1e3;
    if (up >= 0.0 && up <= 1.0) {
      // Tapering crown with faint branch whorls and ragged tips.
      // Each whorl is widest at its drooping tips and tucks in above them.
      float whorl = 1.0 - fract(up * (10.0 + 4.0 * h.x) + h.y);
      float ragged = noise1(y * 9.0 + c * 17.0 + sign(x - cx) * 5.0) * 0.18;
      float radius = height * 0.12 * pow(1.0 - up, 1.05) * (0.78 + 0.22 * whorl * whorl) * (0.9 + ragged);
      inside = radius - abs(x - cx);
    }
    // Only the nearer rows show trunks; far ones stand in their own mist.
    if (near > 0.4 && y < height * 0.85) inside = max(inside, 0.1 + height * 0.004 - abs(x - cx));
    if (inside > best) {
      best = inside;
      side = sign(x - cx);
    }
  }
  return best;
}

vec3 sky(vec3 rd, float px) {
  vec3 col = mix(SKY_LOW, SKY_TOP, sqrt(smoothstep(0.0, 0.55, rd.y)));
  float m = max(dot(rd, MOON_DIR), 0.0);
  col += MOONLIGHT * (pow(m, 30.0) * 0.12 + pow(m, 400.0) * 0.35);
  float disc = smoothstep(0.0155 + px, 0.0155 - px, acos(min(m, 1.0)));
  col += MOONLIGHT * disc * 2.2;
  col += stars(rd, px) * 0.8;
  return col;
}

// Light shafts radiating from the moon, visible only where there is mist.
float shafts(vec3 rd) {
  vec3 side = normalize(cross(MOON_DIR, vec3(0.0, 1.0, 0.0)));
  vec3 up = cross(side, MOON_DIR);
  vec3 a = rd - MOON_DIR * dot(rd, MOON_DIR);
  float angle = atan(dot(a, up), dot(a, side));
  float beams = pow(noise1(angle * 18.0 + uTime * 0.03), 3.0) + 0.6 * pow(noise1(angle * 41.0 - uTime * 0.02), 4.0);
  return beams * exp(-acos(clamp(dot(rd, MOON_DIR), -1.0, 1.0)) * 2.2);
}

vec3 fogged(vec3 col, vec3 rd, float dist, float height, float rays) {
  float fog = 1.0 - exp(-dist * 0.012);
  float ground = exp(-max(height, 0.0) / 3.5) * (1.0 - exp(-dist * 0.05));
  float wisps = fbm(vec2(dist * 0.02 + uTime * 0.02, height * 0.3 + rd.x * 4.0), 4);
  float amount = clamp(fog * 0.85 + ground * smoothstep(0.3, 0.75, wisps) * 0.8, 0.0, 0.95);
  vec3 mist = MIST * (1.0 + shafts(rd) * rays) + MOONLIGHT * pow(max(dot(rd, MOON_DIR), 0.0), 8.0) * 0.12;
  return mix(col, mist, amount);
}

vec3 render(vec2 fragCoord) {
  vec3 ro = cameraOrigin();
  vec3 rd = cameraRay(fragCoord, FOCAL, PITCH, 0.012);
  float px = pixelAngle(FOCAL);
  float ground = rd.y < 0.0 ? -ro.y / rd.y : 1e9;

  for (int j = 0; j < ROWS; j++) {
    int row = ROWS - 1 - j;
    float lambda = DIST[row] / rd.z;
    if (lambda > ground) break;
    float x = ro.x + rd.x * lambda;
    float y = ro.y + rd.y * lambda;
    if (y > HEIGHT[row] * 1.2) continue;
    float side;
    float inside = pine(x, y, row, side);
    if (inside < 0.0) continue;
    float near = float(row) / float(ROWS - 1);
    // Moonlit rims on the side facing the moon.
    float rim = (1.0 - smoothstep(0.0, 0.35 + 0.8 * (1.0 - near), inside)) * step(0.0, side * MOON_DIR.x);
    vec3 col = NEEDLES * (0.6 + 0.4 * near) + MOONLIGHT * rim * mix(0.05, 0.02, near);
    return fogged(col, rd, lambda, y, 0.15);
  }

  if (ground < 1e8) {
    vec3 p = ro + rd * ground;
    float moss = fbm(p.xz * 0.4, 4);
    vec3 col = NEEDLES * mix(0.6, 1.2, moss);
    return fogged(col, rd, ground, 0.0, 0.9);
  }
  vec3 col = sky(rd, px);
  return mix(col, MIST * (1.0 + shafts(rd)), exp(-rd.y * 14.0) * 0.6);
}
`,
  particles: {
    count: 72,
    vertex: /* glsl */ `
${CAMERA}
out float vGlow;
out vec3 vColor;
void main() {
  float i = float(gl_VertexID);
  vec3 h = vec3(hash11(i * 1.31 + 0.5), hash11(i * 7.17 + 3.0), hash11(i * 3.73 + 11.0));
  float t = uTime;
  float depth = mix(4.0, 34.0, h.z * h.z);
  vec3 wander = vec3(
    sin(t * 0.21 * (0.5 + h.x) + h.y * 20.0) * 1.3 + sin(t * 0.47 + h.z * 9.0) * 0.4,
    sin(t * 0.29 * (0.5 + h.y) + h.x * 13.0) * 0.4 + sin(t * 0.8 + h.x * 5.0) * 0.12,
    sin(t * 0.15 + h.x * 7.0) * 1.2
  );
  vec3 p = vec3((h.x - 0.5) * depth * 1.9, 0.3 + h.y * h.y * 3.2, depth) + wander;
  vec3 v = p - cameraOrigin();
  // Undo the camera pitch, then project like the fragment pass.
  v = vec3(v.x, v.y * cos(PITCH) - v.z * sin(PITCH), v.y * sin(PITCH) + v.z * cos(PITCH));
  if (v.z < 0.5) {
    gl_Position = vec4(2.0, 2.0, 0.0, 1.0);
    gl_PointSize = 0.0;
    return;
  }
  vec2 uv = v.xy / v.z * FOCAL;
  float aspect = uResolution.x / uResolution.y;
  gl_Position = vec4(uv.x / (0.5 * aspect), uv.y / 0.5, 0.0, 1.0);
  // Each firefly flashes in its own rhythm with a faint glow in between.
  float flash = pow(max(sin(t * (0.55 + h.y * 0.9) + h.x * 40.0), 0.0), 4.0);
  vGlow = (0.1 + 0.9 * flash) * mix(1.0, 0.45, h.z);
  vColor = mix(vec3(0.72, 1.0, 0.32), vec3(1.0, 0.86, 0.36), h.y);
  gl_PointSize = clamp(uResolution.y * 0.42 / v.z, 4.0, 72.0);
}
`,
    fragment: /* glsl */ `
in float vGlow;
in vec3 vColor;
void main() {
  vec2 d = gl_PointCoord - 0.5;
  float r2 = dot(d, d) * 4.0;
  float core = exp(-r2 * 26.0) * 1.2;
  float halo = exp(-r2 * 5.0) * 0.45;
  float fade = 1.0 - smoothstep(0.75, 1.0, r2);
  fragColor = vec4(vColor * (core + halo) * fade * vGlow, 1.0);
}
`,
  },
}
