package sprite

import (
	"fmt"
	"image"

	xdraw "golang.org/x/image/draw"
)

const alphaThreshold = 10 // 이 값 이하의 알파는 빈(투명) 픽셀로 취급

// frameContent는 스트립 좌표계에서 추출한 한 포즈의 콘텐츠입니다.
type frameContent struct {
	img    *image.NRGBA // bbox로 자른 콘텐츠
	minX   int
	cx     float64 // 알파 가중 질량 중심 X (스트립 좌표)
	bottom int     // 베이스라인(콘텐츠 최하단 행, 스트립 좌표)
}

// connectedKeepMask는 span×h 영역에서 연결성분(8근방, gap px 빈틈 브리지)을 찾아,
// 가장 큰 성분과 그 크기의 40% 이상인 성분만 유지하는 픽셀 마스크(길이 w*h)를 만듭니다.
// 몸통과 이어진 발/팔/머리카락/들고 있는 무기는 같은 성분이라 유지되고(=발 잘림 방지),
// 옆 포즈에서 빈 칸 너머로 새어든 동떨어진 조각만 버립니다. 인덱스: y*w + (x-span.start).
func connectedKeepMask(strip *image.NRGBA, span colSpan, h, gap int) []bool {
	w := span.end - span.start
	if w <= 0 || h <= 0 {
		return make([]bool, 0)
	}
	keep := make([]bool, w*h)
	op := make([]bool, w*h)
	for y := 0; y < h; y++ {
		base := strip.PixOffset(span.start, y) + 3
		for lx := 0; lx < w; lx++ {
			if strip.Pix[base+lx*4] > alphaThreshold {
				op[y*w+lx] = true
			}
		}
	}
	label := make([]int, w*h)
	for i := range label {
		label[i] = -1
	}
	var sizes []int
	stack := make([]int, 0, 1024)
	for s := 0; s < w*h; s++ {
		if !op[s] || label[s] != -1 {
			continue
		}
		id := len(sizes)
		size := 0
		label[s] = id
		stack = append(stack[:0], s)
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			size++
			px, py := p%w, p/w
			for dy := -gap; dy <= gap; dy++ {
				ny := py + dy
				if ny < 0 || ny >= h {
					continue
				}
				for dx := -gap; dx <= gap; dx++ {
					nx := px + dx
					if nx < 0 || nx >= w {
						continue
					}
					q := ny*w + nx
					if op[q] && label[q] == -1 {
						label[q] = id
						stack = append(stack, q)
					}
				}
			}
		}
		sizes = append(sizes, size)
	}
	if len(sizes) == 0 {
		return keep
	}
	maxSize := 0
	for _, sz := range sizes {
		if sz > maxSize {
			maxSize = sz
		}
	}
	thr := maxSize * 40 / 100 // 최대 성분의 40% 미만 동떨어진 조각은 버림
	for i := 0; i < w*h; i++ {
		if label[i] >= 0 && sizes[label[i]] >= thr {
			keep[i] = true
		}
	}
	return keep
}

// extractContent는 span 구간 안의 불투명 픽셀을 bbox로 잘라냅니다(컬럼클립 경로).
// 메인 몸체와 연결되지 않은 동떨어진 조각(옆 포즈 누출)만 connectedKeepMask로 걸러냅니다.
// 포즈가 서로 닿아 컬럼 분할이 불가피한 경우의 폴백입니다.
func extractContent(strip *image.NRGBA, span colSpan, h int) frameContent {
	w := span.end - span.start
	keep := connectedKeepMask(strip, span, h, 2)
	return extractWithKeep(strip, span.start, span.end, h, func(x, y int) bool {
		i := y*w + (x - span.start)
		return i >= 0 && i < len(keep) && keep[i]
	})
}

