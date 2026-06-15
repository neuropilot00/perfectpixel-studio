package puppet

import (
	"encoding/json"
	"image"
	"math"
	"testing"

	"perfectpixel/internal/rig"
)

// shiftKeys는 한 파라미터가 모든 정점을 (dx,dy)만큼 미는 1D 바인딩을 만듭니다.
// value=0에서 0 오프셋, value=1에서 (dx,dy).
func shiftKeys(nVerts int, dx, dy float64) []DeformKey {
	zero := make([][2]float64, nVerts)
	full := make([][2]float64, nVerts)
	for i := range full {
		full[i] = [2]float64{dx, dy}
	}
	return []DeformKey{{Value: 0, Offsets: zero}, {Value: 1, Offsets: full}}
}

func TestAutoMeshPart(t *testing.T) {
	p := AutoMeshPart("body", "body.png", 100, 80, 4, 4, 0)
	if p.Mesh == nil {
		t.Fatal("메시 nil")
	}
	wantV := (4 + 1) * (4 + 1)
	if len(p.Mesh.Verts) != wantV {
		t.Fatalf("정점 수 %d, 기대 %d", len(p.Mesh.Verts), wantV)
	}
	// rest 상태: 메시가 텍스처 전체(0..100, 0..80)를 덮어야
	var maxX, maxY float64
	for _, v := range p.Mesh.Verts {
		maxX = math.Max(maxX, v.X)
		maxY = math.Max(maxY, v.Y)
	}
	if maxX != 100 || maxY != 80 {
		t.Fatalf("메시 범위 %.0f×%.0f, 기대 100×80", maxX, maxY)
	}
}

func TestDeformInterp(t *testing.T) {
	part := AutoMeshPart("body", "body.png", 100, 100, 2, 2, 0)
	n := len(part.Mesh.Verts)
	pup := &Puppet{
		Name:  "t",
		Parts: []Part{part},
		Params: []Parameter{{
			Name: "wind", Min: 0, Max: 1, Default: 0,
			Bindings: []Binding{{Part: "body", Keys: shiftKeys(n, 10, 0)}},
		}},
	}

	// value=0 → rest 그대로
	rest := pup.DeformPart(0, map[string]float64{"wind": 0})
	for i, v := range rest.Verts {
		if v.X != part.Mesh.Verts[i].X {
			t.Fatalf("value=0인데 정점 %d 이동: %.2f", i, v.X)
		}
	}
	// value=0.5 → +5
	half := pup.DeformPart(0, map[string]float64{"wind": 0.5})
	for i, v := range half.Verts {
		want := part.Mesh.Verts[i].X + 5
		if math.Abs(v.X-want) > 1e-9 {
			t.Fatalf("value=0.5 정점 %d X=%.3f, 기대 %.3f", i, v.X, want)
		}
	}
	// value=1 → +10, 범위 초과(2.0)도 클램프되어 +10
	for _, val := range []float64{1.0, 2.0} {
		full := pup.DeformPart(0, map[string]float64{"wind": val})
		for i, v := range full.Verts {
			want := part.Mesh.Verts[i].X + 10
			if math.Abs(v.X-want) > 1e-9 {
				t.Fatalf("value=%.1f 정점 %d X=%.3f, 기대 %.3f", val, i, v.X, want)
			}
		}
	}
	// rest 메시는 변형으로 오염되지 않아야(복사본 반환)
	if part.Mesh.Verts[0].X != pup.Parts[0].Mesh.Verts[0].X {
		t.Fatal("rest 메시가 변형됨")
	}
}

