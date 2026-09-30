// Shared GLSL for every scene. Scenes supply only their own `render()` (and an
// optional particle pass); uniforms, noise, sky helpers, tone mapping and
// dithering live here so each scene chunk stays a few kilobytes.

export const UNIFORMS = /* glsl */ `
uniform vec2 uResolution;
uniform float uTime;
uniform float uHour;
uniform vec2 uPointer;
uniform float uDetail;
uniform vec3 uZenith;
uniform vec3 uHorizon;
uniform vec3 uGlow;
uniform vec3 uSunColor;
uniform vec3 uMoonColor;
uniform vec3 uSunDir;
uniform vec3 uMoonDir;
uniform float uLight;
uniform float uStars;
`

export const HELPERS = /* glsl */ `
const float PI = 3.14159265;
const float TAU = 6.28318531;

// Sine-free hashes (Dave Hoskins) stay stable across GPU vendors.
float hash11(float p) { p = fract(p * 0.1031); p *= p + 33.33; p *= p + p; return fract(p); }
float hash12(vec2 p) { vec3 p3 = fract(vec3(p.xyx) * 0.1031); p3 += dot(p3, p3.yzx + 33.33); return fract((p3.x + p3.y) * p3.z); }
vec2 hash22(vec2 p) { vec3 p3 = fract(vec3(p.xyx) * vec3(0.1031, 0.1030, 0.0973)); p3 += dot(p3, p3.yzx + 33.33); return fract((p3.xx + p3.yz) * p3.zy); }
vec3 hash32(vec2 p) { vec3 p3 = fract(vec3(p.xyx) * vec3(0.1031, 0.1030, 0.0973)); p3 += dot(p3, p3.yxz + 33.33); return fract((p3.xxy + p3.yzz) * p3.zyx); }

float noise(vec2 p) {
  vec2 i = floor(p);
  vec2 f = fract(p);
  vec2 u = f * f * (3.0 - 2.0 * f);
  float a = hash12(i);
  float b = hash12(i + vec2(1.0, 0.0));
  float c = hash12(i + vec2(0.0, 1.0));
  float d = hash12(i + vec2(1.0, 1.0));
  return mix(mix(a, b, u.x), mix(c, d, u.x), u.y);
}

float noise1(float x) {
  float i = floor(x);
  float f = fract(x);
  return mix(hash11(i), hash11(i + 1.0), f * f * (3.0 - 2.0 * f));
}

// Octave counts shrink with the quality level so slow GPUs keep their frame rate.
int octaves(float full) { return max(1, int(ceil(full * mix(0.55, 1.0, uDetail)))); }

const mat2 OCTAVE = mat2(1.6, 1.2, -1.2, 1.6);

float fbm(vec2 p, int count) {
  float value = 0.0;
  float amplitude = 0.5;
  float total = 0.0;
  for (int i = 0; i < 8; i++) {
    if (i >= count) break;
    value += amplitude * noise(p);
    total += amplitude;
    p = OCTAVE * p + 17.13;
    amplitude *= 0.5;
  }
  return value / total;
}

float fbm1(float x, int count) {
  float value = 0.0;
  float amplitude = 0.5;
  float total = 0.0;
  for (int i = 0; i < 8; i++) {
    if (i >= count) break;
    value += amplitude * noise1(x);
    total += amplitude;
    x = x * 2.03 + 11.7;
    amplitude *= 0.5;
  }
  return value / total;
}

// Sharp-crested ridges for mountain silhouettes.
float ridged1(float x, int count) {
  float value = 0.0;
  float amplitude = 0.5;
  float total = 0.0;
  for (int i = 0; i < 8; i++) {
    if (i >= count) break;
    float n = 1.0 - abs(noise1(x) * 2.0 - 1.0);
    value += amplitude * n * n;
    total += amplitude;
    x = x * 2.1 + 5.3;
    amplitude *= 0.48;
  }
  return value / total;
}

float luma(vec3 c) { return dot(c, vec3(0.2126, 0.7152, 0.0722)); }

// Camera looking down +z; the pointer rotates it a little for depth.
vec3 cameraRay(vec2 fragCoord, float focal, float pitch, float sway) {
  vec2 uv = (fragCoord - 0.5 * uResolution) / uResolution.y;
  vec3 rd = normalize(vec3(uv, focal));
  float p = pitch - uPointer.y * sway * 0.5;
  float y = uPointer.x * sway;
  rd = vec3(rd.x, rd.y * cos(p) + rd.z * sin(p), -rd.y * sin(p) + rd.z * cos(p));
  return vec3(rd.x * cos(y) + rd.z * sin(y), rd.y, -rd.x * sin(y) + rd.z * cos(y));
}

// Angular size of one pixel for a camera with the given focal length.
float pixelAngle(float focal) { return 1.0 / (uResolution.y * focal); }

// Time-of-day sky ------------------------------------------------------------

vec3 skyGradient(vec3 rd) {
  float y = max(rd.y, 0.0);
  // Blend in a perceptual space so the horizon colour stays a band, not a wash.
  vec3 col = mix(sqrt(uZenith), sqrt(uHorizon), exp(-y * 7.0));
  col *= col;
  float mu = max(dot(normalize(vec3(rd.x, 0.0, rd.z)), normalize(vec3(uSunDir.x, 0.0, uSunDir.z))), 0.0);
  float band = exp(-y * 9.0);
  col = mix(col, uGlow, band * (0.15 + 0.85 * pow(mu, 4.0)) * smoothstep(-0.3, 0.05, uSunDir.y) * 0.8);
  // Forward scattering around the sun.
  float s = max(dot(rd, uSunDir), 0.0);
  col += uSunColor * (pow(s, 10.0) * 0.07 + pow(s, 160.0) * 0.45);
  float m = max(dot(rd, uMoonDir), 0.0);
  col += uMoonColor * pow(m, 48.0) * 0.18;
  return col;
}

vec3 sunAndMoon(vec3 rd, float px) {
  vec3 col = vec3(0.0);
  float s = acos(clamp(dot(rd, uSunDir), -1.0, 1.0));
  col += uSunColor * 14.0 * smoothstep(0.0105 + px, 0.0105 - px, s);
  float m = acos(clamp(dot(rd, uMoonDir), -1.0, 1.0));
  float disc = smoothstep(0.0125 + px, 0.0125 - px, m);
  if (disc > 0.0) {
    vec3 side = normalize(cross(uMoonDir, vec3(0.0, 1.0, 0.0)));
    vec3 up = cross(side, uMoonDir);
    vec2 q = vec2(dot(rd, side), dot(rd, up)) / 0.0125;
    float maria = fbm(q * 2.2 + 3.0, 3);
    float lit = 0.78 + 0.22 * smoothstep(0.35, 0.65, maria);
    col += uMoonColor * 9.0 * disc * lit;
  }
  return col;
}

// Hashed star cells in view-angle space; stars stay about one pixel wide.
float starField(vec3 rd, float density, float px, float seed) {
  vec2 p = vec2(atan(rd.x, rd.z), rd.y) * density + seed;
  vec2 cell = floor(p);
  vec3 h = hash32(cell);
  if (h.z > 0.36) return 0.0;
  vec2 center = 0.5 + (h.xy - 0.5) * 0.7;
  float d = length(fract(p) - center) / density;
  float size = px * (0.7 + h.z * 1.6);
  float twinkle = 0.65 + 0.35 * sin(uTime * (0.8 + h.x * 2.6) + h.y * TAU);
  return smoothstep(size, 0.0, d) * pow(1.0 - h.z / 0.36, 2.2) * twinkle;
}

vec3 stars(vec3 rd, float px) {
  if (rd.y < 0.0) return vec3(0.0);
  float s = starField(rd, 170.0, px, 0.0) * 1.3 + starField(rd, 72.0, px, 31.7) * 2.6;
  vec3 tint = mix(vec3(0.75, 0.82, 1.0), vec3(1.0, 0.9, 0.78), hash12(floor(rd.xy * 300.0)));
  return tint * s * smoothstep(0.0, 0.06, rd.y);
}

// A drifting cloud deck lit by the sun and moon. Returns colour and coverage.
vec4 cloudDeck(vec3 ro, vec3 rd, float height, float scale, float coverage, float speed) {
  if (rd.y < 0.012) return vec4(0.0);
  float t = (height - ro.y) / rd.y;
  vec2 p = (ro.xz + rd.xz * t) * scale;
  vec2 wind = vec2(uTime * speed, uTime * speed * 0.35);
  int count = octaves(6.0);
  vec2 warp = vec2(fbm(p * 0.5 + wind * 0.5, 3), fbm(p * 0.5 + 7.7 - wind * 0.3, 3));
  vec2 q = p + warp * 1.6 + wind;
  float n = fbm(q, count);
  float density = smoothstep(1.0 - coverage, 1.0 - coverage + 0.32, n);
  if (density <= 0.001) return vec4(0.0);
  vec2 toLight = normalize(uSunDir.xz + vec2(0.0001)) * 0.35;
  float toward = fbm(q + toLight, max(2, count - 2));
  float lit = clamp(0.55 + (n - toward) * 4.0, 0.0, 1.0);
  float sunUp = smoothstep(-0.05, 0.1, uSunDir.y);
  vec3 shade = mix(uZenith * 0.6 + uHorizon * 0.25, uHorizon * 0.9, 0.35 + 0.35 * uLight);
  vec3 lightCol = uSunColor * 1.2 + uMoonColor * 0.8 + uGlow * (1.0 - uLight) * 0.4;
  vec3 col = shade * mix(0.55, 1.0, density) + lightCol * lit * mix(0.35, 0.9, sunUp);
  // Thin cloud edges catch light when the sun is behind them.
  float behind = pow(max(dot(rd, uSunDir), 0.0), 8.0);
  col += uSunColor * behind * (1.0 - density) * 1.5;
  float fade = smoothstep(0.012, 0.16, rd.y);
  col = mix(uHorizon, col, fade);
  return vec4(col, density * mix(0.35, 1.0, fade));
}
`

