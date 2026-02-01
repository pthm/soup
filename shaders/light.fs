#version 330

// Light shader - renders caustic sunlight using FBM noise
// Potential field masks where caustics are visible
// Renders in worldspace with camera-aware transforms

in vec2 fragTexCoord;
out vec4 finalColor;

uniform sampler2D texture0;     // Current potential field
uniform sampler2D texture1;     // Previous potential field
uniform float time;
uniform vec2 resolution;
uniform vec2 cameraPos;         // Camera center in world coordinates
uniform float cameraZoom;       // Zoom level (1.0 = 1:1)
uniform vec2 worldSize;         // World dimensions for toroidal wrapping
uniform float blend;            // Blend factor (0 = previous, 1 = current)

// Simplex noise - Ashima Arts (MIT License)
// https://github.com/ashima/webgl-noise

vec3 mod289(vec3 x) {
    return x - floor(x * (1.0 / 289.0)) * 289.0;
}
vec4 mod289(vec4 x) {
    return x - floor(x * (1.0 / 289.0)) * 289.0;
}
vec4 permute(vec4 x) {
   return mod289(((x*34.0)+1.0)*x);
}
vec4 taylorInvSqrt(vec4 r) {
    return 1.79284291400159 - 0.85373472095314 * r;
}

float snoise(vec3 v) {
    const vec2  C = vec2(1.0/6.0, 1.0/3.0) ;
    const vec4  D = vec4(0.0, 0.5, 1.0, 2.0);
    vec3 i  = floor(v + dot(v, C.yyy) );
    vec3 x0 =   v - i + dot(i, C.xxx) ;
    vec3 g = step(x0.yzx, x0.xyz);
    vec3 l = 1.0 - g;
    vec3 i1 = min( g.xyz, l.zxy );
    vec3 i2 = max( g.xyz, l.zxy );
    vec3 x1 = x0 - i1 + C.xxx;
    vec3 x2 = x0 - i2 + C.yyy;
    vec3 x3 = x0 - D.yyy;
    i = mod289(i);
    vec4 p = permute( permute( permute(
               i.z + vec4(0.0, i1.z, i2.z, 1.0 ))
             + i.y + vec4(0.0, i1.y, i2.y, 1.0 ))
             + i.x + vec4(0.0, i1.x, i2.x, 1.0 ));
    float n_ = 0.142857142857;
    vec3  ns = n_ * D.wyz - D.xzx;
    vec4 j = p - 49.0 * floor(p * ns.z * ns.z);
    vec4 x_ = floor(j * ns.z);
    vec4 y_ = floor(j - 7.0 * x_ );
    vec4 x = x_ *ns.x + ns.yyyy;
    vec4 y = y_ *ns.x + ns.yyyy;
    vec4 h = 1.0 - abs(x) - abs(y);
    vec4 b0 = vec4( x.xy, y.xy );
    vec4 b1 = vec4( x.zw, y.zw );
    vec4 s0 = floor(b0)*2.0 + 1.0;
    vec4 s1 = floor(b1)*2.0 + 1.0;
    vec4 sh = -step(h, vec4(0.0));
    vec4 a0 = b0.xzyw + s0.xzyw*sh.xxyy ;
    vec4 a1 = b1.xzyw + s1.xzyw*sh.zzww ;
    vec3 p0 = vec3(a0.xy,h.x);
    vec3 p1 = vec3(a0.zw,h.y);
    vec3 p2 = vec3(a1.xy,h.z);
    vec3 p3 = vec3(a1.zw,h.w);
    vec4 norm = taylorInvSqrt(vec4(dot(p0,p0), dot(p1,p1), dot(p2, p2), dot(p3,p3)));
    p0 *= norm.x;
    p1 *= norm.y;
    p2 *= norm.z;
    p3 *= norm.w;
    vec4 m = max(0.5 - vec4(dot(x0,x0), dot(x1,x1), dot(x2,x2), dot(x3,x3)), 0.0);
    m = m * m;
    return 105.0 * dot( m*m, vec4( dot(p0,x0), dot(p1,x1), dot(p2,x2), dot(p3,x3) ) );
}

// FBM caustics parameters
const vec2 OFFSET = vec2(69.0, 420.0);
const float SCALE = 1.0;
const float AMPLITUDE = 0.4;
const int OCTAVES = 4;
const float LACUNARITY = 2.4;
const float GAIN = 0.5;

float fbm(in vec2 p, float t) {
    float value = 0.0;
    float amplitude = AMPLITUDE;
    vec3 q = vec3(p.xy, t);

    for (int i = 0; i < OCTAVES; i++) {
        float n = snoise(q);
        n = abs(n);
        value += amplitude * n;
        q.xy *= LACUNARITY;
        amplitude *= GAIN;
    }
    return value;
}

vec3 gamma(in vec3 color) {
    return pow(max(color, 0.0), vec3(1.0 / 2.2));
}

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

    // Normalize world position for caustics
    vec2 uv = worldPos / worldSize * 4.0;

    // Generate FBM caustics
    float t = time / 12.0;
    vec2 p = SCALE * uv + OFFSET;
    float n = fbm(p, t);
    n = 0.02 / n;
    n = pow(n, 1.9);

    vec3 color = vec3(n);
    color = gamma(color);

    // Caustic tint - warm white
    color *= vec3(0.95, 0.98, 1.0);

    // Mask by potential field - caustics visible where potential is high
    float mask = smoothstep(0.1, 0.6, potential);
    color *= mask;

    // For additive blending, use brightness as alpha
    float brightness = (color.r + color.g + color.b) / 3.0;
    finalColor = vec4(color, brightness);
}
