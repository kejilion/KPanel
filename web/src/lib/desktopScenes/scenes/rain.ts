import type { ShaderScene } from '../types'

// A rainy window at night: drops slide down the glass in stick-slip bursts,
// wiping trails through the condensation, and each drop refracts a sharper,
// inverted view of the out-of-focus neon city behind it.

export const scene: ShaderScene = {
  fragment: /* glsl */ `
const vec3 NEON[5] = vec3[5](
  vec3(1.0, 0.1, 0.42),
  vec3(0.05, 0.62, 1.0),
  vec3(1.0, 0.45, 0.08),
  vec3(0.46, 0.22, 1.0),
  vec3(1.0, 0.74, 0.46)
);
const vec3 SKY_TOP = pow(vec3(0.03, 0.025, 0.08), vec3(2.2));
const vec3 SKY_GLOW = pow(vec3(0.3, 0.1, 0.32), vec3(2.2));
const vec3 STREET = pow(vec3(0.05, 0.035, 0.08), vec3(2.2));
const float STREET_Y = -0.2;

vec3 neon(float h) { return NEON[int(h * 4.999)]; }

float softStep(float edge, float blur, float x) { return 1.0 - smoothstep(edge - blur, edge + blur, x); }

// One row of towers. Windows resolve only where the view is in focus; out of
// focus they average into a soft glow. rgb colour, a coverage.
vec4 towers(vec2 uv, float blur, float width, float base, float range, float seed, float shade) {
  float cell = floor(uv.x / width);
  float lx = (fract(uv.x / width) - 0.5) * width;
  vec3 h = hash32(vec2(cell, seed));
  float top = base + h.x * range;
  float halfWidth = width * (0.34 + 0.14 * h.y);
  float cover = softStep(top, blur, uv.y) * softStep(halfWidth, blur, abs(lx));
  if (cover <= 0.001) return vec4(0.0);
  vec3 col = SKY_TOP * shade;
  vec3 warm = vec3(1.0, 0.68, 0.38);
  float focus = 1.0 - smoothstep(0.003, 0.012, blur);
  vec3 windows = warm * 0.035;
  if (focus > 0.0) {
    vec2 pane = vec2((lx + halfWidth) / 0.011, (uv.y - base) / 0.016);
    vec3 w = hash32(floor(pane) + cell * 17.0 + seed);
    float lit = step(0.64, w.x);
    vec2 inPane = abs(fract(pane) - 0.5);
    float glass = softStep(0.28, 0.08, inPane.x) * softStep(0.3, 0.08, inPane.y);
    vec3 paneColor = mix(warm, vec3(0.55, 0.75, 1.0), step(0.8, w.y)) * (0.2 + 0.4 * w.z);
    windows = mix(windows, paneColor * glass * lit * 0.3, focus);
  }
  col += windows * smoothstep(top, top - 0.02, uv.y);
  // A vertical neon sign on some towers.
  if (h.z > 0.68) {
    float sx = (h.y - 0.5) * halfWidth * 1.2;
    float y0 = top - range * 0.3 - 0.05;
    float y1 = top - 0.025;
    float dy = max(max(y0 - uv.y, uv.y - y1), 0.0);
    float d = length(vec2(lx - sx, dy));
    float flicker = 0.8 + 0.2 * step(0.2, fract(uTime * (0.3 + h.x) + h.y));
    col += neon(fract(h.z * 7.3)) * exp(-d / (blur * 1.2 + 0.003)) * 0.35 * flicker;
  }
  return vec4(col, cover);
}

// Point lights seen out of focus: discs whose size grows with the blur while
// their edge stays crisp, as a lens renders them.
vec3 bokeh(vec2 uv, float size, float radius, float blur, float seed, float yMin, float yMax) {
  vec2 cell = floor(uv / size);
  vec2 f = fract(uv / size) - 0.5;
  vec3 h = hash32(cell + seed);
  float cy = (cell.y + 0.5) * size;
  if (h.z < 0.62 || cy < yMin || cy > yMax) return vec3(0.0);
  vec2 center = (h.xy - 0.5) * 0.3;
  float defocus = smoothstep(0.004, 0.03, blur);
  float r = mix(0.004, radius * (0.4 + 0.6 * h.x), defocus) / size;
  float edge = r * 0.1 + 0.01;
  float d = length(f - center);
  float disc = softStep(r, edge, d);
  float rim = smoothstep(r * 0.6, r, d) * disc;
  // Energy spreads over the disc, so big bokeh are dimmer than sharp points.
  float energy = mix(2.5, 0.28, defocus);
  float flicker = 0.85 + 0.15 * sin(uTime * (0.4 + h.y * 1.6) + h.x * 20.0);
  return neon(h.y) * (disc + rim * 0.6) * energy * flicker * (0.35 + 0.65 * (h.z - 0.62) / 0.38);
}

// Head- and tail-light streams along the street.
vec3 traffic(vec2 uv, float blur) {
  float lane = exp(-abs(uv.y - STREET_Y + 0.012) / (0.006 + blur));
  float farLane = exp(-abs(uv.y - STREET_Y - 0.004) / (0.004 + blur));
  float heads = pow(noise1(uv.x * 26.0 - uTime * 1.6), 6.0) * 3.0;
  float tails = pow(noise1(uv.x * 22.0 + uTime * 1.2 + 40.0), 6.0) * 3.0;
  return vec3(1.0, 0.82, 0.6) * heads * lane * 0.35 + vec3(1.0, 0.08, 0.05) * tails * farLane * 0.45;
}

vec3 city(vec2 uv, float blur) {
  vec3 col = mix(SKY_GLOW, SKY_TOP, smoothstep(-0.15, 0.5, uv.y));
  vec4 far = towers(uv + vec2(0.37, 0.0), blur, 0.085, 0.0, 0.2, 3.0, 1.6);
  col = mix(col, far.rgb + SKY_GLOW * 0.3, far.a);
  vec4 near = towers(uv, blur, 0.16, -0.08, 0.42, 9.0, 0.8);
  col = mix(col, near.rgb, near.a);
  if (uv.y < STREET_Y) {
    // Wet street: the lights above smear into long reflections.
    vec2 mirrored = vec2(uv.x, STREET_Y + (STREET_Y - uv.y) * 0.45);
    col = mix(STREET, col, 0.15);
    col += bokeh(mirrored, 0.12, 0.04, blur, 1.0, -0.5, 0.2) * 0.35;
  }
  col += bokeh(uv, 0.12, 0.04, blur, 1.0, -0.5, 0.2);
  col += bokeh(uv + vec2(0.05, 0.02), 0.08, 0.026, blur, 2.0, -0.45, 0.35) * 0.8;
  col += bokeh(uv + vec2(0.3, 0.1), 0.2, 0.065, blur, 3.0, -0.35, 0.05) * 0.6;
  col += traffic(uv, blur);
  return col;
}

// Stick-slip motion: pauses, then a quick slide; always monotonic.
float slip(float u) { return u - sin(u * TAU * 3.0) / (TAU * 3.0) * 0.92; }

// Sliding drops with trails. xy: refraction offset, z: how clear the glass is.
vec4 slidingDrops(vec2 uv, float scale, float seed) {
  vec2 grid = vec2(1.0, 2.6);
  vec2 p = uv * scale;
  float column = floor(p.x / grid.x);
  float speed = 0.05 + 0.05 * hash11(column * 3.1 + seed);
  p.y += uTime * speed * grid.y * 0.4 + hash11(column + seed) * 7.0;
  vec2 id = floor(p / grid);
  vec2 q = fract(p / grid) * grid;
  vec3 h = hash32(id + seed * 13.0);
  if (h.z < 0.25) return vec4(0.0);
  float life = fract(uTime * (0.05 + 0.06 * h.y) + h.z);
  float y = grid.y * (0.9 - 0.8 * slip(life));
  float wobble = sin(q.y * 4.0 + h.x * TAU) * 0.08;
  float x = 0.5 + (h.x - 0.5) * 0.5 + wobble;
  float r = 0.13 + 0.07 * h.y;
  vec2 d = (q - vec2(x, y)) * vec2(1.0, 0.85);
  float drop = smoothstep(r, r * 0.55, length(d));
  float above = step(y, q.y);
  float trail = smoothstep(r * 0.55, r * 0.2, abs(q.x - x)) * above * exp(-(q.y - y) * 0.9);
  // Beads left behind in the trail.
  float bead = length(vec2(q.x - x, (fract(q.y * 6.0) - 0.5) / 6.0));
  float beads = smoothstep(r * 0.32, r * 0.12, bead) * trail * step(0.4, hash11(floor(q.y * 6.0) + id.x));
  vec2 offset = -d / r * drop + vec2(q.x - x, 0.0) * beads * 4.0;
  return vec4(offset, max(trail, drop), max(drop, beads));
}

// Condensation beads that swell and evaporate in place.
vec4 staticDrops(vec2 uv, float scale, float seed) {
  vec2 p = uv * scale;
  vec2 id = floor(p);
  vec2 q = fract(p) - 0.5;
  vec3 h = hash32(id + seed);
  vec2 center = (h.xy - 0.5) * 0.5;
  float grow = sin(uTime * (0.05 + 0.1 * h.z) + h.x * TAU) * 0.5 + 0.5;
  float r = 0.3 * h.z * smoothstep(0.1, 0.6, grow);
  vec2 d = q - center;
  float drop = smoothstep(r, r * 0.4, length(d)) * step(0.3, h.z);
  return vec4(-d / max(r, 0.01) * drop, 0.0, drop);
}

vec3 render(vec2 fragCoord) {
  vec2 uv = (fragCoord - 0.5 * uResolution) / uResolution.y;
  vec4 big = slidingDrops(uv, 3.2, 1.0);
  vec4 small = slidingDrops(uv * 1.9 + vec2(3.7, 1.3), 3.2, 4.0);
  vec4 beads = staticDrops(uv, 26.0, 2.0) * smoothstep(0.0, 0.7, uDetail + 0.3);
  vec2 offset = big.xy * 0.045 + small.xy * 0.026 + beads.xy * 0.012;
  float clear = max(big.z, small.z * 0.8);
  float lens = max(big.w, small.w);
  // Condensation blurs the view; drops and their trails bring it partly into focus.
  float blur = mix(0.03, 0.017, max(clear, lens));
  vec2 parallax = uPointer * vec2(0.012, -0.008);
  vec3 col = city(uv + offset + parallax, blur);
  // Condensation scatters a little light over the fogged glass.
  col = mix(col, col * 0.85 + SKY_GLOW * 0.12, (1.0 - max(clear, lens)) * 0.4);
  // Drop edges bend light away and darken; the upper rim catches a highlight.
  col *= 1.0 - lens * (1.0 - lens) * 1.2;
  vec2 facing = -(big.xy + small.xy);
  col += vec3(0.85, 0.8, 1.0) * pow(max(dot(facing, normalize(vec2(-0.4, 0.9))), 0.0), 5.0) * lens * 0.05;
  float vignette = smoothstep(1.25, 0.3, length(uv * vec2(0.8, 1.0)));
  return col * mix(0.55, 1.0, vignette);
}
`,
}
