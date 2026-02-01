#version 330

// Shadow shader - darkens areas with low potential field
// Creates contrast between light and dark regions

in vec2 fragTexCoord;
out vec4 finalColor;

uniform sampler2D texture0;     // Current potential field
uniform sampler2D texture1;     // Previous potential field
uniform vec2 resolution;
uniform vec2 cameraPos;
uniform float cameraZoom;
uniform vec2 worldSize;
uniform float blend;
uniform float shadowStrength;   // How dark the shadows get (0-1)

void main() {
    // Convert screen UV to world coordinates
    vec2 screenOffset = (fragTexCoord - 0.5) * resolution;
    vec2 worldOffset = screenOffset / cameraZoom;
    vec2 worldPos = cameraPos + worldOffset;

    // Wrap to toroidal world
    worldPos = mod(mod(worldPos, worldSize) + worldSize, worldSize);

    // Convert world position to potential texture UV
    vec2 potentialUV = worldPos / worldSize;

    // Sample potential field from both textures and blend
    float prevPotential = texture(texture1, potentialUV).r;
    float currPotential = texture(texture0, potentialUV).r;
    float potential = mix(prevPotential, currPotential, blend);

    // Invert and smooth - low potential = more shadow
    // smoothstep creates nice falloff at edges
    float shadow = 1.0 - smoothstep(0.15, 0.7, potential);

    // Apply shadow strength - controls max darkness
    float darkness = shadow * shadowStrength;

    // Output black with alpha for darkening
    // Alpha blending: result = src * alpha + dst * (1 - alpha)
    // So black with alpha will darken the background
    finalColor = vec4(0.0, 0.0, 0.0, darkness);
}