export const FINISH = /* glsl */ `
out vec4 fragColor;

// Linear below 0.8, then a soft shoulder so suns and glints roll off to white.
vec3 shoulder(vec3 c) {
  vec3 over = max(c - 0.8, 0.0);
  return min(c, 0.8) + 0.2 * (1.0 - exp(-over / 0.2));
}

void main() {
  vec3 col = shoulder(max(render(gl_FragCoord.xy), 0.0));
  col = pow(col, vec3(1.0 / 2.2));
  // Triangular dither breaks up banding in the long sky gradients.
  float dither = hash12(gl_FragCoord.xy) + hash12(gl_FragCoord.xy + 71.3) - 1.0;
  fragColor = vec4(col + dither / 255.0, 1.0);
}
`

export const VERTEX_FULLSCREEN = /* glsl */ `#version 300 es
void main() {
  vec2 p = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2));
  gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}
`

const HEADER = '#version 300 es\nprecision highp float;\nprecision highp int;\n'

export function fragmentSource(body: string): string {
  return `${HEADER}${UNIFORMS}${HELPERS}${body}\n${FINISH}`
}

export function particleVertexSource(body: string): string {
  return `${HEADER}${UNIFORMS}${HELPERS}${body}`
}

export function particleFragmentSource(body: string): string {
  return `${HEADER}${UNIFORMS}out vec4 fragColor;\n${body}`
}
