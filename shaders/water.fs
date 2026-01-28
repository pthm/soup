#version 330

in vec2 fragTexCoord;
out vec4 finalColor;

uniform float time;
uniform vec2 resolution;

// Permutation polynomial hash
vec3 permute(vec3 x) {
    return mod(((x * 34.0) + 1.0) * x, 289.0);
}

// Simplex 2D noise
float snoise(vec2 v) {
    const vec4 C = vec4(0.211324865405187, 0.366025403784439,
                        -0.577350269189626, 0.024390243902439);
    vec2 i  = floor(v + dot(v, C.yy));
    vec2 x0 = v - i + dot(i, C.xx);
    vec2 i1;
    i1 = (x0.x > x0.y) ? vec2(1.0, 0.0) : vec2(0.0, 1.0);
    vec4 x12 = x0.xyxy + C.xxzz;
    x12.xy -= i1;
    i = mod(i, 289.0);
    vec3 p = permute(permute(i.y + vec3(0.0, i1.y, 1.0))
                     + i.x + vec3(0.0, i1.x, 1.0));
    vec3 m = max(0.5 - vec3(dot(x0, x0), dot(x12.xy, x12.xy),
                            dot(x12.zw, x12.zw)), 0.0);
    m = m * m;
    m = m * m;
    vec3 x = 2.0 * fract(p * C.www) - 1.0;
    vec3 h = abs(x) - 0.5;
    vec3 ox = floor(x + 0.5);
    vec3 a0 = x - ox;
    m *= 1.79284291400159 - 0.85373472095314 * (a0 * a0 + h * h);
    vec3 g;
    g.x = a0.x * x0.x + h.x * x0.y;
    g.yz = a0.yz * x12.xz + h.yz * x12.yw;
    return 130.0 * dot(m, g);
}

// Fractal Brownian Motion (multiple octaves of noise)
float fbm(vec2 p, float t) {
    float value = 0.0;
    float amplitude = 0.5;
    float frequency = 1.0;

    // Add time-based movement in different directions per octave
    // Slowed down by 5x for gentle water movement
    for (int i = 0; i < 4; i++) {
        float timeOffset = t * (0.004 + float(i) * 0.002);
        vec2 offset = vec2(
            sin(timeOffset * 0.7 + float(i)) * 0.5,
            cos(timeOffset * 0.5 + float(i) * 1.3) * 0.5
        );
        value += amplitude * snoise(p * frequency + offset);
        frequency *= 2.0;
        amplitude *= 0.5;
    }

    return value;
}

void main() {
    vec2 uv = fragTexCoord;
    vec2 p = uv * 4.0; // Scale for noise detail

    // Multiple layers of noise at different scales and speeds
    float n1 = fbm(p, time);
    float n2 = fbm(p * 0.5 + vec2(100.0), time * 0.7);
    float n3 = fbm(p * 2.0 + vec2(50.0), time * 1.3);

    // Combine noise layers
    float noise = n1 * 0.5 + n2 * 0.35 + n3 * 0.15;
    noise = noise * 0.5 + 0.5; // Normalize to 0-1

    // Dark water colors
    vec3 deepBlue = vec3(0.02, 0.04, 0.08);   // Very dark blue
    vec3 darkTeal = vec3(0.04, 0.08, 0.10);   // Dark teal
    vec3 midTeal = vec3(0.06, 0.12, 0.14);    // Slightly lighter teal

    // Mix colors based on noise
    vec3 color;
    if (noise < 0.4) {
        color = mix(deepBlue, darkTeal, noise / 0.4);
    } else if (noise < 0.7) {
        color = mix(darkTeal, midTeal, (noise - 0.4) / 0.3);
    } else {
        color = mix(midTeal, darkTeal, (noise - 0.7) / 0.3);
    }

    // Add subtle caustic-like highlights
    float caustic = snoise(p * 3.0 + vec2(time * 0.1, time * 0.08));
    caustic = pow(max(caustic, 0.0), 3.0) * 0.15;
    color += vec3(caustic * 0.3, caustic * 0.5, caustic * 0.6);

    finalColor = vec4(color, 1.0);
}
