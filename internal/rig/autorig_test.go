package rig

import (
	"image"
	"testing"
)

func fillRect(im *image.NRGBA, x0, y0, x1, y1 int, r, g, b uint8) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			i := im.PixOffset(x, y)
			im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = r, g, b, 255
		}
	}
}

// 합성 휴머노이드 스프라이트(투명 배경 + 부위별 색).
func syntheticHumanoid() *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, 120, 200))
	fillRect(im, 50, 10, 70, 40, 210, 180, 150)  // 머리
	fillRect(im, 48, 40, 72, 112, 90, 90, 150)   // 몸통
	fillRect(im, 36, 46, 48, 104, 70, 70, 110)   // 뒤 팔(왼)
	fillRect(im, 72, 46, 84, 104, 120, 120, 170) // 앞 팔(오)
	fillRect(im, 50, 112, 60, 192, 60, 60, 90)   // 뒤 다리(왼)
	fillRect(im, 60, 112, 70, 192, 110, 110, 160) // 앞 다리(오)
	return im
}

func TestAutoRigPipeline(t *testing.T) {
	src := syntheticHumanoid()
	sk := AutoRigHumanoid(src)
	if len(sk.Bones) != 7 {
		t.Fatalf("본 7개 기대(hip/torso/head/armB/armF/thighB/thighF), %d개", len(sk.Bones))
	}
	parts := 0
	for _, b := range sk.Bones {
		if b.Part != nil {
			parts++
		}
	}
	if parts < 6 {
		t.Fatalf("부위 이미지 6개 이상 기대, %d개", parts)
	}

	// rest 렌더 비어있지 않아야
	rest := Render(sk, nil, 120, 200)
	_, _, n := centroid(rest)
	if n < 200 {
		t.Fatalf("rest 렌더가 비었거나 너무 작음: 불투명 %d px", n)
	}

	// 걷기 애니: 앞다리가 프레임마다 움직여야
	walk := MotionLibrary()["walk"]
	frames := RenderFrames(sk, walk, 8, 120, 220)
	frontLegX := func(im *image.NRGBA) (float64, int) {
		w, h := im.Rect.Dx(), im.Rect.Dy()
		var sx float64
		var c int
		for y := h * 3 / 5; y < h; y++ {
			for x := 0; x < w; x++ {
				i := im.PixOffset(x, y)
				if im.Pix[i+3] > 0 && im.Pix[i] > 95 && im.Pix[i+2] > 140 { // 앞다리(밝은) 색
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
	min, max := 1e9, -1e9
	for i, f := range frames {
		x, c := frontLegX(f)
		if c == 0 {
			t.Fatalf("프레임 %d 앞다리 없음", i)
		}
		if x < min {
			min = x
		}
		if x > max {
			max = x
		}
	}
	if max-min < 3 {
		t.Fatalf("자동리그 걷기에서 앞다리가 거의 안 움직임: 스윙 %.1fpx", max-min)
	}
	t.Logf("OK 자동리그 파이프라인: 본 7개, 부위 %d개, 걷기 앞다리 스윙 %.1fpx", parts, max-min)
}
