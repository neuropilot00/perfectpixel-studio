package rig

import (
	"image"
	"math"
	"testing"
)

// solidPart는 색이 채워진 불투명 부위 이미지를 만듭니다.
func solidPart(w, h int) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(im.Pix); i += 4 {
		im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = 200, 100, 50, 255
	}
	return im
}

// centroid는 불투명 픽셀의 무게중심과 개수를 반환합니다.
func centroid(im *image.NRGBA) (cx, cy float64, n int) {
	w, h := im.Rect.Dx(), im.Rect.Dy()
	var sx, sy float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if im.Pix[im.PixOffset(x, y)+3] > 0 {
				sx += float64(x)
				sy += float64(y)
				n++
			}
		}
	}
	if n > 0 {
		cx, cy = sx/float64(n), sy/float64(n)
	}
	return
}

// "팔" 본 하나: 길쭉한 막대, 어깨(상단)를 피벗으로 회전.
func armSkeleton() *Skeleton {
	bar := solidPart(6, 40) // 세로 막대
	return &Skeleton{
		OriginX: 100, OriginY: 100,
		Bones: []Bone{
			{Name: "arm", Parent: -1, Part: bar, PivotX: 3, PivotY: 2, Z: 0}, // 피벗=상단 중앙
		},
	}
}

func TestRenderRotatesPart(t *testing.T) {
	sk := armSkeleton()
	// 각도 0: 막대가 피벗(100,100) 아래로 뻗음 → 무게중심이 피벗보다 아래(y 큼)
	f0 := Render(sk, Pose{"arm": 0}, 200, 200)
	cx0, cy0, n0 := centroid(f0)
	if n0 == 0 {
		t.Fatal("각도0 렌더가 비어있음")
	}
	if cy0 <= 100 {
		t.Fatalf("각도0: 막대가 피벗 아래로 가야 함, cy=%.1f", cy0)
	}
	// 각도 +90°: 막대가 오른쪽으로 뻗음 → 무게중심 x가 피벗(100)보다 오른쪽
	f90 := Render(sk, Pose{"arm": math.Pi / 2}, 200, 200)
	cx90, cy90, n90 := centroid(f90)
	if n90 == 0 {
		t.Fatal("각도90 렌더가 비어있음")
	}
	// 90° 회전: 막대가 수평이 됨 → 무게중심이 피벗(100)에서 수평으로 크게 이동, 수직은 피벗 근처
	if math.Abs(cx90-100) < 15 {
		t.Fatalf("각도90: 막대가 수평으로 이동해야 함, cx=%.1f", cx90)
	}
	if math.Abs(cy90-100) > 20 {
		t.Fatalf("각도90: 막대가 수평이면 무게중심 y가 피벗 근처여야 함, cy=%.1f", cy90)
	}
	// 픽셀 수는 회전해도 대략 보존
	if math.Abs(float64(n90-n0))/float64(n0) > 0.25 {
		t.Fatalf("회전 후 픽셀 수가 너무 달라짐: n0=%d n90=%d", n0, n90)
	}
	t.Logf("OK 회전 동작: 각도0 무게중심=(%.0f,%.0f), 각도90=(%.0f,%.0f)", cx0, cy0, cx90, cy90)
}

func TestRenderFramesAnim(t *testing.T) {
	sk := armSkeleton()
	an := &Animation{Name: "swing", FPS: 12, Loop: true, Keys: []Keyframe{
		{T: 0, Pose: Pose{"arm": -0.6}},
		{T: 0.5, Pose: Pose{"arm": 0.6}},
		{T: 1, Pose: Pose{"arm": -0.6}},
	}}
	frames := RenderFrames(sk, an, 8, 200, 200)
	if len(frames) != 8 {
		t.Fatalf("프레임 8개 기대, %d개", len(frames))
	}
	var prevCx float64
	moved := false
	for i, f := range frames {
		cx, _, n := centroid(f)
		if n == 0 {
			t.Fatalf("프레임 %d 비어있음", i)
		}
		if i > 0 && math.Abs(cx-prevCx) > 0.5 {
			moved = true
		}
		prevCx = cx
	}
	if !moved {
		t.Fatal("프레임 간 팔이 움직이지 않음(보간 실패)")
	}
	t.Log("OK 8프레임 스윙 애니메이션 렌더 + 프레임 간 움직임 확인")
}
