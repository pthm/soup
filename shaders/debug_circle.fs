#version 330

in vec2 fragTexCoord;
out vec4 finalColor;

uniform float time;
uniform vec2 resolution;

void main() {
    // World position in pixels
    vec2 worldPos = fragTexCoord * resolution;

    // Single hotspot at center
    vec2 center = resolution * 0.5;

    // Distance to center
    float dist = length(worldPos - center);

    // Circle with radius 100
    float radius = 100.0;
    float t = clamp(dist / radius, 0.0, 1.0);
    float value = 1.0 - t;  // Linear falloff

    finalColor = vec4(value, value, value, 1.0);
}
