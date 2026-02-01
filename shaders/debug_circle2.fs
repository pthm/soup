#version 330

in vec2 fragTexCoord;
out vec4 finalColor;

uniform float time;
uniform vec2 resolution;

void main() {
    // Use fragTexCoord directly (0-1 range), no resolution dependency
    vec2 uv = fragTexCoord;

    // Single hotspot at center (0.5, 0.5)
    vec2 center = vec2(0.5, 0.5);

    // Distance in UV space
    float dist = length(uv - center);

    // Circle with radius 0.2 (20% of screen)
    float radius = 0.2;
    float t = clamp(dist / radius, 0.0, 1.0);
    float value = 1.0 - t;  // Linear falloff

    finalColor = vec4(value, value, value, 1.0);
}
