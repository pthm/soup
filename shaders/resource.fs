#version 330

out vec4 finalColor;

uniform float time;
uniform vec2 resolution;

// Resource field parameters
const float SCALE = 4.0;        // Patch size: higher = more, smaller patches
const int OCTAVES = 4;
const float LACUNARITY = 2.0;
const float GAIN = 0.5;
const float SEED = 42.0;
const float DRIFT_SPEED = 0.005; // How fast patches drift (UV units per second)

// Tileable hash
float hash12(vec2 p) {
    vec3 p3 = fract(vec3(p.xyx) * 0.1031 + SEED);
    p3 += dot(p3, p3.yzx + 33.33);
    return fract((p3.x + p3.y) * p3.z);
}

// Tileable value noise on a unit torus
float valueNoiseTileable(vec2 uv, float freq) {
    vec2 p = uv * freq;

    vec2 i = floor(p);
    vec2 f = fract(p);

    // Wrap lattice coords so edges match (tiling)
    float fx = freq;
    vec2 i00 = mod(i + vec2(0.0, 0.0), fx);
    vec2 i10 = mod(i + vec2(1.0, 0.0), fx);
    vec2 i01 = mod(i + vec2(0.0, 1.0), fx);
    vec2 i11 = mod(i + vec2(1.0, 1.0), fx);

    float a = hash12(i00);
    float b = hash12(i10);
    float c = hash12(i01);
    float d = hash12(i11);

    // Smoothstep interpolation
    vec2 u = f * f * (3.0 - 2.0 * f);

    return mix(mix(a, b, u.x), mix(c, d, u.x), u.y);
}

// Tileable FBM
float fbm(vec2 uv) {
    float sum = 0.0;
    float amp = 0.5;
    float freq = SCALE;

    for (int o = 0; o < 8; o++) {
        if (o >= OCTAVES) break;
        sum += amp * valueNoiseTileable(uv, freq);
        freq *= LACUNARITY;
        amp *= GAIN;
    }
    return clamp(sum, 0.0, 1.0);
}

void main() {
    // UV in [0, 1] - tiles seamlessly
    vec2 uv = gl_FragCoord.xy / resolution;

    // Drift the resource field over time
    // Different drift directions for visual interest
    vec2 drift = vec2(
        time * DRIFT_SPEED,
        time * DRIFT_SPEED * 0.7  // Slightly different Y speed
    );
    vec2 driftedUV = fract(uv + drift);

    // Base resource from tileable FBM
    float r = fbm(driftedUV);

    // Shape to create stronger grazing patches (more contrast)
    r = pow(r, 1.5);

    // Store in R channel for sampling
    finalColor = vec4(r, r, r, 1.0);
}
