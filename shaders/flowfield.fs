#version 330

in vec2 fragTexCoord;
out vec4 finalColor;

uniform float time;
uniform vec2 resolution;
uniform sampler2D terrainTex; // Terrain distance field (R = distance to solid)

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
    vec2 uv = fragTexCoord;

    // Flow field parameters (match CPU version)
    const float flowScale = 0.003;
    const float timeScale = 0.0001;
    const float baseStrength = 0.08;

    // World position (assuming 1200x800 screen)
    vec2 worldPos = uv * resolution;

    // Sample noise for flow angle and magnitude
    float noiseX = snoise(vec2(worldPos.x * flowScale, worldPos.y * flowScale) + vec2(0.0, time * timeScale));
    float noiseY = snoise(vec2(worldPos.x * flowScale + 100.0, worldPos.y * flowScale + 100.0) + vec2(0.0, time * timeScale));

    // Convert to flow vector
    float flowAngle = noiseX * 3.14159 * 2.0;
    float flowMagnitude = (noiseY + 1.0) * 0.5;

    float flowX = cos(flowAngle) * flowMagnitude * baseStrength;
    float flowY = sin(flowAngle) * flowMagnitude * baseStrength;

    // Add constant drift (downward + slight side-to-side)
    flowY += 0.01;
    flowX += sin(time * 0.0002) * 0.005;

    // Terrain deflection
    // Sample terrain distance (R channel = normalized distance, 0 = solid, 1 = far)
    float terrainDist = texture(terrainTex, uv).r * 100.0; // Denormalize to ~pixels

    if (terrainDist < 40.0) {
        // Compute gradient by sampling neighbors
        float texelSize = 1.0 / 128.0; // Assuming 128x128 terrain texture
        float distLeft = texture(terrainTex, uv + vec2(-texelSize, 0.0)).r * 100.0;
        float distRight = texture(terrainTex, uv + vec2(texelSize, 0.0)).r * 100.0;
        float distUp = texture(terrainTex, uv + vec2(0.0, -texelSize)).r * 100.0;
        float distDown = texture(terrainTex, uv + vec2(0.0, texelSize)).r * 100.0;

        // Gradient points away from terrain
        float gradX = (distRight - distLeft) * 0.5;
        float gradY = (distDown - distUp) * 0.5;

        // Blend based on proximity
        float blend = 1.0 - terrainDist / 40.0;
        flowX += gradX * blend * 0.1;
        flowY += gradY * blend * 0.1;
    }

    // Encode flow vector as color
    // Map [-0.5, 0.5] to [0, 1] for storage
    // R = flowX, G = flowY, B = magnitude (for debugging), A = 1
    float encodedX = flowX + 0.5;
    float encodedY = flowY + 0.5;
    float magnitude = length(vec2(flowX, flowY));

    finalColor = vec4(encodedX, encodedY, magnitude, 1.0);
}