// extractWithKeep는 [xlo,xhi)×h 영역에서 kept(x,y)가 참인 픽셀을 bbox로 잘라냅니다.
// 수평 앵커(cx)는 가장 조밀한 세로 열 묶음(=토르소/몸통 코어)의 중심을 씁니다. 얇게 뻗은
// 팔다리는 열 픽셀 수가 적어 자동 제외되므로 스윙에 불변 → 프레임 정렬이 안정됩니다.
func extractWithKeep(strip *image.NRGBA, xlo, xhi, h int, kept func(x, y int) bool) frameContent {
	minX, minY, maxX, maxY := xhi, h, xlo-1, -1
	var sumWX, sumW float64
	for x := xlo; x < xhi; x++ {
		for y := 0; y < h; y++ {
			if !kept(x, y) {
				continue
			}
			a := strip.Pix[strip.PixOffset(x, y)+3]
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
			sumWX += float64(x) * float64(a)
			sumW += float64(a)
		}
	}
	if maxX < minX || maxY < minY {
		return frameContent{}
	}
	gw, gh := maxX-minX+1, maxY-minY+1
	dst := image.NewNRGBA(image.Rect(0, 0, gw, gh))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if !kept(x, y) {
				continue
			}
			si := strip.PixOffset(x, y)
			di := dst.PixOffset(x-minX, y-minY)
			copy(dst.Pix[di:di+4], strip.Pix[si:si+4])
		}
	}
	colCount := make([]int, gw)
	maxCol := 0
	for x := 0; x < gw; x++ {
		c := 0
		for y := 0; y < gh; y++ {
			if dst.Pix[dst.PixOffset(x, y)+3] > alphaThreshold {
				c++
			}
		}
		colCount[x] = c
		if c > maxCol {
			maxCol = c
		}
	}
	cx := float64(minX+maxX+1) / 2
	if maxCol > 0 {
		thr := maxCol * 60 / 100 // 최대 열 높이의 60% 이상 = 몸통 코어
		first, last := -1, -1
		for x := 0; x < gw; x++ {
			if colCount[x] >= thr {
				if first < 0 {
					first = x
				}
				last = x
			}
		}
		if first >= 0 {
			cx = float64(minX) + float64(first+last)/2
		}
	} else if sumW > 0 {
		cx = sumWX / sumW
	}
	return frameContent{img: dst, minX: minX, cx: cx, bottom: maxY}
}

// component는 전체 스트립 연결성분 하나의 통계입니다.
type component struct {
	size                   int
	sumX                   float64
	minX, minY, maxX, maxY int
}

func (c component) centroidX() float64 {
	if c.size == 0 {
		return 0
	}
	return c.sumX / float64(c.size)
}

// labelStrip은 전체 스트립을 8근방(gap px 빈틈 브리지) 연결성분으로 라벨링합니다.
// labels[y*w+x] = 성분 id(투명/배경은 -1). 성분별 크기·무게중심X·바운딩박스를 함께 반환합니다.
func labelStrip(img *image.NRGBA, gap int) (labels []int, comps []component) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w <= 0 || h <= 0 {
		return nil, nil
	}
	labels = make([]int, w*h)
	for i := range labels {
		labels[i] = -1
	}
	op := make([]bool, w*h)
	for y := 0; y < h; y++ {
		base := img.PixOffset(0, y) + 3
		for x := 0; x < w; x++ {
			if img.Pix[base+x*4] > alphaThreshold {
				op[y*w+x] = true
			}
		}
	}
	stack := make([]int, 0, 1024)
	for s := 0; s < w*h; s++ {
		if !op[s] || labels[s] != -1 {
			continue
		}
		id := len(comps)
		c := component{minX: w, minY: h, maxX: -1, maxY: -1}
		labels[s] = id
		stack = append(stack[:0], s)
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			px, py := p%w, p/w
			c.size++
			c.sumX += float64(px)
			if px < c.minX {
				c.minX = px
			}
			if px > c.maxX {
				c.maxX = px
			}
			if py < c.minY {
				c.minY = py
			}
			if py > c.maxY {
				c.maxY = py
			}
			for dy := -gap; dy <= gap; dy++ {
				ny := py + dy
				if ny < 0 || ny >= h {
					continue
				}
				for dx := -gap; dx <= gap; dx++ {
					nx := px + dx
					if nx < 0 || nx >= w {
						continue
					}
					q := ny*w + nx
					if op[q] && labels[q] == -1 {
						labels[q] = id
						stack = append(stack, q)
					}
				}
			}
		}
		comps = append(comps, c)
	}
	return labels, comps
}

