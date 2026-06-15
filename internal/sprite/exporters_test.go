package sprite

import (
	"encoding/json"
	"strings"
	"testing"
)

func sampleManifest() Manifest {
	return Manifest{
		App: "perfectpixel", Schema: "v2", Version: 2, Character: "hero",
		Sheet: SheetInfo{Image: "hero-sprite-sheet.png", Width: 256, Height: 128, CellWidth: 64, CellHeight: 64},
		Animations: map[string]AnimationEntry{
			"walk": {Row: 0, Frames: 2, FPS: 8, Loop: true,
				Rects: []FrameRect{{0, 0, 64, 64}, {64, 0, 64, 64}}},
			"attack": {Row: 1, Frames: 2, FPS: 12, Loop: false,
				Rects: []FrameRect{{0, 64, 64, 64}, {64, 64, 64, 64}}},
		},
	}
}

func TestBuildGodotTres(t *testing.T) {
	out := string(BuildGodotTres(sampleManifest(), "hero-sprite-sheet.png"))
	for _, want := range []string{
		"[gd_resource type=\"SpriteFrames\"", "ext_resource type=\"Texture2D\"",
		"AtlasTexture", "region = Rect2(", "&\"walk\"", "&\"attack\"", "\"speed\": 8.0", "\"loop\": false",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("Godot .tres에 %q 누락\n%s", want, out)
		}
	}
	// AtlasTexture 서브리소스 4개(총 프레임) 있어야
	if n := strings.Count(out, "[sub_resource type=\"AtlasTexture\""); n != 4 {
		t.Fatalf("AtlasTexture 4개 기대, %d개", n)
	}
}

func TestBuildTexturePackerJSON(t *testing.T) {
	raw, err := BuildTexturePackerJSON(sampleManifest())
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("유효하지 않은 JSON: %v", err)
	}
	frames, ok := parsed["frames"].(map[string]any)
	if !ok || len(frames) != 4 {
		t.Fatalf("frames 4개 기대, got %v", parsed["frames"])
	}
	if _, ok := frames["walk 0"]; !ok {
		t.Fatalf("'walk 0' 프레임 키 누락")
	}
	meta, _ := parsed["meta"].(map[string]any)
	if meta["image"] != "hero-sprite-sheet.png" {
		t.Fatalf("meta.image 불일치: %v", meta["image"])
	}
}
