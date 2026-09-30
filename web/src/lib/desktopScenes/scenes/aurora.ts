import type { ShaderScene } from '../types'

// A frozen shore under the northern lights: two meandering curtains with a
// pink lower fringe and drifting rays, integrated through stacked altitude
// slices; snowy peaks across a still lake that mirrors them all.

export const scene: ShaderScene = {
  fragment: /* glsl */ `
const float FOCAL = 1.3;
const float CAMERA_HEIGHT = 1.8;
const vec3 NIGHT_TOP = pow(vec3(0.008, 0.02, 0.055), vec3(2.2));
const vec3 NIGHT_HORIZON = pow(vec3(0.04, 0.1, 0.16), vec3(2.2));
const vec3 AURORA_GREEN = vec3(0.1, 1.0, 0.45);
const vec3 AURORA_TEAL = vec3(0.05, 0.75, 0.7);
const vec3 AURORA_VIOLET = vec3(0.5, 0.18, 0.85);
const vec3 AURORA_PINK = vec3(0.9, 0.25, 0.5);
const vec3 SNOW = pow(vec3(0.62, 0.7, 0.82), vec3(2.2));

// Brightness of one aurora sheet where a horizontal slice at relative height s
// crosses the point p on the ground plane. Each sheet is an east-west arc
// ahead of the viewer that meanders, leans back with height and pulses.
float sheet(vec2 p, float s, float k) {
  float t = uTime * 0.03;
  float x = p.x;
  float arc = mix(430.0, 760.0, k)
    + 95.0 * sin(x * 0.0041 + t + k * 2.1)
    + 42.0 * sin(x * 0.011 - t * 1.3 + k * 5.0)
    + 16.0 * sin(x * 0.03 + t * 2.1 + k);
  arc += s * 70.0;
  float d = abs(p.y - arc);
  float thin = exp(-d / (14.0 + s * 34.0));
  float rays = 0.3 + 0.7 * noise1(x * 0.085 + t * 3.0 + k * 17.0) * (0.5 + 0.5 * noise1(x * 0.021 - t + k * 3.0));
  float pulse = 0.08 + 0.92 * smoothstep(0.35, 0.8, noise1(x * 0.0045 + t * 0.6 + k * 9.0));
  return thin * rays * pulse;
}

// March the part of the ray inside the curtain zone; per-pixel jitter hides the steps.
vec3 aurora(vec3 ro, vec3 rd, float jitter) {
  if (rd.y < 0.012) return vec3(0.0);
  float start = max((50.0 - ro.y) / rd.y, 300.0 / rd.z);
  float end = min((215.0 - ro.y) / rd.y, 1100.0 / rd.z);
  if (end <= start) return vec3(0.0);
  int steps = int(mix(16.0, 34.0, uDetail));
  float stride = (end - start) / float(steps);
  vec3 sum = vec3(0.0);
  for (int i = 0; i < 34; i++) {
    if (i >= steps) break;
    vec3 p = ro + rd * (start + (float(i) + jitter) * stride);
    float s = (p.y - 55.0) / 150.0;
    float field = (sheet(p.xz, s, 0.0) + sheet(p.xz, s, 1.0) * 0.6) * smoothstep(-0.03, 0.02, s);
    vec3 col = mix(AURORA_GREEN, AURORA_VIOLET, smoothstep(0.3, 0.95, s));
    col = mix(col, AURORA_TEAL, smoothstep(0.1, 0.3, s) * (1.0 - smoothstep(0.3, 0.6, s)) * 0.4);
    col += AURORA_PINK * exp(-max(s, 0.0) * 16.0) * 0.6;
    sum += col * field * exp(-max(s, 0.0) * 3.0);
  }
  // A faint green airglow under the curtains.
  vec3 glow = AURORA_GREEN * exp(-rd.y * 5.0) * 0.012;
  return sum * stride / 38.0 + glow;
}

vec3 milkyWay(vec3 rd) {
  vec3 axis = normalize(vec3(0.55, 0.62, -0.56));
  float band = exp(-pow(dot(rd, axis), 2.0) * 18.0);
  if (band < 0.01) return vec3(0.0);
  vec2 p = vec2(atan(rd.x, rd.z), rd.y) * 5.0;
  float dust = fbm(p * 1.6 + 3.0, octaves(5.0));
  float lanes = smoothstep(0.45, 0.7, fbm(p * 3.1 + 9.0, octaves(4.0)));
  return vec3(0.1, 0.11, 0.16) * band * dust * (1.0 - lanes * 0.7) * 0.35;
}

vec3 meteor(vec3 rd, float px) {
  float period = 11.0;
  float slot = floor(uTime / period);
  if (hash11(slot * 1.7) > 0.5) return vec3(0.0);
  float t = uTime - slot * period;
  if (t > 0.8) return vec3(0.0);
  vec2 start = vec2(mix(-0.5, 0.5, hash11(slot * 3.1)), mix(0.22, 0.36, hash11(slot * 5.3)));
  vec2 dir = normalize(vec2(mix(-1.0, 1.0, hash11(slot * 7.7)), -0.5));
  vec2 head = start + dir * t * 0.45;
  vec2 tail = head - dir * 0.1 * smoothstep(0.0, 0.2, t);
  vec2 p = vec2(atan(rd.x, rd.z), rd.y);
  vec2 pa = p - tail;
  vec2 ba = head - tail;
  float along = clamp(dot(pa, ba) / max(dot(ba, ba), 1e-6), 0.0, 1.0);
  float d = length(pa - ba * along);
  float fade = smoothstep(0.0, 0.1, t) * (1.0 - smoothstep(0.5, 0.8, t));
  return vec3(0.8, 0.9, 1.0) * smoothstep(px * 1.8, 0.0, d) * along * along * fade * 1.6;
}

float peaks(vec3 ro, vec3 rd, out float dist) {
  // Two ranges across the lake; returns their height at this bearing.
  float far = 1700.0 / rd.z;
  float xFar = ro.x + rd.x * far;
  float hFar = -10.0 + 190.0 * ridged1(xFar * 0.0016 + 4.0, octaves(7.0));
  float near = 900.0 / rd.z;
  float xNear = ro.x + rd.x * near;
  float hNear = -30.0 + 120.0 * ridged1(xNear * 0.0026 + 11.0, octaves(6.0)) * smoothstep(-900.0, -200.0, xNear);
  float yNear = ro.y + rd.y * near;
  if (yNear < hNear) { dist = near; return hNear; }
  dist = far;
  return hFar;
}

vec3 mountains(vec3 ro, vec3 rd, float top, float dist) {
  float y = ro.y + rd.y * dist;
  float x = ro.x + rd.x * dist;
  float depth = top - y;
  // Snow faces with their own relief, lit by the aurora from above and the sky glow.
  vec2 face = vec2(x, y) * 0.02;
  float f0 = fbm(face, 4);
  vec2 g = vec2(fbm(face + vec2(0.3, 0.0), 4) - f0, fbm(face + vec2(0.0, 0.3), 4) - f0) * 3.0;
  vec3 n = normalize(vec3(-g.x, 0.7 + g.y, -0.7));
  float rock = smoothstep(0.55, 0.75, f0) * smoothstep(10.0, 60.0, depth);
  vec3 albedo = mix(SNOW, SNOW * 0.2, rock);
  float fromAbove = max(n.y, 0.0);
  vec3 col = albedo * (NIGHT_HORIZON * 3.0 + AURORA_GREEN * 0.07 * fromAbove + vec3(0.02, 0.025, 0.04) * max(dot(n, normalize(vec3(-0.5, 0.6, -0.4))), 0.0));
  col *= mix(1.0, 0.6, smoothstep(0.0, 150.0, depth));
  return mix(col, NIGHT_HORIZON * 1.2, 1.0 - exp(-dist * 0.00035));
}

vec3 sky(vec3 ro, vec3 rd, float px, float jitter, bool direct) {
  float y = max(rd.y, 0.0);
  vec3 col = mix(NIGHT_HORIZON, NIGHT_TOP, sqrt(smoothstep(0.0, 0.5, y)));
  col += milkyWay(rd);
  col += stars(rd, px) * (direct ? 0.7 : 0.3);
  col += aurora(ro, rd, jitter);
  if (direct) col += meteor(rd, px);
  return col;
}

vec3 render(vec2 fragCoord) {
  vec3 ro = vec3(uPointer.x * 3.0, CAMERA_HEIGHT - uPointer.y * 0.6, 0.0);
  vec3 rd = cameraRay(fragCoord, FOCAL, 0.06, 0.012);
  float px = pixelAngle(FOCAL);
  float jitter = hash12(fragCoord);

  if (rd.y >= 0.0) {
    float dist;
    float top = peaks(ro, rd, dist);
    if (ro.y + rd.y * dist < top) return mountains(ro, rd, top, dist);
    return sky(ro, rd, px, jitter, true);
  }

  float t = -ro.y / rd.y;
  vec3 p = ro + rd * t;
  // Snowy shore in the foreground, the lake beyond it.
  float shore = 5.0 + 3.0 * fbm1(p.x * 0.06 + 2.0, 4) + 1.5 * fbm1(p.x * 0.3, 2);
  if (p.z < shore) {
    float grain = hash12(floor(p.xz * 40.0));
    float sparkle = step(0.994, grain) * (0.5 + 0.5 * sin(uTime * 2.0 + grain * 60.0)) * (1.0 - smoothstep(3.0, 8.0, t));
    float drift = fbm(p.xz * vec2(0.5, 1.4), 4);
    vec3 snow = SNOW * (NIGHT_HORIZON * 3.2 + AURORA_GREEN * 0.07) * mix(0.7, 1.1, drift);
    float edge = smoothstep(shore - 1.2, shore, p.z);
    return mix(snow + vec3(0.5, 0.7, 0.8) * sparkle * 0.15 * (1.0 - edge), NIGHT_HORIZON * 0.3, edge * 0.6);
  }

  // Still water with a faint ripple; mostly a mirror of the sky.
  float fade = 1.0 - smoothstep(40.0, 400.0, t);
  vec2 q = p.xz;
  vec2 ripple = vec2(
    sin(q.x * 1.3 + uTime * 0.7) + sin(q.x * 2.9 - q.y * 1.7 + uTime * 1.1),
    sin(q.y * 1.1 - uTime * 0.6) + sin(q.x * 1.9 + q.y * 3.1 + uTime * 0.9)
  ) * 0.012 * fade;
  vec3 n = normalize(vec3(ripple.x, 1.0, ripple.y));
  vec3 r = reflect(rd, n);
  r.y = abs(r.y);
  float fresnel = 0.02 + 0.98 * pow(1.0 - max(dot(n, -rd), 0.0), 5.0);
  vec3 reflected;
  float dist;
  float top = peaks(ro, r, dist);
  if (ro.y + r.y * dist < top) reflected = mountains(ro, r, top, dist);
  else reflected = sky(ro, r, px, jitter, false);
  vec3 water = NIGHT_TOP * 0.5;
  return mix(water, reflected, mix(0.35, 1.0, fresnel));
}
`,
}
