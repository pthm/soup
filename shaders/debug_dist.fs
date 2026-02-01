#version 330

in vec2 fragTexCoord;
out vec4 finalColor;

uniform float time;
uniform vec2 resolution;

void main() {
    vec2 uv = fragTexCoord;
    vec2 center = vec2(0.5, 0.5);
    float dist = length(uv - center);

    // Output distance as brightness (closer = brighter)
    // Distance from center to corner is ~0.707, so scale by that
    float value = 1.0 - dist * 1.414;  // 1/0.707 ≈ 1.414

    finalColor = vec4(value, value, value, 1.0);
}