// bboxGap은 두 성분 바운딩박스의 떨어진 거리(겹치면 0)를 반환합니다.
func bboxGap(a, b component) int {
	dx := 0
	if b.minX > a.maxX {
		dx = b.minX - a.maxX
	} else if a.minX > b.maxX {
		dx = a.minX - b.maxX
	}
	dy := 0
	if b.minY > a.maxY {
		dy = b.minY - a.maxY
	} else if a.minY > b.maxY {
		dy = a.minY - b.maxY
	}
	if dx > dy {
		return dx
	}
	return dy
}

// extractByComponents는 전체 스트립 연결성분을 각 세그먼트(무게중심 X 기준)에 배정하고,
// 세그먼트별로 배정된 성분들의 픽셀 합집합을 프레임으로 추출합니다. 포즈가 서로 닿지
// 않으면 각 포즈가 한 성분이라, 가로로 멀리 뻗은 발도 그 성분에 포함되어 잘리지 않습니다.
// 무게중심이 없는 세그먼트(인접 성분이 가져감)는 컬럼클립으로 폴백합니다.
func extractByComponents(strip *image.NRGBA, labels []int, comps []component, segs []colSpan, h int) []frameContent {
	w := strip.Rect.Dx()
	maxSize := 0
	for _, c := range comps {
		if c.size > maxSize {
			maxSize = c.size
		}
	}
	noise := maxSize * 2 / 100 // 최대 성분의 2% 미만은 잡티로 무시
	var fcs []frameContent
	for _, seg := range segs {
		main, mainSize := -1, 0
		var ids []int
		for id, c := range comps {
			if c.size <= noise {
				continue
			}
			cx := int(c.centroidX())
			if cx >= seg.start && cx < seg.end {
				ids = append(ids, id)
				if c.size > mainSize {
					mainSize, main = c.size, id
				}
			}
		}
		if main < 0 {
			if fc := extractContent(strip, seg, h); fc.img != nil {
				fcs = append(fcs, fc)
			}
			continue
		}
		keepID := map[int]bool{main: true}
		mc := comps[main]
		for _, id := range ids {
			if id == main {
				continue
			}
			c := comps[id]
			if bboxGap(mc, c) <= 15 || c.size*100 >= mainSize*40 {
				keepID[id] = true
			}
		}
		xlo, xhi := w, 0
		for id := range keepID {
			if comps[id].minX < xlo {
				xlo = comps[id].minX
			}
			if comps[id].maxX+1 > xhi {
				xhi = comps[id].maxX + 1
			}
		}
		fc := extractWithKeep(strip, xlo, xhi, h, func(x, y int) bool {
			l := labels[y*w+x]
			return l >= 0 && keepID[l]
		})
		if fc.img != nil {
			fcs = append(fcs, fc)
		}
	}
	return fcs
}

