#version 330

in vec2 fragTexCoord;
out vec4 finalColor;

uniform sampler2D texture0;      // Resource field data (R channel = resource value)
uniform vec2 resolution;
uniform vec2 cameraPos;
uniform float cameraZoom;
uniform vec2 worldSize;
uniform float time;

// Simple hash for organic texture
float hash(vec2 p) {
    return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453);
}

// Value noise for organic variation
float noise(vec2 p) {
    vec2 i = floor(p);
    vec2 f = fract(p);
    f = f * f * (3.0 - 2.0 * f); // smoothstep

    float a = hash(i);
    float b = hash(i + vec2(1.0, 0.0));
    float c = hash(i + vec2(0.0, 1.0));
    float d = hash(i + vec2(1.0, 1.0));

    return mix(mix(a, b, f.x), mix(c, d, f.x), f.y);
}

// FBM for organic texture
float fbm(vec2 p) {
    float sum = 0.0;
    float amp = 0.5;
    for (int i = 0; i < 3; i++) {
        sum += amp * noise(p);
        p *= 2.0;
        amp *= 0.5;
    }
    return sum;
}

void main() {
    // Calculate world UV from screen position
    vec2 screenUV = fragTexCoord;

    // Convert screen position to world position
    vec2 screenPos = screenUV * resolution;
    vec2 worldPos = cameraPos + (screenPos - resolution * 0.5) / cameraZoom;

    // Wrap world coordinates for toroidal geometry
    vec2 worldUV = mod(worldPos / worldSize, 1.0);

    // Sample resource value
    float resource = texture(texture0, worldUV).r;

    // Add organic texture variation
    vec2 noiseCoord = worldPos * 0.02 + time * 0.01;
    float organicNoise = fbm(noiseCoord) * 0.3 + 0.85; // 0.85-1.15 range

    // Algae green color palette
    // Dark murky green at low values, brighter algae green at high
    vec3 colorLow = vec3(0.02, 0.08, 0.03);   // Very dark green
    vec3 colorMid = vec3(0.05, 0.20, 0.08);   // Murky green
    vec3 colorHigh = vec3(0.15, 0.45, 0.12);  // Algae green

    // Blend colors based on resource level
    vec3 color;
    if (resource < 0.5) {
        color = mix(colorLow, colorMid, resource * 2.0);
    } else {
        color = mix(colorMid, colorHigh, (resource - 0.5) * 2.0);
    }

    // Apply organic variation to color
    color *= organicNoise;

    // Alpha: transparent at 0, semi-transparent at cap
    // Use smooth falloff for soft edges
    float alpha = smoothstep(0.0, 0.15, resource) * 0.6;

    // Add subtle pulsing based on resource density
    alpha *= 0.9 + 0.1 * sin(time * 0.5 + resource * 6.28);

    finalColor = vec4(color, alpha);
}
