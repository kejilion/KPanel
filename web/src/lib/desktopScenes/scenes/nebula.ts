import type { ShaderScene } from '../types'

// Deep space: a domain-warped emission nebula threaded with dark dust lanes,
// slowly turning, over three star layers that shift with the pointer at
// different depths; a few bright stars carry soft diffraction spikes.

export const scene: ShaderScene = {
  fragment: /* glsl */ `
const vec3 DEEP = pow(vec3(0.012, 0.014, 0.04), vec3(2.2));
const vec3 VIOLET = pow(vec3(0.36, 0.16, 0.62), vec3(2.2));
const vec3 MAGENTA = pow(vec3(0.85, 0.26, 0.52), vec3(2.2));
const vec3 TEAL = pow(vec3(0.12, 0.58, 0.66), vec3(2.2));
const vec3 EMBER = pow(vec3(1.0, 0.66, 0.38), vec3(2.2));

mat2 rotation(float a) { return mat2(cos(a), sin(a), -sin(a), cos(a)); }

vec3 nebula(vec2 p) {
  int count = octaves(6.0);
  float t = uTime * 0.006;
  vec2 q = vec2(fbm(p + vec2(0.0, t), count), fbm(p + vec2(5.2, 1.3) - t, count));
  vec2 r = vec2(fbm(p + 2.2 * q + vec2(1.7, 9.2) + t * 1.5, count), fbm(p + 2.2 * q + vec2(8.3, 2.8), count));
  float f = fbm(p + 2.0 * r, count);
  // Colour follows the warp: violet clouds, magenta knots, teal filaments, a warm core.
  vec3 col = mix(VIOLET, MAGENTA, smoothstep(0.45, 0.8, r.x) * 0.85);
  col = mix(col, TEAL, smoothstep(0.62, 0.9, length(q)) * 0.7);
  col = mix(col, EMBER, smoothstep(0.66, 0.9, f) * 0.6);
  // Large voids keep the nebula to a few clouds with dark space between.
  float clouds = smoothstep(0.38, 0.72, fbm(p * 0.42 + vec2(3.0, 7.0), 4));
  float density = smoothstep(0.32, 0.85, f) * clouds;
  float dust = smoothstep(0.5, 0.78, fbm(p * 2.3 + r * 1.5 + 11.0, max(2, count - 1)));
  return col * density * density * 1.7 * (1.0 - dust * 0.85);
}

float starLayer(vec2 uv, float density, float size, float chance, float seed, out vec3 tint) {
  vec2 p = uv * density + seed;
  vec2 id = floor(p);
  vec3 h = hash32(id);
  tint = mix(vec3(0.7, 0.8, 1.0), vec3(1.0, 0.85, 0.7), h.y);
  if (h.z > chance) return 0.0;
  vec2 center = 0.5 + (h.xy - 0.5) * 0.7;
  float d = length(fract(p) - center) / density;
  float px = 1.0 / uResolution.y;
  float twinkle = 0.7 + 0.3 * sin(uTime * (0.6 + h.x * 2.0) + h.y * TAU);
  return smoothstep(size * px * (0.8 + h.z / chance), 0.0, d) * pow(1.0 - h.z / chance, 2.0) * twinkle;
}

vec3 brightStars(vec2 uv) {
  vec2 p = uv * 3.2 + 41.0;
  vec2 id = floor(p);
  vec3 h = hash32(id);
  if (h.z > 0.22) return vec3(0.0);
  vec2 d = (fract(p) - 0.5 - (h.xy - 0.5) * 0.6) / 3.2;
  float px = 1.0 / uResolution.y;
  float core = exp(-dot(d, d) / (px * px * 2.5));
  float glow = exp(-length(d) / (px * 9.0)) * 0.12;
  float spikes = (exp(-abs(d.x) / (px * 0.8)) * exp(-abs(d.y) / (px * 28.0)) + exp(-abs(d.y) / (px * 0.8)) * exp(-abs(d.x) / (px * 28.0))) * 0.35;
  vec3 tint = mix(vec3(0.75, 0.85, 1.0), vec3(1.0, 0.88, 0.72), h.x);
  float twinkle = 0.8 + 0.2 * sin(uTime * (0.5 + h.y) + h.x * TAU);
  return tint * (core * 2.0 + glow + spikes) * twinkle * (1.0 - h.z / 0.22);
}

vec3 render(vec2 fragCoord) {
  vec2 uv = (fragCoord - 0.5 * uResolution) / uResolution.y;
  vec2 look = uPointer * vec2(1.0, -1.0);
  float turn = uTime * 0.0035;
  vec2 cloudUV = rotation(turn) * (uv + look * 0.018) * 1.35 + vec2(2.1, -0.7);
  vec3 col = DEEP * (1.0 + 0.6 * smoothstep(0.9, 0.0, length(uv)));
  col += nebula(cloudUV);
  vec3 tint;
  float far = starLayer(rotation(turn) * (uv + look * 0.006), 140.0, 1.1, 0.22, 0.0, tint);
  col += tint * far * 0.7;
  float mid = starLayer(rotation(turn) * (uv + look * 0.012), 60.0, 1.5, 0.3, 17.0, tint);
  col += tint * mid * 1.3;
  float near = starLayer(rotation(turn) * (uv + look * 0.028), 22.0, 2.1, 0.3, 31.0, tint);
  col += tint * near * 2.0;
  col += brightStars(rotation(turn) * (uv + look * 0.022));
  float vignette = smoothstep(1.3, 0.35, length(uv * vec2(0.85, 1.0)));
  return col * mix(0.6, 1.0, vignette);
}
`,
}
