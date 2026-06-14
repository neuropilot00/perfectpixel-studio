package gen

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CodexCLI는 로컬에 설치된 Codex CLI의 내장 이미지 생성($imagegen / gpt-image-2)을
// 호출하는 프로바이더입니다. API 키가 필요 없고 Codex 로그인 세션을 사용하며,
// PixelLab 등 외부 MCP 이미지 도구를 쓰지 않습니다.
type CodexCLI struct {
	Bin     string // codex 실행 파일 (기본 "codex")
	Model   string
	Timeout time.Duration
}

// NewCodexCLI는 Codex CLI 프로바이더를 생성합니다.
func NewCodexCLI(model string) *CodexCLI {
	if model == "" {
		model = "gpt-image-2"
	}
	return &CodexCLI{Bin: CodexBinPath(), Model: model, Timeout: 300 * time.Second}
}

// commonBinDirs는 GUI 앱(Finder 실행 시 최소 PATH)에서도 CLI를 찾기 위한 후보 디렉토리입니다.
func commonBinDirs() []string {
	home, _ := os.UserHomeDir()
	return []string{
		"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin",
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, "go", "bin"),
		filepath.Join(home, ".bun", "bin"),
		"/opt/homebrew/opt/node/bin",
	}
}

// FindBin은 CLI 실행 파일의 절대 경로를 견고하게 찾습니다.
// Finder로 띄운 앱은 PATH가 최소라 LookPath만으론 못 찾으므로 일반 설치 경로도 탐색합니다.
func FindBin(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, d := range commonBinDirs() {
		c := filepath.Join(d, name)
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return name
}

// CodexBinPath는 codex 실행 파일 경로를 반환합니다(CODEX_BIN 환경변수 우선).
func CodexBinPath() string {
	if b := os.Getenv("CODEX_BIN"); b != "" {
		return b
	}
	return FindBin("codex")
}

// authBreakers는 자식 claude의 구독 OAuth 인증을 가로채/깨뜨릴 수 있는 환경변수들입니다.
// (부모 Claude Code 세션에서 상속되면 401/오인증을 유발. CLAUDE_CODE_OAUTH_TOKEN은 유지해야 함.)
var authBreakers = map[string]bool{
	"ANTHROPIC_API_KEY": true, "ANTHROPIC_AUTH_TOKEN": true, "ANTHROPIC_BASE_URL": true,
	"CLAUDE_CODE_USE_BEDROCK": true, "CLAUDE_CODE_USE_VERTEX": true, "CLAUDE_CODE_USE_FOUNDRY": true,
	"CLAUDECODE": true, "CLAUDE_CODE_ENTRYPOINT": true, "CLAUDE_CODE_SESSION_ID": true,
	"CLAUDE_CODE_SDK_HAS_OAUTH_REFRESH": true, "CLAUDE_CODE_SDK_HAS_HOST_AUTH_REFRESH": true,
	"CLAUDE_CODE_OAUTH_SCOPES": true, "CLAUDE_AGENT_SDK_VERSION": true, "AI_AGENT": true,
	"CLAUDE_CODE_EXECPATH": true, "CLAUDE_PLUGIN_DATA": true,
}

// SanitizedAuthEnv는 AugmentedEnv에서 claude 구독 인증을 깨뜨리는 변수를 제거합니다.
// 앱이 자식 claude를 부를 때 사용 — 사용자의 구독 OAuth(키체인/CLAUDE_CODE_OAUTH_TOKEN)로 인증되게 합니다.
func SanitizedAuthEnv() []string {
	src := AugmentedEnv()
	out := make([]string, 0, len(src))
	for _, kv := range src {
		eq := strings.IndexByte(kv, '=')
		if eq > 0 && authBreakers[kv[:eq]] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// AugmentedEnv는 자식 프로세스(codex)가 node 등 의존 도구를 찾을 수 있도록 PATH를 보강합니다.
func AugmentedEnv() []string {
	env := os.Environ()
	extra := strings.Join(commonBinDirs(), ":")
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			env[i] = "PATH=" + kv[len("PATH="):] + ":" + extra
			return env
		}
	}
	return append(env, "PATH="+extra)
}

// GenerateImage는 codex exec를 비대화형으로 실행해 PNG 파일을 생성하고 그 바이트를 반환합니다.
func (c *CodexCLI) GenerateImage(ctx context.Context, prompt string, refImages [][]byte, aspectRatio string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "ppcodex-")
	if err != nil {
		return nil, fmt.Errorf("임시 디렉토리 생성 실패: %w", err)
	}
	defer os.RemoveAll(dir)

	outPath := filepath.Join(dir, "out.png")

	var refLines []string
	for i, r := range refImages {
		rp := filepath.Join(dir, fmt.Sprintf("ref%d.png", i))
		if err := os.WriteFile(rp, r, 0o644); err != nil {
			return nil, fmt.Errorf("참조 이미지 저장 실패: %w", err)
		}
		refLines = append(refLines, rp)
	}

	instruction := buildCodexInstruction(prompt, outPath, refLines, aspectRatio)

	runCtx := ctx
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, c.Bin,
		"exec", "--skip-git-repo-check", "--sandbox", "workspace-write", instruction)
	cmd.Dir = dir
	cmd.Stdin = nil // /dev/null — stdin 대기 방지
	cmd.Env = AugmentedEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("Codex 이미지 생성 시간 초과(%s)", c.Timeout)
		}
		return nil, fmt.Errorf("Codex 실행 실패: %v — %s", err, tailString(stderr.String(), 400))
	}

	data, err := os.ReadFile(outPath)
	if err != nil || len(data) == 0 {
		return nil, fmt.Errorf("Codex가 이미지를 생성하지 못했습니다. 출력: %s", tailString(stderr.String(), 400))
	}
	return data, nil
}