// TestDeformMultiParamSum은 두 파라미터가 같은 파트를 변형할 때 오프셋이 합산되는지 검증합니다.
func TestDeformMultiParamSum(t *testing.T) {
	part := AutoMeshPart("body", "body.png", 100, 100, 1, 1, 0)
	n := len(part.Mesh.Verts)
	pup := &Puppet{
		Parts: []Part{part},
		Params: []Parameter{
			{Name: "x", Default: 0, Bindings: []Binding{{Part: "body", Keys: shiftKeys(n, 10, 0)}}},
			{Name: "y", Default: 0, Bindings: []Binding{{Part: "body", Keys: shiftKeys(n, 0, 20)}}},
		},
	}
	d := pup.DeformPart(0, map[string]float64{"x": 1, "y": 0.5})
	for i, v := range d.Verts {
		wantX := part.Mesh.Verts[i].X + 10 // x=1
		wantY := part.Mesh.Verts[i].Y + 10 // y=0.5 → +10
		if math.Abs(v.X-wantX) > 1e-9 || math.Abs(v.Y-wantY) > 1e-9 {
			t.Fatalf("정점 %d (%.2f,%.2f), 기대 (%.2f,%.2f)", i, v.X, v.Y, wantX, wantY)
		}
	}
}

// solidTex는 단색 불투명 텍스처를 만듭니다.
func solidTex(w, h int, r, g, b uint8) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(im.Pix); i += 4 {
		im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = r, g, b, 255
	}
	return im
}

// TestRenderDeformShift는 파라미터로 파트를 +X 이동시키면 렌더 결과의 무게중심이 오른쪽으로 가는지 검증합니다.
func TestRenderDeformShift(t *testing.T) {
	part := AutoMeshPart("body", "body.png", 40, 40, 3, 3, 0)
	n := len(part.Mesh.Verts)
	pup := &Puppet{
		Parts:  []Part{part},
		Params: []Parameter{{Name: "shift", Default: 0, Bindings: []Binding{{Part: "body", Keys: shiftKeys(n, 30, 0)}}}},
	}
	tex := map[string]*image.NRGBA{"body.png": solidTex(40, 40, 200, 80, 80)}

	cx := func(values map[string]float64) (float64, int) {
		dst := image.NewNRGBA(image.Rect(0, 0, 120, 80))
		pup.Render(dst, tex, values)
		var sx float64
		var c int
		for y := 0; y < 80; y++ {
			for x := 0; x < 120; x++ {
				if dst.Pix[dst.PixOffset(x, y)+3] > 0 {
					sx += float64(x)
					c++
				}
			}
		}
		if c == 0 {
			return 0, 0
		}
		return sx / float64(c), c
	}

	cx0, n0 := cx(map[string]float64{"shift": 0})
	cx1, n1 := cx(map[string]float64{"shift": 1})
	if n0 == 0 || n1 == 0 {
		t.Fatalf("렌더 결과 비어있음: n0=%d n1=%d", n0, n1)
	}
	if cx1 <= cx0+10 {
		t.Fatalf("파라미터 이동이 렌더에 반영 안됨: cx0=%.1f cx1=%.1f", cx0, cx1)
	}
	// 면적은 보존되어야(이동만 했으므로)
	if math.Abs(float64(n1-n0)) > float64(n0)*0.15 {
		t.Fatalf("면적 급변(이동인데 변형 의심): n0=%d n1=%d", n0, n1)
	}
}

func TestJSONRoundtrip(t *testing.T) {
	part := AutoMeshPart("body", "body.png", 64, 64, 2, 2, 1.5)
	n := len(part.Mesh.Verts)
	orig := &Puppet{
		Name:  "hero",
		Parts: []Part{part},
		Params: []Parameter{{
			Name: "angleX", Min: -30, Max: 30, Default: 0,
			Bindings: []Binding{{Part: "body", Keys: shiftKeys(n, 5, -3)}},
		}},
	}
	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var back Puppet
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Name != "hero" || len(back.Parts) != 1 || len(back.Params) != 1 {
		t.Fatalf("구조 손실: %+v", back)
	}
	if back.Parts[0].Mesh == nil || len(back.Parts[0].Mesh.Verts) != n {
		t.Fatalf("메시 직렬화 손실")
	}
	// 역직렬화한 퍼펫도 동일하게 변형되어야
	d := back.DeformPart(0, map[string]float64{"angleX": 30})
	for i, v := range d.Verts {
		wantX := back.Parts[0].Mesh.Verts[i].X + 5
		if math.Abs(v.X-wantX) > 1e-9 {
			t.Fatalf("역직렬화 후 변형 불일치 정점 %d", i)
		}
	}
	_ = rig.Vertex{}
}
