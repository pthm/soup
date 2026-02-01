---
name: shader-debug
description: This skill should be used when debugging GLSL shaders, when shader output looks wrong, when the user mentions "shader not working", "debug shader", "test shader", or when troubleshooting GPU rendering issues in this project.
version: 1.0.0
---

# Shader Debug Skill

Debug GLSL fragment shaders by rendering them to PNG files.

## When to Use

- Shader output doesn't look right in the game
- Need to inspect shader values without running the simulation
- Iterating on shader code and want quick visual feedback
- Debugging coordinate systems, UV mapping, or color output

## Tool Location

```
cmd/shaderdebug/main.go
```

## Basic Usage

```bash
# Render a shader to PNG
go run ./cmd/shaderdebug -shader shaders/resource.fs -out /tmp/debug.png

# View the result
open /tmp/debug.png  # macOS
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-shader` | `shaders/resource.fs` | Fragment shader path |
| `-out` | `debug.png` | Output PNG path |
| `-width` | 512 | Render width |
| `-height` | 512 | Render height |

## Shader Requirements

The shader must accept these uniforms (automatically set by the tool):
- `uniform float time;` - Set to 0.0
- `uniform vec2 resolution;` - Set to (width, height)

Standard inputs available:
- `in vec2 fragTexCoord;` - UV coordinates (0,0 at top-left, 1,1 at bottom-right after flip)

## Debugging Workflow

### 1. Create a minimal test shader

```glsl
#version 330
in vec2 fragTexCoord;
out vec4 finalColor;
uniform vec2 resolution;

void main() {
    // Output UV as color to verify coordinates
    finalColor = vec4(fragTexCoord.x, fragTexCoord.y, 0.0, 1.0);
}
```

### 2. Test the pipeline

```bash
go run ./cmd/shaderdebug -shader shaders/debug_test.fs -out /tmp/uv.png
```

Expected: Gradient from black (top-left) to yellow (bottom-right).

### 3. Isolate the problem

Add intermediate outputs:
```glsl
// Debug: output distance as brightness
float dist = length(uv - center);
finalColor = vec4(vec3(dist), 1.0);
```

### 4. Use the Read tool to view results

```
Read /tmp/debug.png
```

The assistant can view PNG files directly and describe what it sees.

## Common Issues

### Shader appears all black
- Check if values are in valid range [0, 1]
- Verify uniforms are being set correctly
- Test with debug_test.fs to verify pipeline works

### Coordinates seem wrong
- fragTexCoord has (0,0) at top-left after image flip
- For world coordinates: `vec2 worldPos = fragTexCoord * resolution;`
- Remember toroidal wrapping if simulating torus

### Values too dark/bright
- Normalize values to [0, 1] range
- Use `clamp()` to prevent overflow
- Check math for division by zero

## Pre-made Debug Shaders

| Shader | Purpose |
|--------|---------|
| `shaders/debug_test.fs` | UV coordinates as colors |
| `shaders/debug_circle.fs` | Single circle at center |
| `shaders/debug_dist.fs` | Distance from center |

## Example: Debug Resource Field

```bash
# Test resource shader at game resolution
go run ./cmd/shaderdebug \
  -shader shaders/resource.fs \
  -out /tmp/resource.png \
  -width 1280 -height 720

# Check the output
open /tmp/resource.png
```

If hotspots aren't visible:
1. Check radius values (might be too small relative to resolution)
2. Verify UV vs world coordinate usage
3. Test with a simple single-hotspot shader first
