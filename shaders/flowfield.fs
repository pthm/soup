#version 330

in vec2 fragTexCoord;
out vec4 finalColor;

uniform float time;
uniform vec2 resolution;

// Permutation polynomial hash
vec3 permute(vec3 x) {
    return mod(((x * 34.0) + 1.0) * x, 289.0);
}

// Simplex 2D noise - returns value in [-1, 1]
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

void main() {
    // Flow field: simple noise -> angle -> vector
    // Based on Nature of Code flow fields

    const float noiseScale = 0.006;   // Spatial frequency of flow patterns
    const float timeScale = 0.00015;  // How fast patterns evolve
    const float flowStrength = 0.06;  // Base strength of flow vectors

    // World position
    vec2 worldPos = fragTexCoord * resolution;

    // Single noise sample -> angle (Nature of Code approach)
    float noise = snoise(vec2(
        worldPos.x * noiseScale + time * timeScale,
        worldPos.y * noiseScale
    ));

    // Map noise [-1, 1] to angle [0, 2π]
    float angle = (noise + 1.0) * 3.14159;

    // Convert angle to unit vector, scale by strength
    float flowX = cos(angle) * flowStrength;
    float flowY = sin(angle) * flowStrength;

    // Encode flow vector as color
    // Map [-0.5, 0.5] to [0, 1] for storage in texture
    float encodedX = clamp(flowX + 0.5, 0.0, 1.0);
    float encodedY = clamp(flowY + 0.5, 0.0, 1.0);
    float magnitude = length(vec2(flowX, flowY));

    finalColor = vec4(encodedX, encodedY, magnitude, 1.0);
}
