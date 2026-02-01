#version 330

in vec2 fragTexCoord;
out vec4 finalColor;

uniform float time;
uniform vec2 resolution;

void main() {
    // Simple test: output texture coordinates as colors
    // Red = X position, Green = Y position
    finalColor = vec4(fragTexCoord.x, fragTexCoord.y, 0.0, 1.0);
}
