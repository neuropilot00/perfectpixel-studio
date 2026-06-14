package rig

import (
	"image"
	"testing"
)

func box(w, h int, r, g, b uint8) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(im.Pix); i += 4 {
		im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = r, g, b, 255
	}
	return im
}

// 박스 부위로 휴머노이드를 조립합니다(엔진/모션 검증용 더미 캐릭터).
func dummyHumanoid() *Skeleton {
	sk := HumanoidTemplate()
	sk.OriginX, sk.OriginY = 100, 150 // 엉덩이 위치
	sk.AttachPart(BHip, box(20, 14, 80, 80, 120), 10, 7)
	sk.AttachPart(BTorso, box(22, 36, 90, 90, 140), 11, 34)
	sk.AttachPart(BHead, box(20, 20, 200, 170, 140), 10, 20)
	sk.AttachPart(BArmUpB, box(7, 22, 70, 70, 110), 3, 2)
	sk.AttachPart(BArmLoB, box(6, 20, 70, 70, 110), 3, 2)
	sk.AttachPart(BArmUpF, box(7, 22, 110, 110, 160), 3, 2)
	sk.AttachPart(BArmLoF, box(6, 20, 110, 110, 160), 3, 2)
	sk.AttachPart(BThighB, box(9, 24, 60, 60, 90), 4, 2)
	sk.AttachPart(BShinB, box(8, 24, 60, 60, 90), 4, 2)
	sk.AttachPart(BThighF, box(9, 24, 100, 100, 150), 4, 2)
	sk.AttachPart(BShinF, box(8, 24, 100, 100, 150), 4, 2)
	return sk
}

func TestHumanoidWalkAlternatesLegs(t *testing.T) {
	sk := dummyHumanoid()
	walk := MotionLibrary()["walk"]
	if walk == nil {
		t.Fatal("walk 애니 없음")
	}
	frames := RenderFrames(sk, walk, 8, 220, 240)
	if len(frames) != 8 {
		t.Fatalf("8프레임 기대, %d", len(frames))
	}

	// 앞다리 끝(발) 위치 추적: 프레임 하단부 앞쪽(밝은 색) 픽셀의 x 무게중심이 프레임마다 달라야 함
	footX := func(im *image.NRGBA) float64 {
		w, h := im.Rect.Dx(), im.Rect.Dy()
		var sx float64
		var n int
		for y := h * 3 / 5; y < h; y++ { // 하반신 영역
			for x := 0; x < w; x++ {
				i := im.PixOffset(x, y)
				if im.Pix[i+3] > 0 && im.Pix[i] > 95 && im.Pix[i+2] > 140 { // 앞쪽(밝은) 다리색
					sx += float64(x)
					n++
				}
			}
		}
		if n == 0 {
			return -1
		}
		return sx / float64(n)
	}

	var xs []float64
	for i, f := range frames {
		fx := footX(f)
		if fx < 0 {
			t.Fatalf("프레임 %d 앞다리 픽셀 없음", i)
		}
		xs = append(xs, fx)
	}
	// 앞다리가 앞→뒤로 스윙: 최대-최소 차이가 의미있게 커야 함
	min, max := xs[0], xs[0]
	for _, v := range xs {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	if max-min < 4 {
		t.Fatalf("걷기 사이클에서 앞다리가 거의 안 움직임: 스윙폭=%.1fpx", max-min)
	}
	t.Logf("OK 걷기 사이클: 앞다리 스윙폭=%.1fpx (8프레임)", max-min)
}
