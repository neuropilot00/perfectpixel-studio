package sprite

import (
	"image"
	"testing"
)

// TestExtractKeepsConnectedFoot은 몸통에서 멀리 뻗었지만 다리로 연결된 발이
// 유지되고(발 잘림 해결), 연결되지 않은 동떨어진 조각(옆 포즈 누출)은 버려지는지 검증합니다.
func TestExtractKeepsConnectedFoot(t *testing.T) {
	W, H := 220, 110
	strip := image.NewNRGBA(image.Rect(0, 0, W, H))
	fillBox(strip, 40, 20, 70, 80, 200, 100, 50)  // 몸통
	fillBox(strip, 68, 76, 130, 82, 200, 100, 50) // 가는 다리(몸통→발 연결)
	fillBox(strip, 122, 76, 145, 96, 200, 100, 50) // 발(몸통에서 멀리)
	fillBox(strip, 185, 20, 200, 36, 90, 200, 90)  // 동떨어진 조각(연결 안 됨, 옆 포즈 누출)

	fc := extractContent(strip, colSpan{start: 0, end: W}, H)
	if fc.img == nil {
		t.Fatal("콘텐츠 추출 실패")
	}
	wdt := fc.img.Rect.Dx()
	// 발(오른쪽 끝 ~145)까지 포함 → 폭 ≥ 100 (몸통만이면 ~31, 다리까지면 ~91)
	if wdt < 100 {
		t.Fatalf("발이 잘린 듯: bbox 폭=%d (기대 ≥100)", wdt)
	}
	// 동떨어진 조각(x185~200)이 병합되면 폭이 ~160 이상 → 버려져야 함
	if wdt > 160 {
		t.Fatalf("동떨어진 조각이 병합됨: bbox 폭=%d (기대 ~106)", wdt)
	}
}

// TestComponentKeepsFarFoot은 전체 스트립 연결성분 추출이 가로로 멀리 뻗은 발을
// 한 포즈 성분으로 통째 유지하고(세로 컷으로 안 자름), 떨어진 두 포즈를 2프레임으로
// 정확히 분리하는지 검증합니다(전력질주 발 잘림 케이스).
func TestComponentKeepsFarFoot(t *testing.T) {
	W, H := 420, 140
	strip := image.NewNRGBA(image.Rect(0, 0, W, H))
	// 포즈 A: 토르소 + 가는 다리로 오른쪽 멀리 뻗은 발(x144~168)
	fillBox(strip, 40, 20, 72, 96, 200, 100, 50)
	fillBox(strip, 70, 90, 150, 96, 200, 100, 50)
	fillBox(strip, 144, 90, 168, 116, 200, 100, 50)
	// 포즈 B: 멀리 떨어진 토르소(깨끗한 골)
	fillBox(strip, 250, 20, 282, 116, 200, 100, 50)

	labels, comps := labelStrip(strip, 2)
	idA := labels[50*W+50] // 토르소 A 안의 픽셀
	if idA < 0 {
		t.Fatal("토르소 A 라벨링 실패")
	}
	cA := comps[idA]
	// A 성분이 멀리 뻗은 발(x~168, y~116)까지 포함해야(세로 컷으로 잘리면 maxX가 작아짐)
	if cA.maxX < 160 {
		t.Fatalf("발이 성분에서 빠짐: A.maxX=%d (기대 ≥160)", cA.maxX)
	}
	if cA.maxY < 110 {
		t.Fatalf("발 아래쪽이 성분에서 빠짐: A.maxY=%d (기대 ≥110)", cA.maxY)
	}
	// 끝에서 끝까지 2프레임으로 분리되어야
	res := ExtractFrames(strip, 2, 200, 200, 8)
	if res.Found != 2 {
		t.Fatalf("프레임 수 오류: %d (%v)", res.Found, res.Warnings)
	}
}
