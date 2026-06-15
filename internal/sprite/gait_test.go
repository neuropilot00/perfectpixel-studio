package sprite

import (
	"strings"
	"testing"
)

func TestGaitSchedule(t *testing.T) {
	// 보행류가 아니면 빈 문자열
	if s := gaitSchedule("idle", 8); s != "" {
		t.Fatalf("비보행류인데 스케줄 생성됨: %q", s)
	}
	// 프레임 4 미만이면 빈 문자열
	if s := gaitSchedule("run", 3); s != "" {
		t.Fatalf("3프레임인데 스케줄 생성됨")
	}
	// 6프레임 달리기: 앞 절반은 RIGHT 주도, 뒤 절반은 LEFT 주도여야
	s := gaitSchedule("달리기", 6)
	if s == "" {
		t.Fatal("달리기 스케줄이 비었음")
	}
	for _, want := range []string{"Pose 1", "Pose 4", "Pose 6", "MOST IMPORTANT", "OPPOSITE of Pose 1"} {
		if !strings.Contains(s, want) {
			t.Fatalf("스케줄에 %q 누락:\n%s", want, s)
		}
	}
	// Pose 1 줄엔 RIGHT, Pose 4 줄엔 LEFT가 앞발로 명시되어야(교대 강제)
	lines := strings.Split(s, "\n")
	var p1, p4 string
	for _, ln := range lines {
		if strings.HasPrefix(ln, "- Pose 1:") {
			p1 = ln
		}
		if strings.HasPrefix(ln, "- Pose 4:") {
			p4 = ln
		}
	}
	if !strings.Contains(p1, "RIGHT foot striking") {
		t.Fatalf("Pose 1이 오른발 주도가 아님: %q", p1)
	}
	if !strings.Contains(p4, "LEFT foot striking") {
		t.Fatalf("Pose 4가 왼발 주도가 아님(교대 실패): %q", p4)
	}
}
