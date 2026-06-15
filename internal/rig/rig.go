// Package rig는 2D 스켈레탈(본) 애니메이션 엔진입니다.
// 부위 이미지를 본 계층에 붙이고, 키프레임 포즈를 FK로 보간해 프레임을 렌더링합니다.
// (PixelLab/Spine식 정통 방식 — 단일-스트립 생성 대비 프레임 일관성·무한 프레임·누출 없음)
package rig

import (
	"image"
	"math"
	"sort"
)

// Bone은 스켈레톤의 한 마디입니다.
// 부위 이미지는 Part에 담고, Pivot은 부위 이미지 안에서 이 본이 회전하는 관절점(픽셀)입니다.
type Bone struct {
	Name   string `json:"name"`
	Parent int    `json:"parent"` // Skeleton.Bones 인덱스, 루트는 -1
	// 부모 관절 → 이 본 관절까지의 rest 오프셋(부모 로컬 프레임 기준, 픽셀).
	OffX float64 `json:"offX"`
	OffY float64 `json:"offY"`
	// rest 각도(라디안). 부모 누적각에 더해진다.
	RestAngle float64      `json:"restAngle"`
	Part      *image.NRGBA `json:"-"` // 이 본에 붙는 부위 이미지(nil 가능)
	// 부위 이미지 안에서의 관절점(픽셀). 이 점이 월드 관절 위치에 놓이고 그 주위로 회전.
	PivotX float64 `json:"pivotX"`
	PivotY float64 `json:"pivotY"`
	Z      int     `json:"z"` // 그리기 순서(작을수록 뒤)
}

// Skeleton은 본 배열(계층). Bones[i].Parent < i 를 권장(위상정렬된 입력).
type Skeleton struct {
	Bones   []Bone  `json:"bones"`
	OriginX float64 `json:"originX"` // 루트 관절의 월드 기준점(보통 발/엉덩이)
	OriginY float64 `json:"originY"`
}

// Pose는 본 이름 → rest 대비 각도 변화(라디안). 없으면 0.
type Pose map[string]float64

// Keyframe은 T∈[0,1] 시점의 포즈입니다.
type Keyframe struct {
	T    float64 `json:"t"`
	Pose Pose    `json:"pose"`
}

// Animation은 키프레임 시퀀스입니다.
type Animation struct {
	Name string     `json:"name"`
	FPS  int        `json:"fps"`
	Loop bool       `json:"loop"`
	Keys []Keyframe `json:"keys"`
}

// worldBone은 FK 계산 결과(월드 관절 위치 + 누적 각도)입니다.
type worldBone struct {
	x, y, ang float64
}

// computeWorld는 포즈를 적용한 각 본의 월드 관절 위치와 누적 각도를 구합니다(FK).
func computeWorld(sk *Skeleton, pose Pose) []worldBone {
	wb := make([]worldBone, len(sk.Bones))
	for i := range sk.Bones {
		b := &sk.Bones[i]
		delta := 0.0
		if pose != nil {
			delta = pose[b.Name]
		}
		if b.Parent < 0 || b.Parent >= i {
			// 루트(또는 비정상 부모): 원점에서 시작
			ang := b.RestAngle + delta
			wb[i] = worldBone{sk.OriginX + b.OffX, sk.OriginY + b.OffY, ang}
			continue
		}
		p := wb[b.Parent]
		ca, sa := math.Cos(p.ang), math.Sin(p.ang)
		// 부모 로컬 오프셋을 부모 누적각으로 회전해 월드 관절 위치 산출
		x := p.x + b.OffX*ca - b.OffY*sa
		y := p.y + b.OffX*sa + b.OffY*ca
		wb[i] = worldBone{x, y, p.ang + b.RestAngle + delta}
	}
	return wb
}

// Render는 포즈를 적용한 한 프레임을 (w×h) 이미지로 렌더링합니다.
func Render(sk *Skeleton, pose Pose, w, h int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	wb := computeWorld(sk, pose)
	order := make([]int, len(sk.Bones))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return sk.Bones[order[a]].Z < sk.Bones[order[b]].Z })
	for _, i := range order {
		b := &sk.Bones[i]
		if b.Part == nil {
			continue
		}
		blitRotated(dst, b.Part, b.PivotX, b.PivotY, wb[i].x, wb[i].y, wb[i].ang)
	}
	return dst
}

