import type { ShaderScene } from '../types'

// Open sea from a low headland: a perspective swell whose band-limited normals
// reflect the sky by Fresnel, a sun or moon glitter path that emerges from the
// wave slopes, drifting clouds and a far coast on the horizon.

export const scene: ShaderScene = {
  fragment: /* glsl */ `
const float FOCAL = 1.55;
const float CAMERA_HEIGHT = 2.4;
const vec3 DEEP_DAY = pow(vec3(0.03, 0.2, 0.3), vec3(2.2));
const vec3 DEEP_NIGHT = pow(vec3(0.02, 0.04, 0.09), vec3(2.2));
const vec3 SHALLOW = pow(vec3(0.1, 0.62, 0.62), vec3(2.2));

// Sum of travelling waves: x is the height, yz its slope in world x and z, and
// w the slope variance of waves too small to resolve, which widens the glints.
vec4 swell(vec2 p, float footprint) {
  vec4 sum = vec4(0.0);
  float k = 0.12;
  int count = int(mix(14.0, 24.0, uDetail));
  for (int i = 0; i < 24; i++) {
    if (i >= count) break;
    float fi = float(i);
    // The long swell arrives from one bearing; shorter wind waves spread wider.
    float spread = mix(0.5, 2.8, smoothstep(0.0, 9.0, fi));
    float angle = (hash11(fi * 7.31 + 1.0) - 0.5) * spread + 0.2;
    vec2 d = vec2(sin(angle), -cos(angle));
    // Waves shorter than a pixel fade out instead of aliasing into noise.
    float fade = 1.0 - smoothstep(0.25, 1.0, k * footprint);
    float phase = dot(d, p) * k - sqrt(9.8 * k) * uTime * 0.55 + fi * 1.93;
    // A little noise in the phase breaks the long crests into irregular chop.
    if (i < 10) phase += noise(p * k * 0.3 + fi * 3.1) * 1.6;
    // exp(sin) waves: peaked crests over wide, flat troughs.
    float crest = exp(sin(phase) - 1.0);
    // Gentle long swell, steeper short chop.
    float steep = mix(0.022, 0.055, smoothstep(2.0, 12.0, fi));
    float amplitude = steep / k;
    sum.xyz += vec3(crest, d * crest * cos(phase) * k) * amplitude * fade;
    sum.w += (1.0 - fade) * steep * steep * 0.5;
    k *= 1.3;
  }
  return sum;
}

// Elevation of the far coast above the horizon at this view direction.
float coast(vec3 rd) {
  float az = rd.x / max(rd.z, 0.05);
  float left = smoothstep(-1.2, -0.72, az) * (1.0 - smoothstep(-0.34, -0.12, az));
  float right = smoothstep(0.42, 0.5, az) * (1.0 - smoothstep(0.6, 0.72, az));
  float headland = (0.012 + 0.034 * ridged1(az * 5.0 + 3.1, 5)) * left;
  float islet = (0.004 + 0.009 * fbm1(az * 18.0, 3)) * right;
  return max(headland, islet);
}

vec3 coastColor(vec3 rd, float top) {
  vec3 base = mix(uZenith * 0.35, uHorizon * 0.55, 0.55) * (0.55 + 0.45 * uLight);
  // Aerial haze lifts the far coast towards the horizon colour.
  vec3 col = mix(base, uHorizon, 0.38 + 0.2 * smoothstep(0.0, top, rd.y));
  float rim = smoothstep(top - 0.0025, top, rd.y) * pow(max(dot(rd, uSunDir), 0.0), 6.0);
  return col + uSunColor * rim * 0.5;
}

vec3 sky(vec3 ro, vec3 rd, float px, bool direct) {
  vec3 col = skyGradient(rd);
  // Stars and cloud detail are lost in the rippled reflection anyway.
  if (direct) col += stars(rd, px) * uStars;
  col += sunAndMoon(rd, px);
  if (direct) {
    vec4 deck = cloudDeck(ro, rd, 1100.0, 0.0018, 0.44, 0.016);
    col = mix(col, deck.rgb, deck.a);
  }
  return col;
}

vec3 render(vec2 fragCoord) {
  vec3 ro = vec3(uPointer.x * 0.8, CAMERA_HEIGHT - uPointer.y * 0.3, 0.0);
  vec3 rd = cameraRay(fragCoord, FOCAL, -0.03, 0.012);
  float px = pixelAngle(FOCAL);
  if (rd.y >= 0.0) {
    float top = coast(rd);
    if (rd.y < top) return coastColor(rd, top);
    return sky(ro, rd, px, true);
  }

  float t = min(-ro.y / rd.y, 40000.0);
  vec3 p = ro + rd * t;
  float footprint = t * px / max(-rd.y, 0.01);
  vec4 w = swell(p.xz, footprint);
  vec3 n = normalize(vec3(-w.y, 1.0, -w.z));
  float fresnel = 0.02 + 0.98 * pow(1.0 - max(dot(n, -rd), 0.0), 5.0);
  vec3 r = reflect(rd, n);
  r.y = abs(r.y) + 0.001;
  vec3 reflection = sky(ro, r, px, false);
  // Only the far water is calm enough to mirror the coast.
  float reflectedCoast = coast(r) * smoothstep(300.0, 1500.0, t);
  if (r.y < reflectedCoast) reflection = coastColor(r, reflectedCoast) * 0.8;

  // Glints: sharp where the waves are resolved, a broad path where they are not.
  float roughness = 0.0012 + w.w * 2.0;
  float shininess = 1.0 / roughness;
  float norm = (shininess + 2.0) / TAU * 0.035;
  vec3 glitter = uSunColor * pow(max(dot(r, uSunDir), 0.0), shininess) * norm
    + uMoonColor * pow(max(dot(r, uMoonDir), 0.0), shininess) * norm * 1.6;

  float light = 0.2 + 0.8 * uLight;
  vec3 body = mix(DEEP_NIGHT, DEEP_DAY, uLight) * light;
  // Light passing through the crests turns them turquoise.
  float crest = max(w.x, 0.0);
  body += SHALLOW * crest * 0.12 * light * (0.4 + 0.6 * pow(max(dot(rd, uSunDir) * 0.5 + 0.5, 0.0), 2.0));
  vec3 col = mix(body, reflection, fresnel) + glitter;

  // Scattered whitecaps on the tallest crests.
  float foam = smoothstep(0.55, 0.95, w.x) * smoothstep(0.45, 0.75, fbm(p.xz * 0.35 + uTime * 0.05, 3));
  col += foam * (uSunColor * 0.5 + uHorizon * 0.35 + uMoonColor * 0.4) * (1.0 - smoothstep(60.0, 400.0, t));

  float haze = 1.0 - exp(-t * 0.00018);
  return mix(col, uHorizon * mix(0.85, 1.0, uLight), haze);
}
`,
}
