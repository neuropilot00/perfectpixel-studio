package rig

import "image"

// 표준 휴머노이드(측면) 본 이름 — 사이드스크롤러 기준.
const (
	BHip      = "hip"
	BTorso    = "torso"
	BHead     = "head"
	BArmUpB   = "armUpperBack"
	BArmLoB   = "armLowerBack"
	BArmUpF   = "armUpperFront"
	BArmLoF   = "armLowerFront"
	BThighB   = "thighBack"
	BShinB    = "shinBack"
	BThighF   = "thighFront"
	BShinF    = "shinFront"
)

// HumanoidTemplate은 부위 이미지 없는 표준 측면 휴머노이드 스켈레톤(본 계층/오프셋/z순서)을 만듭니다.
// 오프셋은 캔버스 픽셀 단위(엉덩이 기준)이며, AttachPart로 부위 이미지를 붙여 사용합니다.
// 좌표계: x 오른쪽, y 아래. 팔다리는 rest에서 아래로(+y) 뻗음(RestAngle=0 → OffY>0).
func HumanoidTemplate() *Skeleton {
	// 인덱스 순서 = Parent < i 보장
	bones := []Bone{
		{Name: BHip, Parent: -1, OffX: 0, OffY: 0, Z: 5},
		{Name: BTorso, Parent: 0, OffX: 0, OffY: -34, Z: 5},   // 엉덩이 위로
		{Name: BHead, Parent: 1, OffX: 0, OffY: -30, Z: 6},    // 몸통 위로
		{Name: BArmUpB, Parent: 1, OffX: -2, OffY: -26, Z: 1}, // 뒤쪽 팔(뒤)
		{Name: BArmLoB, Parent: 3, OffX: 0, OffY: 20, Z: 1},
		{Name: BArmUpF, Parent: 1, OffX: 2, OffY: -26, Z: 9}, // 앞쪽 팔(앞)
		{Name: BArmLoF, Parent: 5, OffX: 0, OffY: 20, Z: 9},
		{Name: BThighB, Parent: 0, OffX: -4, OffY: 0, Z: 2}, // 뒤쪽 다리
		{Name: BShinB, Parent: 7, OffX: 0, OffY: 22, Z: 2},
		{Name: BThighF, Parent: 0, OffX: 4, OffY: 0, Z: 8}, // 앞쪽 다리
		{Name: BShinF, Parent: 9, OffX: 0, OffY: 22, Z: 8},
	}
	return &Skeleton{Bones: bones, OriginX: 0, OriginY: 0}
}

// AttachPart는 이름으로 본을 찾아 부위 이미지와 피벗을 설정합니다.
func (sk *Skeleton) AttachPart(name string, part *image.NRGBA, pivotX, pivotY float64) bool {
	for i := range sk.Bones {
		if sk.Bones[i].Name == name {
			sk.Bones[i].Part = part
			sk.Bones[i].PivotX = pivotX
			sk.Bones[i].PivotY = pivotY
			return true
		}
	}
	return false
}

// MotionLibrary는 표준 휴머노이드용 기본 애니메이션(본 각도 키프레임)을 반환합니다.
// 각도는 rest 대비 라디안. 양수 = 시계방향(y-down).
func MotionLibrary() map[string]*Animation {
	const d = 0.0174532925 // deg→rad
	walk := &Animation{Name: "walk", FPS: 12, Loop: true, Keys: []Keyframe{
		{T: 0.0, Pose: legSwing(28*d, -22*d, 18*d)},
		{T: 0.25, Pose: legSwing(0, 0, 0)},
		{T: 0.5, Pose: legSwing(-22*d, 28*d, -18*d)},
		{T: 0.75, Pose: legSwing(0, 0, 0)},
		{T: 1.0, Pose: legSwing(28*d, -22*d, 18*d)},
	}}
	run := &Animation{Name: "run", FPS: 14, Loop: true, Keys: []Keyframe{
		{T: 0.0, Pose: legSwing(45*d, -38*d, 32*d)},
		{T: 0.25, Pose: tuck(20*d)},
		{T: 0.5, Pose: legSwing(-38*d, 45*d, -32*d)},
		{T: 0.75, Pose: tuck(20*d)},
		{T: 1.0, Pose: legSwing(45*d, -38*d, 32*d)},
	}}
	idle := &Animation{Name: "idle", FPS: 6, Loop: true, Keys: []Keyframe{
		{T: 0.0, Pose: Pose{BTorso: 0}},
		{T: 0.5, Pose: Pose{BTorso: 2 * d, BHead: 1 * d}},
		{T: 1.0, Pose: Pose{BTorso: 0}},
	}}
	return map[string]*Animation{"walk": walk, "run": run, "idle": idle}
}

// legSwing은 앞/뒤 허벅지 스윙 + 앞팔 반대 스윙의 포즈를 만듭니다.
func legSwing(thighF, thighB, armF float64) Pose {
	return Pose{
		BThighF: thighF, BThighB: thighB,
		BShinF: thighF * 0.4, BShinB: -thighB * 0.5,
		BArmUpF: armF, BArmUpB: -armF,
		BArmLoF: armF * 0.5, BArmLoB: -armF * 0.5,
	}
}

// tuck은 달리기 통과 자세(양 무릎 살짝 굽힘)입니다.
func tuck(k float64) Pose {
	return Pose{BShinF: k, BShinB: k, BThighF: 0, BThighB: 0}
}
