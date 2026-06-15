package rig

import (
	"image"
	"math"
)

// Live2D식 메시 변형(mesh deformation)의 핵심 primitive.
// 텍스처를 삼각 메시에 입히고(UV 고정), 정점을 움직이면 텍스처가 매끄럽게 따라 휜다.
// 강체 회전과 달리 관절을 공유하는 정점들이 부드럽게 이어져 각진 이음새가 생기지 않는다.

// Vertex는 변형된 화면 위치(X,Y)와 원본 텍스처 좌표(U,V, 텍스처 픽셀)를 갖습니다.
type Vertex struct {
	X, Y float64 // 현재(변형된) 위치
	U, V float64 // 원본 텍스처 좌표(고정)
}

// Mesh는 정점 배열과 삼각형(정점 인덱스 3개) 목록입니다.
type Mesh struct {
	Verts []Vertex
	Tris  [][3]int
}

// GridMesh는 (w×h) 텍스처 위에 cols×rows 격자 메시를 만듭니다(rest 상태: X,Y=U,V).
// 정점을 이후 변형하면 WarpRender가 텍스처를 따라 휘게 그립니다.
func GridMesh(w, h, cols, rows int) *Mesh {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	m := &Mesh{}
	idx := func(c, r int) int { return r*(cols+1) + c }
	for r := 0; r <= rows; r++ {
		for c := 0; c <= cols; c++ {
			x := float64(w) * float64(c) / float64(cols)
			y := float64(h) * float64(r) / float64(rows)
			m.Verts = append(m.Verts, Vertex{X: x, Y: y, U: x, V: y})
		}
	}
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			a, b, cc, d := idx(c, r), idx(c+1, r), idx(c, r+1), idx(c+1, r+1)
			m.Tris = append(m.Tris, [3]int{a, b, cc}, [3]int{b, d, cc})
		}
	}
	return m
}

func edgeFn(ax, ay, bx, by, px, py float64) float64 {
	return (px-ax)*(by-ay) - (py-ay)*(bx-ax)
}

// WarpRender는 메시의 현재 정점 위치에 맞춰 텍스처를 변형 합성합니다(삼각형별 바리센트릭 샘플링).
// 픽셀아트 보존을 위해 nearest-neighbor 샘플.
func WarpRender(dst, tex *image.NRGBA, m *Mesh) {
	tw, th := tex.Rect.Dx(), tex.Rect.Dy()
	dw, dh := dst.Rect.Dx(), dst.Rect.Dy()
	for _, tri := range m.Tris {
		a, b, c := m.Verts[tri[0]], m.Verts[tri[1]], m.Verts[tri[2]]
		area := edgeFn(a.X, a.Y, b.X, b.Y, c.X, c.Y)
		if math.Abs(area) < 1e-9 {
			continue // 퇴화 삼각형
		}
		minX := int(math.Floor(min3(a.X, b.X, c.X)))
		maxX := int(math.Ceil(max3(a.X, b.X, c.X)))
		minY := int(math.Floor(min3(a.Y, b.Y, c.Y)))
		maxY := int(math.Ceil(max3(a.Y, b.Y, c.Y)))
		for y := minY; y <= maxY; y++ {
			if y < 0 || y >= dh {
				continue
			}
			for x := minX; x <= maxX; x++ {
				if x < 0 || x >= dw {
					continue
				}
				px, py := float64(x)+0.5, float64(y)+0.5
				w0 := edgeFn(b.X, b.Y, c.X, c.Y, px, py) / area
				w1 := edgeFn(c.X, c.Y, a.X, a.Y, px, py) / area
				w2 := edgeFn(a.X, a.Y, b.X, b.Y, px, py) / area
				if w0 < 0 || w1 < 0 || w2 < 0 {
					continue // 삼각형 밖
				}
				u := w0*a.U + w1*b.U + w2*c.U
				v := w0*a.V + w1*b.V + w2*c.V
				sx, sy := int(u), int(v)
				if sx < 0 || sx >= tw || sy < 0 || sy >= th {
					continue
				}
				si := tex.PixOffset(sx, sy)
				if tex.Pix[si+3] == 0 {
					continue
				}
				di := dst.PixOffset(x, y)
				sa := tex.Pix[si+3]
				if sa == 255 {
					copy(dst.Pix[di:di+4], tex.Pix[si:si+4])
					continue
				}
				af := float64(sa) / 255
				for k := 0; k < 3; k++ {
					dst.Pix[di+k] = uint8(float64(tex.Pix[si+k])*af + float64(dst.Pix[di+k])*(1-af))
				}
				dst.Pix[di+3] = uint8(float64(sa) + float64(dst.Pix[di+3])*(1-af))
			}
		}
	}
}

func min3(a, b, c float64) float64 { return math.Min(a, math.Min(b, c)) }
func max3(a, b, c float64) float64 { return math.Max(a, math.Max(b, c)) }