// ExtractFrames는 투명 배경 스트립에서 포즈를 투영 분할로 검출해 셀 크기 프레임으로
// 만듭니다. 모든 프레임에 공통 스케일을 적용하고, 질량 중심으로 수평 정렬하며,
// 공통 베이스라인 기준으로 수직 오프셋(점프 호 등)을 보존합니다.
func ExtractFrames(strip *image.NRGBA, expected, cellW, cellH, margin int) ExtractResult {
	res := ExtractResult{Expected: expected}
	segs, natural := segmentStrip(strip, expected)
	if len(segs) == 0 {
		res.Warnings = append(res.Warnings, "이미지에서 캐릭터를 찾지 못했습니다. 다시 생성해 주세요.")
		return res
	}
	h := strip.Rect.Dy()

	// 전체 스트립을 연결성분으로 라벨링. 유의미 성분(최대의 2% 이상) 수가 세그먼트 수
	// 이상이면 = 포즈들이 서로 떨어져 있다는 뜻 → 성분 기반 추출(발이 옆으로 뻗어도 안 잘림).
	// 적으면(포즈가 닿아 한 덩어리) 컬럼클립으로 폴백.
	labels, comps := labelStrip(strip, 2)
	maxSize := 0
	for _, c := range comps {
		if c.size > maxSize {
			maxSize = c.size
		}
	}
	sig := 0
	for _, c := range comps {
		if c.size*100 >= maxSize*2 {
			sig++
		}
	}

	var fcs []frameContent
	if len(comps) > 0 && sig >= len(segs) {
		fcs = extractByComponents(strip, labels, comps, segs, h)
	} else {
		for _, s := range segs {
			if fc := extractContent(strip, s, h); fc.img != nil {
				fcs = append(fcs, fc)
			}
		}
	}
	if len(fcs) == 0 {
		res.Warnings = append(res.Warnings, "유효한 포즈를 찾지 못했습니다. 다시 생성해 주세요.")
		return res
	}

	res.Frames = placeFrames(fcs, cellW, cellH, margin)
	res.Found = len(fcs)
	if res.Found != expected {
		res.Warnings = append(res.Warnings,
			fmt.Sprintf("기대한 %d개와 다른 %d개의 포즈가 감지되었습니다. 포즈가 겹쳤거나 누락됐을 수 있어 재생성을 권장합니다.", expected, res.Found))
	}
	_ = natural
	return res
}

// placeFrames는 추출된 콘텐츠들을 공통 베이스라인 + 공유 스케일로 셀에 배치합니다.
// (스트립/그리드 추출 공용)
func placeFrames(fcs []frameContent, cellW, cellH, margin int) []*image.NRGBA {
	var out []*image.NRGBA
	if len(fcs) == 0 {
		return out
	}
	baseline := 0
	for _, g := range fcs {
		if g.bottom > baseline {
			baseline = g.bottom
		}
	}
	availW := cellW - margin*2
	availH := cellH - margin*2
	if availW < 8 || availH < 8 {
		availW, availH = cellW, cellH
	}
	maxW, maxEffH := 1, 1
	for _, g := range fcs {
		offset := baseline - g.bottom
		if g.img.Rect.Dx() > maxW {
			maxW = g.img.Rect.Dx()
		}
		if eff := g.img.Rect.Dy() + offset; eff > maxEffH {
			maxEffH = eff
		}
	}
	scale := minf(float64(availW)/float64(maxW), float64(availH)/float64(maxEffH))
	if scale > 1 {
		scale = 1
	}
	for _, g := range fcs {
		sw := int(float64(g.img.Rect.Dx())*scale + 0.5)
		sh := int(float64(g.img.Rect.Dy())*scale + 0.5)
		if sw < 1 {
			sw = 1
		}
		if sh < 1 {
			sh = 1
		}
		scaled := g.img
		if sw != g.img.Rect.Dx() || sh != g.img.Rect.Dy() {
			scaled = image.NewNRGBA(image.Rect(0, 0, sw, sh))
			xdraw.CatmullRom.Scale(scaled, scaled.Rect, g.img, g.img.Rect, xdraw.Over, nil)
		}
		offset := int(float64(baseline-g.bottom)*scale + 0.5)
		cell := image.NewNRGBA(image.Rect(0, 0, cellW, cellH))
		left := int(float64(cellW)/2 - (g.cx-float64(g.minX))*scale + 0.5)
		if left < 0 {
			left = 0
		}
		if left+sw > cellW {
			left = cellW - sw
		}
		top := cellH - margin - offset - sh
		if top < 0 {
			top = 0
		}
		xdraw.Copy(cell, image.Point{X: left, Y: top}, scaled, scaled.Rect, xdraw.Over, nil)
		out = append(out, cell)
	}
	return out
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
