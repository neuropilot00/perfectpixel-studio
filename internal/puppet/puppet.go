// Package puppet은 라이브2D식 실시간 2D 퍼펫의 데이터 모델과 변형(deformation)을 제공합니다.
//
// Live2D Cubism Core는 독점 바이너리(.moc3, Go 런타임 없음)라 클론할 수 없으므로,
// 오픈 표준 Inochi2D(파라미터→deform 바인딩 구조)를 본떠 자체 엔진을 만듭니다.
// 변형 primitive는 internal/rig(GridMesh + WarpRender 바리센트릭 텍스처 워프)를 재사용합니다.
//
// 모델 = Part(레이어=텍스처 메시) 여러 개 + Parameter(슬라이더) 여러 개.
// 각 Parameter는 Binding으로 특정 Part의 메시를 변형하며, Binding은 파라미터 값별
// 정점 오프셋 키프레임을 갖습니다(런타임이 그 사이를 선형 보간). 여러 파라미터가 같은
// Part를 변형하면 오프셋이 합산됩니다(Inochi2D와 동일).
package puppet

import (
	"image"
	"sort"

	"perfectpixel/internal/rig"
)

// Part는 한 레이어입니다: rest 상태 메시 + 텍스처 키 + z정렬 순서.
// 텍스처 픽셀은 직렬화에서 분리(PNG 별도)하므로 여기엔 키(TextureKey)만 둡니다.
type Part struct {
	Name       string    `json:"name"`
	TextureKey string    `json:"textureKey"`
	ZSort      float64   `json:"zSort"`
	Mesh       *rig.Mesh `json:"mesh"`
}

// DeformKey는 특정 파라미터 값(Value)에서의 정점별 오프셋(dx,dy)입니다.
// len(Offsets)는 대상 Part 메시의 정점 수와 같아야 합니다.
type DeformKey struct {
	Value   float64      `json:"value"`
	Offsets [][2]float64 `json:"offsets"`
}

// Binding은 한 파라미터가 한 Part의 메시를 변형하는 키프레임 모음입니다(Value 오름차순).
type Binding struct {
	Part string      `json:"part"`
	Keys []DeformKey `json:"keys"`
}

// interp는 파라미터 값 v에서의 정점 오프셋을 키프레임 선형 보간으로 구합니다.
// 키가 없으면 nil, v가 범위를 벗어나면 양끝 키로 클램프합니다.
func (b Binding) interp(v float64) [][2]float64 {
	n := len(b.Keys)
	if n == 0 {
		return nil
	}
	if n == 1 || v <= b.Keys[0].Value {
		return b.Keys[0].Offsets
	}
	if v >= b.Keys[n-1].Value {
		return b.Keys[n-1].Offsets
	}
	// v를 감싸는 두 키 찾기
	hi := sort.Search(n, func(i int) bool { return b.Keys[i].Value >= v })
	lo := hi - 1
	k0, k1 := b.Keys[lo], b.Keys[hi]
	span := k1.Value - k0.Value
	if span <= 0 {
		return k0.Offsets
	}
	t := (v - k0.Value) / span
	m := len(k0.Offsets)
	if len(k1.Offsets) < m {
		m = len(k1.Offsets)
	}
	out := make([][2]float64, m)
	for i := 0; i < m; i++ {
		out[i][0] = k0.Offsets[i][0] + (k1.Offsets[i][0]-k0.Offsets[i][0])*t
		out[i][1] = k0.Offsets[i][1] + (k1.Offsets[i][1]-k0.Offsets[i][1])*t
	}
	return out
}

// Parameter는 슬라이더(이름·범위·기본값)와 그것이 구동하는 바인딩들입니다.
type Parameter struct {
	Name     string    `json:"name"`
	Min      float64   `json:"min"`
	Max      float64   `json:"max"`
	Default  float64   `json:"default"`
	Bindings []Binding `json:"bindings"`
}

// Puppet은 파트와 파라미터의 집합입니다(라이브2D 모델 한 개).
type Puppet struct {
	Name   string      `json:"name"`
	Parts  []Part      `json:"parts"`
	Params []Parameter `json:"params"`
}

// AutoMeshPart는 텍스처 크기에 맞춘 cols×rows 격자 메시 Part를 만듭니다.
// (단일 레이어 자동 메시화 — Phase 1. 이후 불투명 영역 클리핑/Delaunay로 정교화 가능)
func AutoMeshPart(name, textureKey string, texW, texH, cols, rows int, z float64) Part {
	return Part{
		Name:       name,
		TextureKey: textureKey,
		ZSort:      z,
		Mesh:       rig.GridMesh(texW, texH, cols, rows),
	}
}

// DeformPart는 파라미터 값 맵에 따라 partIdx 파트의 변형된 메시를 반환합니다.
// rest 메시에서 시작해, 이 파트를 대상으로 하는 모든 파라미터의 보간 오프셋을 합산합니다.
// 원본 rest 메시는 변경하지 않습니다(복사본 반환).
func (p *Puppet) DeformPart(partIdx int, values map[string]float64) *rig.Mesh {
	if partIdx < 0 || partIdx >= len(p.Parts) {
		return nil
	}
	src := p.Parts[partIdx].Mesh
	if src == nil {
		return nil
	}
	out := &rig.Mesh{
		Verts: make([]rig.Vertex, len(src.Verts)),
		Tris:  src.Tris, // 인덱스는 불변 → 공유
	}
	copy(out.Verts, src.Verts)

	name := p.Parts[partIdx].Name
	for _, param := range p.Params {
		v, ok := values[param.Name]
		if !ok {
			v = param.Default
		}
		for _, b := range param.Bindings {
			if b.Part != name {
				continue
			}
			offs := b.interp(v)
			for i := 0; i < len(offs) && i < len(out.Verts); i++ {
				out.Verts[i].X += offs[i][0]
				out.Verts[i].Y += offs[i][1]
			}
		}
	}
	return out
}

// Render는 모든 파트를 z정렬(작은 ZSort부터=뒤)하여 변형·합성합니다.
// textures는 Part.TextureKey → 텍스처 이미지 맵입니다. dst는 호출자가 크기를 정합니다.
func (p *Puppet) Render(dst *image.NRGBA, textures map[string]*image.NRGBA, values map[string]float64) {
	order := make([]int, len(p.Parts))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return p.Parts[order[a]].ZSort < p.Parts[order[b]].ZSort
	})
	for _, idx := range order {
		tex := textures[p.Parts[idx].TextureKey]
		if tex == nil {
			continue
		}
		mesh := p.DeformPart(idx, values)
		if mesh == nil {
			continue
		}
		rig.WarpRender(dst, tex, mesh)
	}
}

// DefaultValues는 모든 파라미터를 기본값으로 채운 맵을 반환합니다.
func (p *Puppet) DefaultValues() map[string]float64 {
	m := make(map[string]float64, len(p.Params))
	for _, param := range p.Params {
		m[param.Name] = param.Default
	}
	return m
}
