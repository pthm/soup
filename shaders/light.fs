#version 330

// Light shader - renders potential field as caustic sunlight filtering through water
// Caustics algorithm based on David Hoskins' Shadertoy (originally by joltz0r)

in vec2 fragTexCoord;
out vec4 finalColor;

uniform sampler2D texture0;  // Potential field
uniform float time;
uniform vec2 resolution;

#define TAU 6.28318530718
#define MAX_ITER 5

// Water caustics pattern - creates realistic light refraction effect
float caustics(vec2 uv, float t) {
    vec2 p = mod(uv * TAU, TAU) - 250.0;
    vec2 i = p;
    float c = 1.0;
    float inten = 0.005;

    for (int n = 0; n < MAX_ITER; n++) {
        float nt = t * (1.0 - (3.5 / float(n + 1)));
        i = p + vec2(cos(nt - i.x) + sin(nt + i.y), sin(nt - i.y) + cos(nt + i.x));
        c += 1.0 / length(vec2(p.x / (sin(i.x + nt) / inten), p.y / (cos(i.y + nt) / inten)));
    }
    c /= float(MAX_ITER);
    c = 1.17 - pow(c, 1.4);
    return pow(abs(c), 8.0);
}

void main() {
    vec2 uv = fragTexCoord;

    // Sample potential field
    float potential = texture(texture0, uv).r;

    // Generate caustics - scale UV for nice pattern size
    float t = time * 0.5 + 23.0;
    float caustic = caustics(uv * 2.0, t);

    // Base colors
    vec3 deepWater = vec3(0.0, 0.02, 0.04);    // Very dark blue-black
    vec3 shadowWater = vec3(0.0, 0.06, 0.10);  // Dark blue shadow
    vec3 litWater = vec3(0.0, 0.20, 0.28);     // Lit water (cyan tint)
    vec3 causticColor = vec3(0.4, 0.8, 0.9);   // Bright caustic highlights

    // Light intensity based on potential
    float lightBase = smoothstep(0.05, 0.6, potential);

    // Start with deep water
    vec3 color = deepWater;

    // Add shadow water tint based on potential
    color = mix(color, shadowWater, potential * 0.8);

    // Add lit water where potential is high
    color = mix(color, litWater, lightBase * 0.6);

    // Caustics are visible where light reaches (modulated by potential)
    float causticIntensity = caustic * lightBase;

    // Add caustic highlights
    color += causticColor * causticIntensity * 0.4;

    // Extra bright spots in high-potential areas
    float brightSpot = caustic * potential * potential;
    color += vec3(0.5, 0.9, 1.0) * brightSpot * 0.3;

    finalColor = vec4(color, 1.0);
}
