package rig

import "image"

// contentBBox는 불투명(알파>10) 픽셀의 경계상자를 반환합니다.
func contentBBox(src *image.NRGBA) (l, t, w, h int) {
	W, H := src.Rect.Dx(), src.Rect.Dy()
	minX, minY, maxX, maxY := W, H, -1, -1
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			if src.Pix[src.PixOffset(x, y)+3] > 10 {
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
	if maxX < minX {
		return 0, 0, 0, 0
	}
	return minX, minY, maxX - minX + 1, maxY - minY + 1
}

// cropRegion은 src의 [x0,y0,x1,y1)(클램프) 영역을 잘라 NRGBA로 반환합니다.
func cropRegion(src *image.NRGBA, x0, y0, x1, y1 int) *image.NRGBA {
	W, H := src.Rect.Dx(), src.Rect.Dy()
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > W {
		x1 = W
	}
	if y1 > H {
		y1 = H
	}
	if x1 <= x0 || y1 <= y0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	out := image.NewNRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			si := src.PixOffset(x, y)
			di := out.PixOffset(x-x0, y-y0)
			copy(out.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return out
}

// AutoRigHumanoid는 대략 직립한 휴머노이드/캐릭터 스프라이트를 표준 비율로 자동 분할해
// 스켈레톤(머리/몸통/앞뒤 팔/앞뒤 다리)을 만듭니다. MotionLibrary의 walk/run/idle과 호환.
// 직사각 영역 슬라이스라 정밀하진 않지만, 에디터 없이 생성→리그→애니가 바로 돌아갑니다.
// 더 정밀하게는 수동 리그 에디터(다음 단계)로 관절을 직접 잡습니다.
func AutoRigHumanoid(src *image.NRGBA) *Skeleton {
	l, t, w, h := contentBBox(src)
	if w <= 4 || h <= 8 {
		return HumanoidTemplate()
	}
	fl, ft, fw, fh := float64(l), float64(t), float64(w), float64(h)
	cx := fl + fw/2

	// 비율 기반 관절(원본 좌표)
	neckY := ft + 0.27*fh
	shoulderY := ft + 0.30*fh
	hipY := ft + 0.54*fh
	headJ := [2]float64{cx, ft + 0.26*fh}
	hipJ := [2]float64{cx, hipY}
	shB := [2]float64{cx - 0.12*fw, shoulderY}
	shF := [2]float64{cx + 0.12*fw, shoulderY}
	hipBk := [2]float64{cx - 0.07*fw, hipY}
	hipFr := [2]float64{cx + 0.07*fw, hipY}

	xi := func(f float64) int { return int(f + 0.5) }
	// 영역 크롭 (겹침 허용)
	headP := cropRegion(src, xi(cx-0.22*fw), l, xi(cx+0.22*fw), xi(neckY))
	torsoP := cropRegion(src, xi(cx-0.18*fw), xi(neckY-0.02*fh), xi(cx+0.18*fw), xi(hipY+0.04*fh))
	armBP := cropRegion(src, xi(cx-0.36*fw), xi(shoulderY), xi(cx-0.08*fw), xi(ft+0.72*fh))
	armFP := cropRegion(src, xi(cx+0.08*fw), xi(shoulderY), xi(cx+0.36*fw), xi(ft+0.72*fh))
	legBP := cropRegion(src, xi(cx-0.22*fw), xi(hipY), xi(cx+0.02*fw), xi(ft+fh))
	legFP := cropRegion(src, xi(cx-0.02*fw), xi(hipY), xi(cx+0.22*fw), xi(ft+fh))

	// 크롭 원점(원본좌표) — 피벗 계산용
	headOX, headOY := cx-0.22*fw, ft
	torsoOX, torsoOY := cx-0.18*fw, neckY-0.02*fh
	armBOX, armBOY := cx-0.36*fw, shoulderY
	armFOX, armFOY := cx+0.08*fw, shoulderY
	legBOX, legBOY := cx-0.22*fw, hipY
	legFOX, legFOY := cx-0.02*fw, hipY

	piv := func(j [2]float64, ox, oy float64) (float64, float64) { return j[0] - ox, j[1] - oy }

	sk := &Skeleton{OriginX: hipJ[0], OriginY: hipJ[1]}
	add := func(name string, parent int, joint [2]float64, parentJoint [2]float64, part *image.NRGBA, ox, oy float64, z int) {
		px, py := piv(joint, ox, oy)
		sk.Bones = append(sk.Bones, Bone{
			Name: name, Parent: parent,
			OffX: joint[0] - parentJoint[0], OffY: joint[1] - parentJoint[1],
			Part: part, PivotX: px, PivotY: py, Z: z,
		})
	}
	// 인덱스: 0 hip, 1 torso, 2 head, 3 armB, 4 armF, 5 thighB, 6 thighF
	add(BHip, -1, hipJ, hipJ, nil, 0, 0, 5)                       // 루트(부위 없음, 몸통이 가림)
	add(BTorso, 0, hipJ, hipJ, torsoP, torsoOX, torsoOY, 5)       // 몸통: 엉덩이서 위로
	add(BHead, 1, headJ, hipJ, headP, headOX, headOY, 6)          // 머리
	add(BArmUpB, 1, shB, hipJ, armBP, armBOX, armBOY, 1)          // 뒤 팔
	add(BArmUpF, 1, shF, hipJ, armFP, armFOX, armFOY, 9)          // 앞 팔
	add(BThighB, 0, hipBk, hipJ, legBP, legBOX, legBOY, 2)        // 뒤 다리
	add(BThighF, 0, hipFr, hipJ, legFP, legFOX, legFOY, 8)        // 앞 다리
	return sk
}
