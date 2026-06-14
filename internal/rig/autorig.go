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
// 스켈레톤을 만듭니다. 사지를 2분절(상박+하박=팔꿈치, 허벅지+정강이=무릎)로 쪼개므로
// MotionLibrary의 walk/run/idle에서 무릎·팔꿈치가 굽어 자연스럽게 움직입니다.
// 직사각 슬라이스라 정밀하진 않지만 에디터 없이 생성→리그→애니가 바로 돌아갑니다.
func AutoRigHumanoid(src *image.NRGBA) *Skeleton {
	l, t, w, h := contentBBox(src)
	if w <= 4 || h <= 8 {
		return HumanoidTemplate()
	}
	fl, ft, fw, fh := float64(l), float64(t), float64(w), float64(h)
	cx := fl + fw/2

	// 비율 기반 관절(원본 좌표) — 사지를 팔꿈치/무릎에서 분절
	neckY := ft + 0.27*fh
	shoulderY := ft + 0.30*fh
	elbowY := ft + 0.45*fh
	wristY := ft + 0.62*fh
	hipY := ft + 0.54*fh
	kneeY := ft + 0.76*fh
	ankleY := ft + fh

	headJ := [2]float64{cx, ft + 0.26*fh}
	hipJ := [2]float64{cx, hipY}
	shB := [2]float64{cx - 0.12*fw, shoulderY}
	shF := [2]float64{cx + 0.12*fw, shoulderY}
	elB := [2]float64{cx - 0.13*fw, elbowY}
	elF := [2]float64{cx + 0.13*fw, elbowY}
	hipBk := [2]float64{cx - 0.07*fw, hipY}
	hipFr := [2]float64{cx + 0.07*fw, hipY}
	kneeB := [2]float64{cx - 0.07*fw, kneeY}
	kneeF := [2]float64{cx + 0.07*fw, kneeY}

	xi := func(f float64) int { return int(f + 0.5) }
	pad := 0.03 * fh // 관절 클리핑 방지용 여유

	sk := &Skeleton{OriginX: hipJ[0], OriginY: hipJ[1]}
	// region: 크롭 후 부위로 부착. ox/oy(크롭 원점)로 관절 피벗 좌표 변환.
	region := func(name string, parent int, joint, parentJoint [2]float64, x0, y0, x1, y1 float64, z int) {
		part := cropRegion(src, xi(x0), xi(y0), xi(x1), xi(y1))
		sk.Bones = append(sk.Bones, Bone{
			Name: name, Parent: parent,
			OffX: joint[0] - parentJoint[0], OffY: joint[1] - parentJoint[1],
			Part: part, PivotX: joint[0] - x0, PivotY: joint[1] - y0, Z: z,
		})
	}

	// 인덱스: 0 hip,1 torso,2 head,3 armUpB,4 armLoB,5 armUpF,6 armLoF,7 thighB,8 shinB,9 thighF,10 shinF
	sk.Bones = append(sk.Bones, Bone{Name: BHip, Parent: -1, Z: 4}) // 루트(부위 없음)
	region(BTorso, 0, hipJ, hipJ, cx-0.18*fw, neckY-0.02*fh, cx+0.18*fw, hipY+0.04*fh, 5)
	region(BHead, 1, headJ, hipJ, cx-0.22*fw, ft, cx+0.22*fw, neckY, 6)
	// 뒤 팔(상박/하박)
	region(BArmUpB, 1, shB, hipJ, cx-0.32*fw, shoulderY-pad, cx-0.04*fw, elbowY+pad, 1)
	region(BArmLoB, 3, elB, shB, cx-0.34*fw, elbowY-pad, cx-0.02*fw, wristY+pad, 1)
	// 앞 팔(상박/하박)
	region(BArmUpF, 1, shF, hipJ, cx+0.04*fw, shoulderY-pad, cx+0.32*fw, elbowY+pad, 9)
	region(BArmLoF, 5, elF, shF, cx+0.02*fw, elbowY-pad, cx+0.34*fw, wristY+pad, 9)
	// 뒤 다리(허벅지/정강이)
	region(BThighB, 0, hipBk, hipJ, cx-0.20*fw, hipY-pad, cx+0.02*fw, kneeY+pad, 2)
	region(BShinB, 7, kneeB, hipBk, cx-0.20*fw, kneeY-pad, cx+0.04*fw, ankleY, 2)
	// 앞 다리(허벅지/정강이)
	region(BThighF, 0, hipFr, hipJ, cx-0.02*fw, hipY-pad, cx+0.20*fw, kneeY+pad, 8)
	region(BShinF, 9, kneeF, hipFr, cx-0.04*fw, kneeY-pad, cx+0.20*fw, ankleY, 8)
	return sk
}