// blitRotated는 part를 (anchorX,anchorY) 기준으로 angle 회전해 dst의 (wx,wy)에 합성합니다.
// 픽셀아트 보존을 위해 역매핑 nearest-neighbor(앤티앨리어싱 없음)를 씁니다.
func blitRotated(dst, part *image.NRGBA, anchorX, anchorY, wx, wy, angle float64) {
	pw, ph := part.Rect.Dx(), part.Rect.Dy()
	ca, sa := math.Cos(angle), math.Sin(angle)
	// 회전된 part의 dst상 bbox 계산
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, c := range [][2]float64{{0, 0}, {float64(pw), 0}, {0, float64(ph)}, {float64(pw), float64(ph)}} {
		lx, ly := c[0]-anchorX, c[1]-anchorY
		dx := wx + lx*ca - ly*sa
		dy := wy + lx*sa + ly*ca
		minX, minY = math.Min(minX, dx), math.Min(minY, dy)
		maxX, maxY = math.Max(maxX, dx), math.Max(maxY, dy)
	}
	x0, y0 := int(math.Floor(minX)), int(math.Floor(minY))
	x1, y1 := int(math.Ceil(maxX)), int(math.Ceil(maxY))
	dw, dh := dst.Rect.Dx(), dst.Rect.Dy()
	for dy := y0; dy <= y1; dy++ {
		if dy < 0 || dy >= dh {
			continue
		}
		for dx := x0; dx <= x1; dx++ {
			if dx < 0 || dx >= dw {
				continue
			}
			// 역회전: dst → part 로컬
			rx, ry := float64(dx)-wx, float64(dy)-wy
			lx := rx*ca + ry*sa
			ly := -rx*sa + ry*ca
			sx := int(math.Round(lx + anchorX))
			sy := int(math.Round(ly + anchorY))
			if sx < 0 || sx >= pw || sy < 0 || sy >= ph {
				continue
			}
			si := part.PixOffset(sx, sy)
			if part.Pix[si+3] == 0 {
				continue
			}
			di := dst.PixOffset(dx, dy)
			// 알파 오버 합성(part가 위)
			sa8 := part.Pix[si+3]
			if sa8 == 255 {
				copy(dst.Pix[di:di+4], part.Pix[si:si+4])
				continue
			}
			af := float64(sa8) / 255
			for k := 0; k < 3; k++ {
				dst.Pix[di+k] = uint8(float64(part.Pix[si+k])*af + float64(dst.Pix[di+k])*(1-af))
			}
			da := float64(dst.Pix[di+3])
			dst.Pix[di+3] = uint8(float64(sa8) + da*(1-af))
		}
	}
}

// LerpPose는 두 포즈를 t∈[0,1]로 선형 보간합니다(없는 본은 0 기준).
func LerpPose(a, b Pose, t float64) Pose {
	out := Pose{}
	for k, v := range a {
		out[k] = v
	}
	for k, vb := range b {
		va := a[k]
		out[k] = va + (vb-va)*t
	}
	// b에만 있는 키도 위에서 처리됨. a에만 있는 키는 a→0? 아니라 a값 유지가 자연스럽다.
	for k := range a {
		if _, ok := b[k]; !ok {
			out[k] = a[k] * (1 - t)
		}
	}
	return out
}

// SamplePose는 애니메이션을 phase∈[0,1]에서 보간한 포즈를 반환합니다.
func (an *Animation) SamplePose(phase float64) Pose {
	if len(an.Keys) == 0 {
		return Pose{}
	}
	if len(an.Keys) == 1 {
		return an.Keys[0].Pose
	}
	if phase < 0 {
		phase = 0
	}
	if phase > 1 {
		phase = 1
	}
	for i := 0; i < len(an.Keys)-1; i++ {
		k0, k1 := an.Keys[i], an.Keys[i+1]
		if phase >= k0.T && phase <= k1.T {
			span := k1.T - k0.T
			t := 0.0
			if span > 1e-9 {
				t = (phase - k0.T) / span
			}
			return LerpPose(k0.Pose, k1.Pose, t)
		}
	}
	return an.Keys[len(an.Keys)-1].Pose
}

// RenderFrames는 애니메이션을 n프레임으로 렌더링합니다.
// Loop면 마지막 프레임이 첫 프레임으로 매끄럽게 이어지도록 [0,1) 균등 샘플.
func RenderFrames(sk *Skeleton, an *Animation, n, w, h int) []*image.NRGBA {
	if n < 1 {
		n = 1
	}
	frames := make([]*image.NRGBA, 0, n)
	for i := 0; i < n; i++ {
		var phase float64
		if an.Loop {
			phase = float64(i) / float64(n) // [0,1) — i=n이 i=0과 동일해 매끄러운 루프
		} else if n == 1 {
			phase = 0
		} else {
			phase = float64(i) / float64(n-1) // [0,1] — 시작~끝
		}
		frames = append(frames, Render(sk, an.SamplePose(phase), w, h))
	}
	return frames
}
