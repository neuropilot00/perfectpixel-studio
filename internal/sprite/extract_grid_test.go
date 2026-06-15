package sprite

import (
	"image"
	"testing"
)

func TestGridDims(t *testing.T) {
	cases := []struct{ n, cols, rows int }{
		{1, 1, 1}, {2, 2, 1}, {3, 2, 2}, {4, 2, 2},
		{5, 3, 2}, {6, 3, 2}, {8, 3, 3}, {9, 3, 3}, {16, 4, 4},
	}
	for _, c := range cases {
		cols, rows := GridDims(c.n)
		if cols != c.cols || rows != c.rows {
			t.Errorf("GridDims(%d) = %d×%d, 기대 %d×%d", c.n, cols, rows, c.cols, c.rows)
		}
		if cols*rows < c.n {
			t.Errorf("GridDims(%d): %d×%d=%d < %d (셀 부족)", c.n, cols, rows, cols*rows, c.n)
		}
	}
}

// TestExtractFramesGrid는 6개 포즈를 3×2 그리드에 배치한 시트에서
// 행우선(좌→우, 위→아래) 순서로 6프레임이 정확히 추출되는지 검증합니다.
func TestExtractFramesGrid(t *testing.T) {
	// 600×400 캔버스 = 3열 × 2행, 셀당 200×200
	sheet := image.NewNRGBA(image.Rect(0, 0, 600, 400))
	cols, rows := 3, 2
	cw, ch := 200, 200
	// 각 셀 중앙에 60×80 본체. 셀 인덱스 i를 식별하려 폭을 (i+1)*8로 살짝 다르게.
	for i := 0; i < 6; i++ {
		c := i % cols
		r := i / cols
		cx := c*cw + cw/2
		cy := r*ch + ch/2
		halfW := 30 + i*4 // 셀마다 폭이 다름
		fillBox(sheet, cx-halfW, cy-40, cx+halfW, cy+40, 200, 100, 50)
	}

	res := ExtractFramesGrid(sheet, 6, cols, rows, 256, 256, 16)
	if res.Found != 6 {
		t.Fatalf("프레임 수 오류: %d (%v)", res.Found, res.Warnings)
	}
	if len(res.Frames) != 6 {
		t.Fatalf("프레임 배열 길이 %d, 기대 6", len(res.Frames))
	}

	// 행우선 순서 검증: 각 프레임의 불투명 폭이 i에 따라 단조 증가해야 함
	// (셀 폭을 i에 비례시켰으므로). 이는 좌→우, 위→아래 순서가 지켜졌다는 증거.
	prevW := 0
	for i, f := range res.Frames {
		minX, maxX := f.Rect.Dx(), -1
		for y := 0; y < f.Rect.Dy(); y++ {
			for x := 0; x < f.Rect.Dx(); x++ {
				if f.Pix[f.PixOffset(x, y)+3] > alphaThreshold {
					if x < minX {
						minX = x
					}
					if x > maxX {
						maxX = x
					}
				}
			}
		}
		w := maxX - minX + 1
		if maxX < 0 {
			t.Fatalf("프레임 %d 비어 있음", i)
		}
		if i > 0 && w <= prevW {
			t.Fatalf("행우선 순서 깨짐: 프레임 %d 폭 %d <= 이전 %d", i, w, prevW)
		}
		prevW = w
	}
}

// TestExtractFramesGridNoFootClip은 셀 하단 가까이 닿는 포즈가
// 그리드 셀 경계로 잘리지 않고 온전히 추출되는지 검증합니다(발 잘림 방지).
func TestExtractFramesGridNoFootClip(t *testing.T) {
	// 2×1 그리드, 셀 200×200
	sheet := image.NewNRGBA(image.Rect(0, 0, 400, 200))
	// 셀0: 머리(상단)~발(하단 근처)까지 세로로 긴 본체. 발이 y=185까지 내려옴.
	fillBox(sheet, 80, 20, 120, 185, 50, 50, 200)
	// 셀1: 동일
	fillBox(sheet, 280, 20, 320, 185, 50, 50, 200)

	res := ExtractFramesGrid(sheet, 2, 2, 1, 256, 256, 16)
	if res.Found != 2 {
		t.Fatalf("프레임 수 오류: %d", res.Found)
	}
	// 원본 본체 높이 = 185-20+1 = 166px. 추출/배치 후에도 종횡비가 보존되어야 함
	// (잘렸다면 높이가 크게 줄어듦). 각 프레임의 불투명 bbox 높이가 폭보다 충분히 커야 함.
	for i, f := range res.Frames {
		minX, minY, maxX, maxY := f.Rect.Dx(), f.Rect.Dy(), -1, -1
		for y := 0; y < f.Rect.Dy(); y++ {
			for x := 0; x < f.Rect.Dx(); x++ {
				if f.Pix[f.PixOffset(x, y)+3] > alphaThreshold {
					if x < minX {
						minX = x
					}
					if x > maxX {
						maxX = x
					}
					if y < minY {
						minY = y
					}
					if y > maxY {
						maxY = y
					}
				}
			}
		}
		bw, bh := maxX-minX+1, maxY-minY+1
		// 원본 종횡비 ≈ 166/41 ≈ 4.0. 발이 잘렸다면 높이가 급감해 비율이 무너짐.
		if float64(bh)/float64(bw) < 3.0 {
			t.Fatalf("프레임 %d 발 잘림 의심: bbox %d×%d (h/w=%.2f, 기대 ≥3.0)", i, bw, bh, float64(bh)/float64(bw))
		}
	}
}