// buildCodexInstruction은 codex에게 내장 이미지 도구만 쓰도록 지시하는 프롬프트를 만듭니다.
// 중요: 아래 이미지 명세(perfectpixel 프롬프트)에는 배경을 마젠타로 채우는 키잉 계약 등이
// 포함되어 있으므로, codex가 임의로 배경을 제거하거나 투명화/크롭/후처리하지 않고
// 명세 그대로 출력하게 해야 후속 파이프라인의 매팅·추출이 동작합니다.
func buildCodexInstruction(prompt, outPath string, refs []string, aspectRatio string) string {
	var b strings.Builder
	b.WriteString("Use ONLY your built-in image generation tool ($imagegen / gpt-image-2). ")
	b.WriteString("Do NOT use the pixellab MCP or any other external MCP image tool. ")
	b.WriteString("Follow the IMAGE SPECIFICATION below EXACTLY — especially any background color and framing rules. ")
	b.WriteString("Do NOT remove, key out, or make the background transparent yourself; do NOT crop, matte, upscale, or otherwise post-process. ")
	b.WriteString("Output the raw generated image exactly as specified (including any solid background fill). ")
	if aspectRatio != "" && aspectRatio != "1:1" {
		b.WriteString("Target a wide " + aspectRatio + " aspect ratio. ")
	}
	if len(refs) > 0 {
		b.WriteString("Use these reference image file(s) for character/style consistency: ")
		b.WriteString(strings.Join(refs, ", "))
		b.WriteString(". ")
	}
	b.WriteString("Save the result as a PNG to exactly this path: ")
	b.WriteString(outPath)
	b.WriteString(".\nDo not ask any questions. Produce only the file at the given path.\n\n=== IMAGE SPECIFICATION ===\n")
	b.WriteString(prompt)
	return b.String()
}

func tailString(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

// ValidateKey는 Codex CLI가 설치되어 있고 로그인되어 있는지 가볍게 확인합니다.
// (API 키가 없으므로 바이너리 존재 + 버전 확인만 수행합니다.)
func (c *CodexCLI) ValidateKey(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.Bin, "--version")
	cmd.Env = AugmentedEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("Codex CLI를 찾을 수 없습니다(%q). 설치 후 'codex login'으로 로그인해 주세요: %s",
			c.Bin, tailString(string(out), 200))
	}
	return nil
}
