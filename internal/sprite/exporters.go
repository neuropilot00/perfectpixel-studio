package sprite

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// 추가 게임엔진 익스포트 포맷: Godot SpriteFrames(.tres), TexturePacker(JSON hash).
// (Aseprite JSON은 aseprite.go에서 별도 제공)

// sortedAnims는 매니페스트의 애니메이션을 행(Row) 순으로 정렬해 반환합니다.
func sortedAnims(m Manifest) []struct {
	name string
	anim AnimationEntry
} {
	out := make([]struct {
		name string
		anim AnimationEntry
	}, 0, len(m.Animations))
	for name, a := range m.Animations {
		out = append(out, struct {
			name string
			anim AnimationEntry
		}{name, a})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].anim.Row < out[j].anim.Row })
	return out
}

// BuildGodotTres는 Godot 4 SpriteFrames 리소스(.tres)를 만듭니다.
// 시트 PNG를 res://<sheetPath>로 참조하고 프레임마다 AtlasTexture 영역을 정의합니다.
func BuildGodotTres(m Manifest, sheetPath string) []byte {
	anims := sortedAnims(m)
	// AtlasTexture 서브리소스 = 전체 프레임 수
	total := 0
	for _, a := range anims {
		total += len(a.anim.Rects)
	}
	var b strings.Builder
	loadSteps := total + 2 // ext_resource 1 + atlas subresources + resource 1
	fmt.Fprintf(&b, "[gd_resource type=\"SpriteFrames\" load_steps=%d format=3]\n\n", loadSteps)
	fmt.Fprintf(&b, "[ext_resource type=\"Texture2D\" path=\"res://%s\" id=\"1\"]\n\n", sheetPath)

	idx := 0
	type frameRef struct {
		sub      string
		duration float64
	}
	animFrames := make(map[string][]frameRef, len(anims))
	for _, a := range anims {
		fps := a.anim.FPS
		if fps <= 0 {
			fps = 8
		}
		for _, r := range a.anim.Rects {
			id := fmt.Sprintf("atlas_%d", idx)
			fmt.Fprintf(&b, "[sub_resource type=\"AtlasTexture\" id=\"%s\"]\n", id)
			b.WriteString("atlas = ExtResource(\"1\")\n")
			fmt.Fprintf(&b, "region = Rect2(%d, %d, %d, %d)\n\n", r.X, r.Y, r.W, r.H)
			animFrames[a.name] = append(animFrames[a.name], frameRef{id, 1.0})
			idx++
		}
	}

	b.WriteString("[resource]\nanimations = [")
	for ai, a := range anims {
		fps := a.anim.FPS
		if fps <= 0 {
			fps = 8
		}
		loop := "true"
		if !a.anim.Loop {
			loop = "false"
		}
		if ai > 0 {
			b.WriteString(", ")
		}
		b.WriteString("{\n\"frames\": [")
		for fi, fr := range animFrames[a.name] {
			if fi > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "{\"duration\": %.1f, \"texture\": SubResource(\"%s\")}", fr.duration, fr.sub)
		}
		fmt.Fprintf(&b, "],\n\"loop\": %s,\n\"name\": &\"%s\",\n\"speed\": %d.0\n}", loop, a.name, fps)
	}
	b.WriteString("]\n")
	return []byte(b.String())
}

// BuildTexturePackerJSON은 TexturePacker "hash" 포맷 JSON을 만듭니다(Unity TexturePacker importer / Phaser atlasHash / PixiJS).
func BuildTexturePackerJSON(m Manifest) ([]byte, error) {
	type rect struct{ X, Y, W, H int }
	frames := map[string]any{}
	tags := []map[string]any{}
	anims := sortedAnims(m)
	idx := 0
	for _, a := range anims {
		from := idx
		for fi, r := range a.anim.Rects {
			frames[fmt.Sprintf("%s %d", a.name, fi)] = map[string]any{
				"frame":            map[string]int{"x": r.X, "y": r.Y, "w": r.W, "h": r.H},
				"rotated":          false,
				"trimmed":          false,
				"spriteSourceSize": map[string]int{"x": 0, "y": 0, "w": r.W, "h": r.H},
				"sourceSize":       map[string]int{"w": r.W, "h": r.H},
			}
			idx++
		}
		tags = append(tags, map[string]any{"name": a.name, "from": from, "to": idx - 1, "direction": "forward"})
	}
	out := map[string]any{
		"frames": frames,
		"meta": map[string]any{
			"app":       "perfectpixel",
			"version":   "1.0",
			"image":     m.Sheet.Image,
			"format":    "RGBA8888",
			"size":      map[string]int{"w": m.Sheet.Width, "h": m.Sheet.Height},
			"scale":     "1",
			"frameTags": tags,
		},
	}
	_ = rect{}
	return json.MarshalIndent(out, "", "  ")
}
