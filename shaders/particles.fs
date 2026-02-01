#version 330

// Particle glow shader - adds soft glow and twinkling to particle render texture

in vec2 fragTexCoord;
out vec4 finalColor;

uniform sampler2D texture0;  // Input particle texture (raw dots)
uniform float time;
uniform vec2 resolution;

void main() {
    vec2 uv = fragTexCoord;
    vec4 center = texture(texture0, uv);

    // Gaussian-ish blur for glow effect (13-tap)
    vec2 texelSize = 1.0 / resolution;
    float glowRadius = 3.0;

    vec4 glow = vec4(0.0);
    float totalWeight = 0.0;

    for (float y = -2.0; y <= 2.0; y += 1.0) {
        for (float x = -2.0; x <= 2.0; x += 1.0) {
            vec2 offset = vec2(x, y) * texelSize * glowRadius;
            float dist = length(vec2(x, y));
            float weight = exp(-dist * dist * 0.3);
            glow += texture(texture0, uv + offset) * weight;
            totalWeight += weight;
        }
    }
    glow /= totalWeight;

    // Combine sharp center with soft glow
    // Center particles are brighter, glow fades out
    vec4 result = center * 1.5 + glow * 0.8;

    // Twinkling - use frag position to create variation
    float twinkle = sin(time * 3.0 + uv.x * 50.0 + uv.y * 37.0) * 0.5 + 0.5;
    twinkle = twinkle * 0.3 + 0.7;  // Range 0.7 to 1.0

    result.rgb *= twinkle;
    result.a = min(result.a, 1.0);

    finalColor = result;
}
