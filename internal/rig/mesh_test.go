package rig

import (
	"image"
	"testing"
)

// quadTex는 4분면 색이 다른 텍스처를 만듭니다(샘플링 검증용).
func quadTex(w, h int) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := im.PixOffset(x, y)
			var r, g, b uint8
			switch {
			case x < w/2 && y < h/2:
				r, g, b = 220, 40, 40 // 좌상 빨강
			case x >= w/2 && y < h/2:
				r, g, b = 40, 200, 40 // 우상 초록
			case x < w/2 && y >= h/2:
				r, g, b = 40, 40, 220 // 좌하 파랑
			default:
				r, g, b = 220, 200, 40 // 우하 노랑
			}
			im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = r, g, b, 255
		}
	}
	return im
}

func TestWarpRenderFollowsMesh(t *testing.T) {
	tex := quadTex(64, 64)
	m := GridMesh(64, 64, 4, 4) // 5x5 정점

	// rest 렌더: 텍스처가 그대로 (좌상=빨강, 우상=초록)
	dst := image.NewNRGBA(image.Rect(0, 0, 160, 160))
	WarpRender(dst, tex, m)
	red := dst.Pix[dst.PixOffset(10, 10):]
	if !(red[0] > 180 && red[1] < 90 && red[3] == 255) {
		t.Fatalf("rest 좌상 빨강 기대, got rgba=%d,%d,%d,%d", red[0], red[1], red[2], red[3])
	}

	// 변형: 상단 정점들을 오른쪽으로 전단(shear) → 텍스처가 기울어 따라가야 함
	for i := range m.Verts {
		v := &m.Verts[i]
		shear := (1 - v.Y/64) * 40 // 위로 갈수록 +40px
		v.X += shear
	}
	dst2 := image.NewNRGBA(image.Rect(0, 0, 160, 160))
	WarpRender(dst2, tex, m)

	// 변형 후 불투명 픽셀 수가 rest와 비슷해야(텍스처 면적 보존, 큰 구멍 없음)
	count := func(im *image.NRGBA) int {
		n := 0
		for p := 3; p < len(im.Pix); p += 4 {
			if im.Pix[p] > 0 {
				n++
			}
		}
		return n
	}
	n1, n2 := count(dst), count(dst2)
	if n2 < n1*85/100 {
		t.Fatalf("변형 후 면적 급감(구멍 의심): rest=%d warped=%d", n1, n2)
	}
	// 전단으로 상단이 오른쪽으로 이동 → 상단 행의 불투명 무게중심 X가 rest보다 커야 함
	topCx := func(im *image.NRGBA) float64 {
		var sx float64
		var c int
		for y := 0; y < 20; y++ {
			for x := 0; x < im.Rect.Dx(); x++ {
				if im.Pix[im.PixOffset(x, y)+3] > 0 {
					sx += float64(x)
					c++
				}
			}
		}
		if c == 0 {
			return 0
		}
		return sx / float64(c)
	}
	if topCx(dst2) <= topCx(dst)+5 {
		t.Fatalf("전단 변형이 텍스처를 안 옮김: rest topCx=%.1f warped=%.1f", topCx(dst), topCx(dst2))
	}
	t.Logf("OK 메시 워프: 면적보존 %d→%d, 상단 전단 이동 %.1f→%.1f", n1, n2, topCx(dst), topCx(dst2))
}
