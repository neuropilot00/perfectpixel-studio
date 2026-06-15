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
