package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"perfectpixel/internal/config"
	"perfectpixel/internal/gen"
	"perfectpixel/internal/rig"
	"perfectpixel/internal/sprite"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App은 Wails 애플리케이션 본체입니다.
type App struct {
	ctx context.Context

	genMu      sync.Mutex
	genCancels map[int]context.CancelFunc // 진행 중인 생성 작업들의 취소 함수 (병렬 배치 지원)
	genSeq     int
}

// NewApp은 새 App 인스턴스를 생성합니다.
func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// provider는 현재 활성 프로바이더 클라이언트를 반환합니다.
func (a *App) provider() (gen.Provider, error) {
	s := config.Load()
	cfg := s.Cfg(s.Provider)
	if cfg.APIKey == "" && !gen.IsKeyless(s.Provider) {
		return nil, fmt.Errorf("%s API 키가 설정되지 않았습니다. 설정에서 입력해 주세요", gen.ProviderLabel(s.Provider))
	}
	return gen.New(s.Provider, cfg.APIKey, cfg.Model)
}

func (a *App) emit(event string, data any) {
	runtime.EventsEmit(a.ctx, event, data)
}

// genContext는 취소 가능한 생성용 컨텍스트를 만듭니다.
// 병렬 배치 생성에서 여러 컨텍스트가 동시에 살아 있을 수 있으므로 각각을 추적하고,
// 호출자가 완료 시 해제할 수 있도록 release 함수를 함께 반환합니다.
func (a *App) genContext() (context.Context, func()) {
	a.genMu.Lock()
	defer a.genMu.Unlock()
	ctx, cancel := context.WithCancel(a.ctx)
	if a.genCancels == nil {
		a.genCancels = make(map[int]context.CancelFunc)
	}
	id := a.genSeq
	a.genSeq++
	a.genCancels[id] = cancel
	release := func() {
		a.genMu.Lock()
		defer a.genMu.Unlock()
		if c, ok := a.genCancels[id]; ok {
			c()
			delete(a.genCancels, id)
		}
	}
	return ctx, release
}

// CancelGeneration은 진행 중인 모든 생성 작업을 취소합니다.
func (a *App) CancelGeneration() {
	a.genMu.Lock()
	defer a.genMu.Unlock()
	for id, cancel := range a.genCancels {
		cancel()
		delete(a.genCancels, id)
	}
}

// friendlyErr는 취소 오류를 사용자 친화적 메시지로 바꿉니다.
func friendlyErr(err error) error {
	if errors.Is(err, context.Canceled) {
		return errors.New("생성이 취소되었습니다")
	}
	return err
}

// ---------- 설정 ----------

// ProviderInfo는 프로바이더별 설정 상태입니다.
type ProviderInfo struct {
	HasKey     bool     `json:"hasKey"`
	KeyPreview string   `json:"keyPreview"`
	Model      string   `json:"model"`
	Models     []string `json:"models"` // 선택 가능한 모델 목록 (최신 모델이 맨 앞)
}

// SettingsInfo는 프론트엔드에 노출되는 설정 상태입니다.
type SettingsInfo struct {
	Provider  string                  `json:"provider"`
	Providers map[string]ProviderInfo `json:"providers"`
}

func keyPreview(key string) string {
	if len(key) > 8 {
		return key[:4] + "····" + key[len(key)-4:]
	}
	return ""
}

// GetSettings는 현재 설정 상태를 반환합니다 (키 원문은 노출하지 않음).
func (a *App) GetSettings() SettingsInfo {
	s := config.Load()
	info := SettingsInfo{Provider: s.Provider, Providers: map[string]ProviderInfo{}}
	for _, p := range gen.SupportedProviders {
		cfg := s.Cfg(p)
		model := cfg.Model
		if model == "" {
			model = gen.DefaultModelFor(p)
		}
		info.Providers[p] = ProviderInfo{
			HasKey:     cfg.APIKey != "" || gen.IsKeyless(p), // 키 불필요 프로바이더는 항상 준비됨으로 표시
			KeyPreview: keyPreview(cfg.APIKey),
			Model:      model,
			Models:     gen.ModelsFor(p),
		}
	}
	return info
}

// SaveProviderKey는 특정 프로바이더의 API 키를 검증 후 저장합니다.
func (a *App) SaveProviderKey(provider, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("API 키를 입력해 주세요")
	}
	c, err := gen.New(provider, key, "")
	if err != nil {
		return err
	}
	if err := c.ValidateKey(a.ctx); err != nil {
		return err
	}
	s := config.Load()
	s.Cfg(provider).APIKey = key
	return config.Save(s)
}

// SaveProviderModel은 프로바이더의 이미지 모델을 변경합니다 (빈 값이면 기본 모델로 복원).
func (a *App) SaveProviderModel(provider, model string) error {
	switch provider {
	case gen.ProviderCodex:
		return nil // Codex는 단일 내장 모델만 사용 — 모델 변경 없음
	case gen.ProviderGemini, gen.ProviderOpenRouter, gen.ProviderFal, gen.ProviderBytePlus:
	default:
		return fmt.Errorf("지원하지 않는 프로바이더입니다: %s", provider)
	}
	model = strings.TrimSpace(model)
	if model == gen.DefaultModelFor(provider) {
		model = "" // 기본 모델은 빈 값으로 저장해 향후 기본값 변경을 따라가게 함
	}
	s := config.Load()
	s.Cfg(provider).Model = model
	return config.Save(s)
}

// SetProvider는 활성 프로바이더를 변경합니다.
func (a *App) SetProvider(provider string) error {
	switch provider {
	case gen.ProviderCodex, gen.ProviderGemini, gen.ProviderOpenRouter, gen.ProviderFal, gen.ProviderBytePlus:
	default:
		return fmt.Errorf("지원하지 않는 프로바이더입니다: %s", provider)
	}
	s := config.Load()
	s.Provider = provider
	return config.Save(s)
}

// ---------- Codex CLI 인증 ----------

// CodexAuthInfo는 Codex CLI 설치/로그인 상태입니다.
type CodexAuthInfo struct {
	Installed bool   `json:"installed"`
	LoggedIn  bool   `json:"loggedIn"`
	Detail    string `json:"detail"`
	BinPath   string `json:"binPath"`
}

// CodexStatus는 codex CLI 설치 및 로그인 상태를 확인합니다.
func (a *App) CodexStatus() CodexAuthInfo {
	bin := gen.CodexBinPath()
	info := CodexAuthInfo{BinPath: bin}
	verCmd := exec.Command(bin, "--version")
	verCmd.Env = gen.AugmentedEnv()
	if _, err := verCmd.CombinedOutput(); err != nil {
		info.Detail = "Codex CLI를 찾을 수 없습니다. 'npm i -g @openai/codex' 등으로 설치해 주세요."
		return info
	}
	info.Installed = true

	stCmd := exec.Command(bin, "login", "status")
	stCmd.Env = gen.AugmentedEnv()
	out, err := stCmd.CombinedOutput()
	detail := strings.TrimSpace(string(out))
	info.Detail = detail
	info.LoggedIn = err == nil && strings.Contains(strings.ToLower(detail), "logged in")
	return info
}

// CodexLogin은 codex 로그인 플로우를 실행합니다(브라우저 OAuth가 열립니다).
// 이미 로그인되어 있으면 즉시 반환합니다.
func (a *App) CodexLogin() (CodexAuthInfo, error) {
	bin := gen.CodexBinPath()
	if cur := a.CodexStatus(); cur.LoggedIn {
		return cur, nil
	}
	ctx, cancel := context.WithTimeout(a.ctx, 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "login")
	cmd.Env = gen.AugmentedEnv()
	out, err := cmd.CombinedOutput()
	info := a.CodexStatus()
	if err != nil && !info.LoggedIn {
		return info, fmt.Errorf("codex 로그인 실패: %v — %s", err, strings.TrimSpace(string(out)))
	}
	return info, nil
}

// ---------- Claude CLI 인증 (AI 챗봇 기획자용) ----------

// ClaudeAuthInfo는 Claude CLI 설치 및 인증 상태입니다.
type ClaudeAuthInfo struct {
	Installed  bool   `json:"installed"`
	HasToken   bool   `json:"hasToken"`   // 설정에 구독 토큰 저장됨
	TokenPrev  string `json:"tokenPrev"`
	BinPath    string `json:"binPath"`
}

// ClaudeStatus는 claude CLI 설치 여부와 저장된 구독 토큰 유무를 반환합니다.
func (a *App) ClaudeStatus() ClaudeAuthInfo {
	bin := gen.FindBin("claude")
	info := ClaudeAuthInfo{BinPath: bin}
	if bin != "claude" {
		info.Installed = true
	} else if _, err := exec.LookPath("claude"); err == nil {
		info.Installed = true
	}
	tok := strings.TrimSpace(config.Load().ClaudeToken)
	if tok != "" {
		info.HasToken = true
		info.TokenPrev = keyPreview(tok)
	}
	return info
}

// SaveClaudeToken은 `claude setup-token`으로 발급한 구독 토큰을 저장합니다(이미지 검증 없음).
func (a *App) SaveClaudeToken(token string) error {
	token = strings.TrimSpace(token)
	s := config.Load()
	s.ClaudeToken = token // 빈 값이면 토큰 제거
	return config.Save(s)
}

// ---------- 내장 AI 챗봇 (에셋 기획자) ----------

// AssetPlan은 챗봇이 만들 에셋 1건의 계획입니다.
type AssetPlan struct {
	Type        string `json:"type"`     // character | background | tile | item
	Description string `json:"description"` // 영어 생성 프롬프트
	StyleKey    string `json:"styleKey"`
	Name        string `json:"name"`
}

// ChatPlanResult는 챗봇 응답 + 생성할 에셋 계획입니다.
type ChatPlanResult struct {
	Reply   string      `json:"reply"`
	Assets  []AssetPlan `json:"assets"`
	Planner string      `json:"planner"` // claude | codex
}

const plannerSystem = `You are the built-in AI assistant of a pixel-art game asset studio. ` +
	`The user chats in Korean. Turn the user's request into concrete game-asset generation tasks. ` +
	`Reply ONLY with a single compact JSON object on its own, no other prose, no markdown fences. Shape:
{"reply":"<short friendly Korean confirmation of what you'll make>","assets":[{"type":"character|background|tile|item","description":"<a vivid ENGLISH image-generation prompt>","styleKey":"pixel|chibi|cartoon|retro16","name":"<short english slug>"}]}
Rules: character=a single creature/person sprite; background=full opaque scene; tile=seamless terrain texture; item=single object/prop. ` +
	`Write rich English prompts (the user may write Korean). If the request is vague, make sensible choices. If the user is just chatting (no asset needed), return an empty assets array with a helpful reply.`

func buildPlannerPrompt(history, message string) string {
	var b strings.Builder
	b.WriteString(plannerSystem)
	if strings.TrimSpace(history) != "" {
		b.WriteString("\n\nConversation so far:\n")
		b.WriteString(history)
	}
	b.WriteString("\n\nUser: ")
	b.WriteString(message)
	b.WriteString("\n\nJSON:")
	return b.String()
}

func extractJSONObject(s string) string {
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i >= 0 && j > i {
		return s[i : j+1]
	}
	return ""
}

// ChatPlan은 사용자 메시지를 받아 Claude(가능 시) 또는 Codex로 에셋 계획(JSON)을 만듭니다.
// 실제 생성은 프론트엔드가 계획의 각 에셋에 대해 GenerateCharacter/GenerateAsset를 호출합니다.
func (a *App) ChatPlan(history, message string) (ChatPlanResult, error) {
	if strings.TrimSpace(message) == "" {
		return ChatPlanResult{}, errors.New("메시지를 입력해 주세요")
	}
	prompt := buildPlannerPrompt(history, message)

	// 1) Claude CLI 우선 (가능하면) — 헤드리스 -p 모드
	if claude := gen.FindBin("claude"); claude != "claude" {
		ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
		cmd := exec.CommandContext(ctx, claude, "-p", prompt)
		cmd.Stdin = nil
		env := gen.SanitizedAuthEnv() // 구독 OAuth 인증을 깨는 ANTHROPIC_*/CLAUDE_CODE_* 변수 제거
		if tok := strings.TrimSpace(config.Load().ClaudeToken); tok != "" {
			env = append(env, "CLAUDE_CODE_OAUTH_TOKEN="+tok) // 설정에 저장된 구독 토큰 사용
		}
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		cancel()
		if err == nil {
			if js := extractJSONObject(string(out)); js != "" {
				var r ChatPlanResult
				if json.Unmarshal([]byte(js), &r) == nil {
					r.Planner = "claude"
					return r, nil
				}
			}
		}
		// Claude 실패(인증/파싱) → Codex로 폴백
	}

	// 2) Codex CLI 폴백
	codex := gen.CodexBinPath()
	ctx, cancel := context.WithTimeout(a.ctx, 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, codex, "exec", "--skip-git-repo-check", "--sandbox", "read-only", prompt)
	cmd.Stdin = nil
	cmd.Env = gen.AugmentedEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ChatPlanResult{}, fmt.Errorf("기획자 호출 실패(codex): %v", err)
	}
	js := extractJSONObject(string(out))
	if js == "" {
		return ChatPlanResult{Reply: strings.TrimSpace(string(out)), Planner: "codex"}, nil
	}
	var r ChatPlanResult
	if err := json.Unmarshal([]byte(js), &r); err != nil {
		return ChatPlanResult{Reply: strings.TrimSpace(string(out)), Planner: "codex"}, nil
	}
	r.Planner = "codex"
	return r, nil
}

// runPlanner는 claude(가능 시) 또는 codex로 프롬프트를 실행해 응답 텍스트를 반환합니다.
// 두 CLI 모두 stderr로 진단 로그(타임스탬프 ERROR 등)를 쏟으므로, 응답으로는
// stdout(또는 codex의 --output-last-message 파일)만 사용하고 stderr는 오류 진단용으로만 씁니다.
func (a *App) runPlanner(prompt string) (string, error) {
	if claude := gen.FindBin("claude"); claude != "claude" {
		ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
		cmd := exec.CommandContext(ctx, claude, "-p", prompt)
		cmd.Stdin = nil
		env := gen.SanitizedAuthEnv()
		if tok := strings.TrimSpace(config.Load().ClaudeToken); tok != "" {
			env = append(env, "CLAUDE_CODE_OAUTH_TOKEN="+tok)
		}
		cmd.Env = env
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err == nil {
			s := strings.TrimSpace(stdout.String())
			low := strings.ToLower(s)
			if s != "" && !strings.Contains(low, "not logged in") && !strings.Contains(low, "invalid authentication") {
				cancel()
				return s, nil
			}
		}
		cancel()
	}

	// codex: --output-last-message로 최종 답변만 파일로 받음(stdout/stderr의 세션 로그 배제)
	codex := gen.CodexBinPath()
	ctx, cancel := context.WithTimeout(a.ctx, 120*time.Second)
	defer cancel()
	tmp, err := os.CreateTemp("", "ppplan-*.txt")
	if err != nil {
		return "", fmt.Errorf("기획자 임시 파일 생성 실패: %w", err)
	}
	outPath := tmp.Name()
	tmp.Close()
	defer os.Remove(outPath)

	cmd := exec.CommandContext(ctx, codex, "exec", "--skip-git-repo-check",
		"--sandbox", "read-only", "--color", "never",
		"--output-last-message", outPath, prompt)
	cmd.Stdin = nil
	cmd.Env = gen.AugmentedEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = nil
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("기획자 호출 실패: %v", err)
	}
	data, _ := os.ReadFile(outPath)
	s := strings.TrimSpace(string(data))
	if s == "" {
		return "", fmt.Errorf("기획자 응답이 비었습니다: %s", tailString(stderr.String(), 300))
	}
	return s, nil
}

// tailString은 문자열의 마지막 n바이트만 반환합니다(로그 꼬리 노출용).
func tailString(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

// ChoreographArgs는 동작 안무 자동작성 요청입니다.
type ChoreographArgs struct {
	Description string `json:"description"`
	Action      string `json:"action"`
	Frames      int    `json:"frames"`
}

// Choreograph는 캐릭터·동작에 맞춘 비트별 상세 안무를 LLM으로 작성합니다(상세 안무 필드에 채움).
func (a *App) Choreograph(args ChoreographArgs) (string, error) {
	action := strings.TrimSpace(args.Action)
	if action == "" {
		return "", errors.New("동작을 입력해 주세요")
	}
	sys := "You are a 2D game animation director. Write a vivid, beat-by-beat CHOREOGRAPHY for a looping sprite animation of the given action, tailored to this specific character. " +
		"Output ONLY the choreography text — no preamble, no headings, no bullet points, no numbers or digits, and never mention frames, grids, pixels, cells, or sprite sheets. " +
		"Describe body position, weight shift, limb placement and motion arcs as one flowing, looping performance where the last beat hands seamlessly back into the first. " +
		"For locomotion (walk/run/etc.) STRICTLY ALTERNATE left and right legs — state which leg steps forward and which pushes off, with every beat a distinctly different leg configuration and the opposite arm swinging. Keep it to about 3-5 sentences."
	prompt := sys + "\n\nCharacter: " + strings.TrimSpace(args.Description) + "\nAction: " + action + "\n\nChoreography:"
	out, err := a.runPlanner(prompt)
	if err != nil {
		return "", err
	}
	// codex/claude가 머리말을 붙이면 마지막 비어있지 않은 문단만 취함
	out = strings.TrimSpace(out)
	if idx := strings.LastIndex(out, "Choreography:"); idx >= 0 {
		out = strings.TrimSpace(out[idx+len("Choreography:"):])
	}
	return out, nil
}

// ---------- 세션 저장/복원 ----------

// SaveSession은 현재 작업 상태(JSON)를 디스크에 저장합니다 (앱 재시작 시 복원용).
func (a *App) SaveSession(data string) error {
	path, err := config.SessionPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// 임시 파일에 쓴 뒤 교체해 손상 방지
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadSession은 저장된 작업 세션 JSON을 반환합니다 (없으면 빈 문자열).
func (a *App) LoadSession() string {
	path, err := config.SessionPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// ClearSession은 저장된 작업 세션을 삭제합니다.
func (a *App) ClearSession() error {
	path, err := config.SessionPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ---------- 이미지 입출력 헬퍼 ----------

var dataURLRe = regexp.MustCompile(`^data:image/[a-zA-Z+.-]+;base64,`)

func decodeDataURL(dataURL string) ([]byte, error) {
	m := dataURLRe.FindString(dataURL)
	if m == "" {
		return nil, errors.New("올바른 이미지 데이터가 아닙니다")
	}
	return base64.StdEncoding.DecodeString(dataURL[len(m):])
}

func pngDataURL(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func decodeImage(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("이미지 디코딩 실패: %w", err)
	}
	return img, nil
}

// PickImage는 파일 선택 대화상자를 열고 선택된 이미지를 dataURL로 반환합니다.
func (a *App) PickImage() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "베이스 이미지 선택",
		Filters: []runtime.FileFilter{
			{DisplayName: "이미지 (*.png;*.jpg;*.jpeg;*.webp)", Pattern: "*.png;*.jpg;*.jpeg;*.webp"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // 취소
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("파일 읽기 실패: %w", err)
	}
	img, err := decodeImage(data)
	if err != nil {
		return "", err
	}
	return pngDataURL(img)
}

// PixelizeImageArgs는 임의의 입력 이미지를 픽셀아트로 변환하는 요청입니다(AI 불필요).
type PixelizeImageArgs struct {
	DataURL  string `json:"dataURL"`  // 입력 이미지 dataURL
	StyleKey string `json:"styleKey"` // 팔레트 크기 산출용 (pixel/retro16 등)
	Colors   int    `json:"colors"`   // 팔레트 색 수 직접 지정 (0이면 styleKey/기본값 사용)
	RemoveBg bool   `json:"removeBg"` // 배경(테두리 단색/크로마키) 제거 여부
}

// PixelizeImage는 사용자가 고른 이미지를 로컬에서 픽셀아트로 변환합니다(API 키 불필요).
// 배경 제거 → 공유 팔레트 양자화 → 픽셀 그리드 스냅, 즉 생성 파이프라인의 후처리만 재사용합니다.
func (a *App) PixelizeImage(args PixelizeImageArgs) (string, error) {
	raw, err := decodeDataURL(args.DataURL)
	if err != nil {
		return "", fmt.Errorf("이미지 오류: %w", err)
	}
	img, err := decodeImage(raw)
	if err != nil {
		return "", err
	}

	var nrgba *image.NRGBA
	if args.RemoveBg {
		nrgba = sprite.RemoveBackground(img)
	} else {
		nrgba = sprite.ToNRGBA(img)
	}

	// 팔레트 크기 결정: 직접 지정 > 스타일 산출 > 기본 32색
	n := args.Colors
	if n <= 0 {
		n = sprite.PaletteSizeForStyle(args.StyleKey)
	}
	if n <= 0 {
		n = 32
	}
	single := []*image.NRGBA{nrgba}
	sprite.PixelPostProcess(single, n)

	saveGalleryPNG("pixelize-"+galleryStamp(), single[0])
	return pngDataURL(single[0])
}

// ---------- 생성 파이프라인 ----------

// GenerateCharacterArgs는 텍스트 → 베이스 캐릭터 생성 요청입니다.
type GenerateCharacterArgs struct {
	Description string `json:"description"`
	StyleKey    string `json:"styleKey"`
	StyleCustom string `json:"styleCustom"`
	View        string `json:"view"` // front | side | threequarter (빈 값=front)
}

// GenerateCharacter는 설명만으로 베이스 캐릭터 이미지를 생성합니다.
func (a *App) GenerateCharacter(args GenerateCharacterArgs) (string, error) {
	if strings.TrimSpace(args.Description) == "" {
		return "", errors.New("캐릭터 설명을 입력해 주세요")
	}
	style := sprite.ResolveStyle(args.StyleKey, args.StyleCustom)
	prompt := sprite.BuildCharacterPrompt(args.Description, style, args.View)

	p, err := a.provider()
	if err != nil {
		return "", err
	}
	// 종료 시 진행 표시 제거
	defer a.emit("progress", map[string]any{"phase": "idle", "message": ""})

	a.emit("progress", map[string]any{"phase": "character", "message": "캐릭터 생성 중..."})
	genCtx, releaseGen := a.genContext()
	defer releaseGen()
	raw, err := p.GenerateImage(genCtx, prompt, nil, "1:1")
	if err != nil {
		return "", friendlyErr(err)
	}
	img, err := decodeImage(raw)
	if err != nil {
		return "", err
	}
	// 배경 제거된 미리보기 제공
	clean := sprite.RemoveBackground(img)
	if n := sprite.PaletteSizeForStyle(args.StyleKey); n > 0 {
		single := []*image.NRGBA{clean}
		sprite.PixelPostProcess(single, n)
		clean = single[0]
	}
	saveGalleryPNG("character-"+galleryStamp(), clean)
	return pngDataURL(clean)
}

// GenerateAssetArgs는 비캐릭터 에셋(배경/타일/아이템) 생성 요청입니다.
type GenerateAssetArgs struct {
	Kind        string `json:"kind"` // item | background | tile
	Description string `json:"description"`
	StyleKey    string `json:"styleKey"`
	StyleCustom string `json:"styleCustom"`
}

// GenerateAsset는 배경/타일/아이템 등 비캐릭터 스프라이트를 생성합니다.
// 아이템은 배경 제거(투명), 배경/타일은 불투명으로 처리합니다.
func (a *App) GenerateAsset(args GenerateAssetArgs) (string, error) {
	if strings.TrimSpace(args.Description) == "" {
		return "", errors.New("설명을 입력해 주세요")
	}
	style := sprite.ResolveStyle(args.StyleKey, args.StyleCustom)

	var prompt, aspect string
	switch args.Kind {
	case "item":
		prompt, aspect = sprite.BuildItemPrompt(args.Description, style), "1:1"
	case "background":
		prompt, aspect = sprite.BuildBackgroundPrompt(args.Description, style), "16:9"
	case "tile":
		prompt, aspect = sprite.BuildTilePrompt(args.Description, style), "1:1"
	default:
		return "", fmt.Errorf("지원하지 않는 에셋 종류입니다: %s", args.Kind)
	}

	p, err := a.provider()
	if err != nil {
		return "", err
	}
	defer a.emit("progress", map[string]any{"phase": "idle", "message": ""})
	a.emit("progress", map[string]any{"phase": "asset", "message": args.Kind + " 생성 중..."})

	genCtx, releaseGen := a.genContext()
	defer releaseGen()
	raw, err := p.GenerateImage(genCtx, prompt, nil, aspect)
	if err != nil {
		return "", friendlyErr(err)
	}
	img, err := decodeImage(raw)
	if err != nil {
		return "", err
	}

	var out image.Image
	if args.Kind == "item" {
		// 아이템: 마젠타 키잉 → 투명 + (스타일에 맞으면) 픽셀화
		clean := sprite.RemoveBackground(img)
		if n := sprite.PaletteSizeForStyle(args.StyleKey); n > 0 {
			single := []*image.NRGBA{clean}
			sprite.PixelPostProcess(single, n)
			clean = single[0]
		}
		out = clean
	} else {
		// 배경/타일: 불투명 유지, 배경 제거 없이 스타일 팔레트만 적용
		nr := sprite.ToNRGBA(img)
		if n := sprite.PaletteSizeForStyle(args.StyleKey); n > 0 {
			single := []*image.NRGBA{nr}
			sprite.PixelPostProcess(single, n)
			nr = single[0]
		}
		out = nr
	}

	saveGalleryPNG("asset-"+args.Kind+"-"+galleryStamp(), out)
	return pngDataURL(out)
}

// GenerateEditArgs는 기존 이미지를 부분 편집(인페인팅)하는 요청입니다.
type GenerateEditArgs struct {
	Image       string `json:"image"`       // 편집할 원본 dataURL
	Instruction string `json:"instruction"` // 편집 지시 (예: "빨간 망토 추가")
	StyleKey    string `json:"styleKey"`
	StyleCustom string `json:"styleCustom"`
	Transparent bool   `json:"transparent"` // true=캐릭터/아이템(투명), false=배경/타일(불투명)
}

// GenerateEdit는 첨부 이미지에 지시한 변경만 적용한 새 이미지를 생성합니다.
func (a *App) GenerateEdit(args GenerateEditArgs) (string, error) {
	if strings.TrimSpace(args.Instruction) == "" {
		return "", errors.New("편집 지시를 입력해 주세요")
	}
	srcRaw, err := decodeDataURL(args.Image)
	if err != nil {
		return "", fmt.Errorf("이미지 오류: %w", err)
	}
	srcImg, err := decodeImage(srcRaw)
	if err != nil {
		return "", err
	}
	var srcBuf bytes.Buffer
	if err := png.Encode(&srcBuf, srcImg); err != nil {
		return "", err
	}

	style := sprite.ResolveStyle(args.StyleKey, args.StyleCustom)
	prompt := sprite.BuildEditPrompt(args.Instruction, style, args.Transparent)

	p, err := a.provider()
	if err != nil {
		return "", err
	}
	defer a.emit("progress", map[string]any{"phase": "idle", "message": ""})
	a.emit("progress", map[string]any{"phase": "edit", "message": "이미지 편집 중..."})
	genCtx, releaseGen := a.genContext()
	defer releaseGen()

	raw, err := p.GenerateImage(genCtx, prompt, [][]byte{srcBuf.Bytes()}, "1:1")
	if err != nil {
		return "", friendlyErr(err)
	}
	img, err := decodeImage(raw)
	if err != nil {
		return "", err
	}
	var out image.Image
	if args.Transparent {
		clean := sprite.RemoveBackground(img)
		if n := sprite.PaletteSizeForStyle(args.StyleKey); n > 0 {
			single := []*image.NRGBA{clean}
			sprite.PixelPostProcess(single, n)
			clean = single[0]
		}
		out = clean
	} else {
		nr := sprite.ToNRGBA(img)
		if n := sprite.PaletteSizeForStyle(args.StyleKey); n > 0 {
			single := []*image.NRGBA{nr}
			sprite.PixelPostProcess(single, n)
			nr = single[0]
		}
		out = nr
	}
	saveGalleryPNG("edit-"+galleryStamp(), out)
	return pngDataURL(out)
}

// ---------- 스켈레톤 리그 애니메이션 (고성능 경로, AI 불필요·오프라인) ----------

// RigAnimateArgs는 캐릭터 1장을 자동 리깅해 애니메이션 프레임을 만드는 요청입니다.
type RigAnimateArgs struct {
	Image  string `json:"image"`  // 캐릭터 dataURL(투명 배경 권장)
	Anim   string `json:"anim"`   // walk | run | idle
	Frames int    `json:"frames"` // 프레임 수(1~24)
}

// RigAnimateResult는 렌더된 프레임들과 가로 스프라이트 시트입니다.
type RigAnimateResult struct {
	Frames []string `json:"frames"` // 각 프레임 dataURL
	Sheet  string   `json:"sheet"`  // 가로 시트 dataURL
}

// RigAnimations는 사용 가능한 내장 모션 목록을 반환합니다.
func (a *App) RigAnimations() []string {
	return []string{"walk", "run", "idle"}
}

// RigAnimate는 캐릭터를 자동 리깅(부위 분할+스켈레톤)하고 모션을 적용해 프레임을 렌더링합니다.
// AI 생성이 아니라 로컬 스켈레탈 렌더라 키/로그인이 필요 없고 빠릅니다.
func (a *App) RigAnimate(args RigAnimateArgs) (RigAnimateResult, error) {
	var res RigAnimateResult
	raw, err := decodeDataURL(args.Image)
	if err != nil {
		return res, fmt.Errorf("이미지 오류: %w", err)
	}
	img, err := decodeImage(raw)
	if err != nil {
		return res, err
	}
	nr := sprite.ToNRGBA(img)
	sk := rig.AutoRigHumanoid(nr)

	anim := rig.MotionLibrary()[args.Anim]
	if anim == nil {
		anim = rig.MotionLibrary()["walk"]
	}
	n := args.Frames
	if n < 1 {
		n = 8
	}
	if n > 24 {
		n = 24
	}
	w := nr.Rect.Dx()
	h := nr.Rect.Dy() + nr.Rect.Dy()/10 // 다리 스윙 여유

	frames := rig.RenderFrames(sk, anim, n, w, h)
	sheet := image.NewNRGBA(image.Rect(0, 0, w*len(frames), h))
	for i, f := range frames {
		url, err := pngDataURL(f)
		if err != nil {
			return res, err
		}
		res.Frames = append(res.Frames, url)
		// 시트에 합성
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				si := f.PixOffset(x, y)
				if f.Pix[si+3] == 0 {
					continue
				}
				di := sheet.PixOffset(i*w+x, y)
				copy(sheet.Pix[di:di+4], f.Pix[si:si+4])
			}
		}
	}
	if res.Sheet, err = pngDataURL(sheet); err != nil {
		return res, err
	}
	saveGalleryPNG("riganim-"+args.Anim+"-"+galleryStamp(), sheet)
	return res, nil
}

// GenerateCharacterRefArgs는 레퍼런스 이미지 화풍으로 새 캐릭터를 만드는 요청입니다.
type GenerateCharacterRefArgs struct {
	ReferenceImage string `json:"referenceImage"` // 화풍 레퍼런스 dataURL
	Description    string `json:"description"`
	StyleKey       string `json:"styleKey"`
	StyleCustom    string `json:"styleCustom"`
	View           string `json:"view"` // front | side | threequarter
}

// GenerateCharacterRef는 레퍼런스 이미지의 화풍을 따라 설명에 맞는 다른 캐릭터를 생성합니다.
func (a *App) GenerateCharacterRef(args GenerateCharacterRefArgs) (string, error) {
	if strings.TrimSpace(args.Description) == "" {
		return "", errors.New("캐릭터 설명을 입력해 주세요")
	}
	refRaw, err := decodeDataURL(args.ReferenceImage)
	if err != nil {
		return "", fmt.Errorf("레퍼런스 이미지 오류: %w", err)
	}
	refImg, err := decodeImage(refRaw)
	if err != nil {
		return "", err
	}
	var refBuf bytes.Buffer
	if err := png.Encode(&refBuf, refImg); err != nil {
		return "", err
	}

	style := sprite.ResolveStyle(args.StyleKey, args.StyleCustom)
	prompt := sprite.BuildCharacterRefPrompt(args.Description, style, args.View)

	p, err := a.provider()
	if err != nil {
		return "", err
	}
	defer a.emit("progress", map[string]any{"phase": "idle", "message": ""})
	a.emit("progress", map[string]any{"phase": "character", "message": "레퍼런스 캐릭터 생성 중..."})
	genCtx, releaseGen := a.genContext()
	defer releaseGen()

	raw, err := p.GenerateImage(genCtx, prompt, [][]byte{refBuf.Bytes()}, "1:1")
	if err != nil {
		return "", friendlyErr(err)
	}
	img, err := decodeImage(raw)
	if err != nil {
		return "", err
	}
	clean := sprite.RemoveBackground(img)
	if n := sprite.PaletteSizeForStyle(args.StyleKey); n > 0 {
		single := []*image.NRGBA{clean}
		sprite.PixelPostProcess(single, n)
		clean = single[0]
	}
	saveGalleryPNG("variant-"+galleryStamp(), clean)
	return pngDataURL(clean)
}

// GenerateStateArgs는 상태별 스트립 생성 요청입니다.
type GenerateStateArgs struct {
	BaseImage   string           `json:"baseImage"` // dataURL
	Description string           `json:"description"`
	StyleKey    string           `json:"styleKey"`
	StyleCustom string           `json:"styleCustom"`
	CellSize    int              `json:"cellSize"`
	SafeMargin  int              `json:"safeMargin"`
	Feedback    string           `json:"feedback"`
	RefStrip    string           `json:"refStrip"` // 정면(south) 스트립 dataURL — 방향 세트 생성 시 모션 참조용
	Mode        string           `json:"mode"`     // "" = 한 장 스트립(기본), "perpose" = 프레임별 개별 생성(실험)
	State       sprite.StateSpec `json:"state"`
}

// StateResult는 상태 생성 결과입니다.
type StateResult struct {
	Name     string   `json:"name"`
	RawStrip string   `json:"rawStrip"`
	Frames   []string `json:"frames"`
	Expected int      `json:"expected"`
	Found    int      `json:"found"`
	Warnings []string `json:"warnings"`
}

// GenerateState는 한 상태의 스트립을 생성하고 프레임을 추출합니다.
func (a *App) GenerateState(args GenerateStateArgs) (StateResult, error) {
	res := StateResult{Name: args.State.Name, Expected: args.State.Frames}

	if args.State.Frames < 1 || args.State.Frames > 16 {
		return res, errors.New("프레임 수는 1~16 사이여야 합니다")
	}
	if args.Mode == "perpose" {
		return a.generateStatePerPose(args)
	}
	baseRaw, err := decodeDataURL(args.BaseImage)
	if err != nil {
		return res, fmt.Errorf("베이스 이미지 오류: %w", err)
	}
	cellSize := args.CellSize
	if cellSize <= 0 {
		cellSize = 256
	}
	margin := args.SafeMargin
	if margin <= 0 {
		margin = max(8, cellSize/12)
	}

	style := sprite.ResolveStyle(args.StyleKey, args.StyleCustom)
	aspect := sprite.AspectForFrames(args.State.Frames)

	p, err := a.provider()
	if err != nil {
		return res, err
	}

	// 베이스 캐릭터 정체성 검사용 (투명 배경일 때만 InspectFrames가 사용).
	// 뒷면 계열 방향은 정면 베이스와 색 구성이 달라 오탐하므로 검사를 건너뜁니다.
	var baseN *image.NRGBA
	if !sprite.IsBackFacing(args.State.Facing) {
		if bimg, err := decodeImage(baseRaw); err == nil {
			baseN = sprite.ToNRGBA(bimg)
		}
	}

	// 생성 참조 이미지: 베이스 캐릭터 + (선택) 정면 스트립
	refs := [][]byte{baseRaw}
	if strings.TrimSpace(args.RefStrip) != "" {
		if refRaw, err := decodeDataURL(args.RefStrip); err == nil {
			refs = append(refs, refRaw)
		}
	}

	// 완벽한 프레임 수가 나올 때까지 자동 재시도 (최대 3회)
	const maxAttempts = 3
	expected := args.State.Frames
	feedback := args.Feedback
	genCtx, releaseGen := a.genContext()
	defer releaseGen()
	var best StateResult
	var bestImgs []*image.NRGBA
	bestScore := -1 << 30
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		prompt := sprite.BuildStripPrompt(args.Description, style, args.State, feedback)
		if len(refs) > 1 {
			prompt += "\nMotion reference: the second attached image is the FRONT-view animation strip of this same character performing this exact action. Reproduce the same motion timing and pose phases frame by frame, but viewed from the required facing direction above.\n"
		}

		msg := "AI 프레임 생성 중..."
		if attempt > 1 {
			msg = fmt.Sprintf("프레임 수 보정 재생성 중... (%d/%d)", attempt, maxAttempts)
		}
		a.emit("progress", map[string]any{"phase": "generate", "state": args.State.Name, "message": msg})

		stripRaw, err := p.GenerateImage(genCtx, prompt, refs, aspect)
		if err != nil {
			lastErr = friendlyErr(err)
			break // API 오류/취소는 재시도 의미 없음 (클라이언트가 자체 재시도함)
		}

		a.emit("progress", map[string]any{"phase": "extract", "state": args.State.Name, "message": "배경 제거 및 프레임 추출 중..."})
		stripImg, err := decodeImage(stripRaw)
		if err != nil {
			lastErr = err
			continue
		}
		nimg := sprite.ToNRGBA(stripImg)
		bgKey := sprite.DetectBackground(nimg)
		clean := sprite.RemoveBackground(nimg)

		cand := StateResult{Name: args.State.Name, Expected: expected}
		if rawURL, err := pngDataURL(clean); err == nil {
			cand.RawStrip = rawURL
		}
		extracted := sprite.ExtractFrames(clean, expected, cellSize, cellSize, margin)
		// 프레임별 품질 검사는 양자화 전 원본 프레임에 수행 (양자화로 미세한
		// 정체성 차이가 뭉개지면 drift 감지 민감도가 떨어짐)
		insp := sprite.InspectFrames(extracted.Frames, bgKey, baseN)
		// 픽셀아트 스타일이면 공유 팔레트 양자화 + 픽셀 그리드 스냅으로 "진짜" 픽셀아트화
		sprite.PixelPostProcess(extracted.Frames, sprite.PaletteSizeForStyle(args.StyleKey))
		cand.Found = extracted.Found
		cand.Warnings = extracted.Warnings
		for _, f := range extracted.Frames {
			u, err := pngDataURL(f)
			if err != nil {
				return res, err
			}
			cand.Frames = append(cand.Frames, u)
		}
		cand.Warnings = append(cand.Warnings, insp.Errors...)
		cand.Warnings = append(cand.Warnings, insp.Warnings...)
		// 인접 프레임 변화가 거의 없으면(사실상 정지) 비차단 경고.
		// 임계값 0.01은 실측 기반: 의도적으로 미동이 적은 meditate(~1.5%)도 통과시키고
		// 0.01 미만, 즉 프레임이 사실상 동일한 "애니메이션이 전혀 움직이지 않는" 결함만 잡습니다.
		if cand.Found >= 2 && sprite.MotionPresence(extracted.Frames) < 0.01 {
			cand.Warnings = append(cand.Warnings,
				"프레임 간 움직임이 거의 없습니다. 동작이 더 분명하게 드러나도록 동작 설명을 보강해 재생성하는 것을 권장합니다.")
		}
		errCount := len(insp.Errors)
		// 인접 중복(포즈 안 바뀐) 프레임 감지 — 걷기 등에서 같은 포즈가 연속되면 그 지점에서 버벅임
		dupPairs := 0
		if cand.Found >= 2 {
			dupPairs = sprite.AdjacentDupPairs(extracted.Frames, 1)
		}
		if dupPairs > 0 {
			cand.Warnings = append(cand.Warnings,
				fmt.Sprintf("거의 같은 포즈가 연속된 프레임이 %d쌍 있습니다(그 지점에서 버벅임). 재생성을 권장합니다.", dupPairs))
		}

		// 프레임 수가 정확하고 심각한 품질 문제도 중복도 없으면 즉시 성공
		if cand.Found == expected && insp.Ok() && dupPairs == 0 {
			saveGalleryFrames(args.State.Name, extracted.Frames)
			return cand, nil
		}
		// 최선 후보 갱신: 프레임 수 우선, 같으면 오류·중복 적은 쪽
		score := cand.Found*100 - errCount*10 - dupPairs*5
		if score > bestScore {
			best, bestScore, bestImgs = cand, score, extracted.Frames
		}
		lastErr = nil

		// 다음 시도용 보정 피드백 (사용자 피드백 유지 + 측정 기반 보정 지시)
		var fixes []string
		if cand.Found != expected {
			fixes = append(fixes, fmt.Sprintf(
				"IMPORTANT CORRECTION: the last attempt read as %d poses but EXACTLY %d are required. Redraw as one horizontal row of %d equally sized poses, one clearly separated pose per column, each ringed by a clean magenta gap so none touch or overlap. Do not draw any frame, border, or film strip.",
				cand.Found, expected, expected))
		}
		if len(insp.RetryHints) > 0 {
			fixes = append(fixes, "QUALITY CORRECTIONS detected by automated inspection (fix all of these):")
			fixes = append(fixes, insp.RetryHints...)
		}
		if dupPairs > 0 {
			fixes = append(fixes, fmt.Sprintf(
				"DUPLICATE POSES: %d adjacent frame pair(s) show the SAME pose — this causes a stutter. Make EVERY pose visibly different from its neighbours; never hold or repeat a pose across two adjacent frames. Each frame must advance the motion by a clear, even step.", dupPairs))
		}
		auto := strings.Join(fixes, "\n")
		if args.Feedback != "" {
			feedback = args.Feedback + "\n" + auto
		} else {
			feedback = auto
		}

		// 마지막 시도 직전 메시지 갱신용: 품질 보정 재시도임을 표시
		if cand.Found == expected && errCount > 0 && attempt < maxAttempts {
			a.emit("progress", map[string]any{"phase": "generate", "state": args.State.Name,
				"message": fmt.Sprintf("품질 보정 재생성 중... (%d/%d)", attempt+1, maxAttempts)})
		}
	}

	if best.Found == 0 {
		if lastErr != nil {
			return res, lastErr
		}
		return res, errors.New("스프라이트 프레임을 추출하지 못했습니다. 캐릭터 설명을 더 구체적으로 작성해 보세요")
	}
	if best.Found != expected {
		best.Warnings = append(best.Warnings,
			fmt.Sprintf("자동 재시도 후에도 프레임 수가 다릅니다 (요청 %d개 → 추출 %d개). 가장 근접한 결과를 표시합니다.", expected, best.Found))
	} else {
		best.Warnings = append(best.Warnings,
			"자동 재시도 후에도 일부 품질 문제가 남아 있습니다. 프레임을 확인하고 필요하면 피드백과 함께 재생성해 주세요.")
	}
	saveGalleryFrames(args.State.Name, bestImgs)
	return best, nil
}

// generateStatePerPose는 프레임을 한 장씩 풀해상도로 개별 생성합니다(실험 모드).
// 각 프레임: 베이스 캐릭터 + (직전 프레임) 레퍼런스로 한 단계 진행된 단일 포즈를 생성 →
// 배경 제거 → 콘텐츠 추출. N장을 모아 공유 스케일/공통 베이스라인으로 정렬(ExtractFramesIndividual)
// 하므로 칸 쪼개기 없이 발 잘림이 없고 프레임당 해상도가 보존됩니다.
func (a *App) generateStatePerPose(args GenerateStateArgs) (StateResult, error) {
	res := StateResult{Name: args.State.Name, Expected: args.State.Frames}
	baseRaw, err := decodeDataURL(args.BaseImage)
	if err != nil {
		return res, fmt.Errorf("베이스 이미지 오류: %w", err)
	}
	cellSize := args.CellSize
	if cellSize <= 0 {
		cellSize = 256
	}
	margin := args.SafeMargin
	if margin <= 0 {
		margin = max(8, cellSize/12)
	}
	style := sprite.ResolveStyle(args.StyleKey, args.StyleCustom)
	n := args.State.Frames

	var baseN *image.NRGBA
	if !sprite.IsBackFacing(args.State.Facing) {
		if bimg, err := decodeImage(baseRaw); err == nil {
			baseN = sprite.ToNRGBA(bimg)
		}
	}

	p, err := a.provider()
	if err != nil {
		return res, err
	}
	genCtx, releaseGen := a.genContext()
	defer releaseGen()

	cleans := make([]*image.NRGBA, 0, n)
	var prevPNG []byte // 직전 프레임(투명 제거 전 원본 생성물) — 일관성 레퍼런스
	var bgKey [3]uint8
	bgSet := false

	for i := 0; i < n; i++ {
		a.emit("progress", map[string]any{"phase": "generate", "state": args.State.Name,
			"message": fmt.Sprintf("프레임별 생성 중... (%d/%d)", i+1, n)})

		refs := [][]byte{baseRaw}
		if prevPNG != nil {
			refs = append(refs, prevPNG)
		}
		prompt := sprite.BuildPosePrompt(args.Description, style, args.State, i, n, prevPNG != nil, args.Feedback)

		// 빈 추출이면 1회 재시도
		var clean *image.NRGBA
		var rawPNG []byte
		for attempt := 0; attempt < 2; attempt++ {
			poseRaw, gerr := p.GenerateImage(genCtx, prompt, refs, "1:1")
			if gerr != nil {
				return res, friendlyErr(gerr)
			}
			poseImg, derr := decodeImage(poseRaw)
			if derr != nil {
				continue
			}
			nimg := sprite.ToNRGBA(poseImg)
			if !bgSet {
				bgKey = sprite.DetectBackground(nimg)
				bgSet = true
			}
			clean = sprite.RemoveBackground(nimg)
			rawPNG = poseRaw
			break
		}
		if clean == nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("프레임 %d 생성 실패(건너뜀)", i+1))
			continue
		}
		cleans = append(cleans, clean)
		prevPNG = rawPNG // 다음 프레임의 진행 레퍼런스로 원본 생성물 사용
	}

	if len(cleans) == 0 {
		return res, errors.New("프레임을 한 장도 생성하지 못했습니다. 다시 시도해 주세요")
	}

	a.emit("progress", map[string]any{"phase": "extract", "state": args.State.Name,
		"message": "프레임 정렬 및 추출 중..."})
	extracted := sprite.ExtractFramesIndividual(cleans, cellSize, cellSize, margin)
	insp := sprite.InspectFrames(extracted.Frames, bgKey, baseN)
	sprite.PixelPostProcess(extracted.Frames, sprite.PaletteSizeForStyle(args.StyleKey))

	res.Found = extracted.Found
	res.Warnings = append(res.Warnings, extracted.Warnings...)
	for _, f := range extracted.Frames {
		u, err := pngDataURL(f)
		if err != nil {
			return res, err
		}
		res.Frames = append(res.Frames, u)
	}
	res.Warnings = append(res.Warnings, insp.Warnings...)
	if dup := sprite.AdjacentDupPairs(extracted.Frames, 1); dup > 0 {
		res.Warnings = append(res.Warnings,
			fmt.Sprintf("거의 같은 포즈가 연속된 프레임이 %d쌍 있습니다. 해당 프레임만 다시 생성하거나 순서를 조정해 보세요.", dup))
	}
	if res.Found != n {
		res.Warnings = append(res.Warnings,
			fmt.Sprintf("요청 %d개 중 %d개 프레임이 추출되었습니다.", n, res.Found))
	}
	saveGalleryFrames(args.State.Name, extracted.Frames)
	return res, nil
}

// ---------- 8방향 세트 ----------

// ListDirections는 8방향 메타데이터 목록을 반환합니다 (3x3 그리드 순서).
func (a *App) ListDirections() []sprite.DirectionInfo {
	return sprite.ListDirections()
}

// ListPresets는 100개 상황 키워드 카탈로그를 반환합니다 (프리셋 선택 UI용).
func (a *App) ListPresets() []sprite.PresetInfo {
	return sprite.ListPresets()
}

// MirrorFrames는 프레임들을 좌우 반전합니다 (east→west 등 미러 방향 생성용).
func (a *App) MirrorFrames(frames []string) ([]string, error) {
	out := make([]string, 0, len(frames))
	for i, fu := range frames {
		raw, err := decodeDataURL(fu)
		if err != nil {
			return nil, fmt.Errorf("프레임 %d 디코딩 실패: %w", i+1, err)
		}
		img, err := decodeImage(raw)
		if err != nil {
			return nil, err
		}
		u, err := pngDataURL(sprite.MirrorNRGBA(sprite.ToNRGBA(img)))
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

// ---------- 내보내기 ----------

// ExportState는 내보내기용 상태 데이터입니다.
type ExportState struct {
	Name   string   `json:"name"`
	FPS    int      `json:"fps"`
	Loop   bool     `json:"loop"`
	Frames []string `json:"frames"` // 선택/정렬된 dataURL
}

// ExportArgs는 프로젝트 내보내기 요청입니다.
type ExportArgs struct {
	Character string        `json:"character"`
	CellSize  int           `json:"cellSize"`
	States    []ExportState `json:"states"`
}

// ExportProject는 디렉토리를 선택받아 스프라이트시트/매니페스트/GIF/프레임을 저장합니다.
func (a *App) ExportProject(args ExportArgs) (string, error) {
	if len(args.States) == 0 {
		return "", errors.New("내보낼 애니메이션이 없습니다")
	}
	cellSize := args.CellSize
	if cellSize <= 0 {
		cellSize = 256
	}

	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "내보낼 폴더 선택",
	})
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", nil // 취소
	}

	charName := strings.TrimSpace(args.Character)
	if charName == "" {
		charName = "character"
	}
	safeName := sanitizeName(charName)
	outDir := filepath.Join(dir, safeName)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}

	defer a.emit("progress", map[string]any{"phase": "idle", "message": ""})
	a.emit("progress", map[string]any{"phase": "export", "message": "스프라이트시트·GIF 내보내는 중..."})

	// 상태 프레임 디코딩
	var stateFrames []sprite.StateFrames
	for _, st := range args.States {
		if len(st.Frames) == 0 {
			continue
		}
		sf := sprite.StateFrames{
			Spec: sprite.StateSpec{Name: st.Name, Frames: len(st.Frames), FPS: st.FPS, Loop: st.Loop},
		}
		for _, fu := range st.Frames {
			raw, err := decodeDataURL(fu)
			if err != nil {
				return "", fmt.Errorf("%s 프레임 디코딩 실패: %w", st.Name, err)
			}
			img, err := decodeImage(raw)
			if err != nil {
				return "", err
			}
			sf.Frames = append(sf.Frames, sprite.ToNRGBA(img))
		}
		stateFrames = append(stateFrames, sf)
	}
	if len(stateFrames) == 0 {
		return "", errors.New("내보낼 프레임이 없습니다")
	}

	// 파일명 프리픽스: 캐릭터별 산출물이 섞여도 구분되도록 모든 파일 앞에 캐릭터 이름을 붙인다.
	prefix := safeName + "-"

	// 1) 스프라이트시트 + 매니페스트
	sheet, manifest := sprite.ComposeAtlas(safeName, stateFrames, cellSize, cellSize)
	if err := writePNG(filepath.Join(outDir, prefix+"sprite-sheet.png"), sheet); err != nil {
		return "", err
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(outDir, prefix+"manifest.json"), manifestJSON, 0o644); err != nil {
		return "", err
	}
	// Aseprite 호환 JSON (Phaser/Unity/Godot 임포터용)
	aseJSON, err := sprite.BuildAsepriteJSON(manifest)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(outDir, prefix+"sprite-sheet.json"), aseJSON, 0o644); err != nil {
		return "", err
	}
	// Godot 4 SpriteFrames(.tres) — 고도에 바로 드래그 임포트
	godotTres := sprite.BuildGodotTres(manifest, prefix+"sprite-sheet.png")
	if err := os.WriteFile(filepath.Join(outDir, prefix+"sprite-frames.tres"), godotTres, 0o644); err != nil {
		return "", err
	}
	// TexturePacker(JSON hash) — Unity TexturePacker importer / Phaser atlasHash / PixiJS
	tpJSON, err := sprite.BuildTexturePackerJSON(manifest)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(outDir, prefix+"texturepacker.json"), tpJSON, 0o644); err != nil {
		return "", err
	}

	// 2) 상태별 GIF + 개별 프레임 PNG
	for _, sf := range stateFrames {
		stateDir := filepath.Join(outDir, "frames", sanitizeName(sf.Spec.Name))
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			return "", err
		}
		for i, frame := range sf.Frames {
			if err := writePNG(filepath.Join(stateDir, fmt.Sprintf("%sframe-%02d.png", prefix, i)), frame); err != nil {
				return "", err
			}
		}
		stateName := sanitizeName(sf.Spec.Name)
		gifBytes, err := sprite.EncodeGIF(sf.Frames, sf.Spec.FPS, sf.Spec.Loop)
		if err != nil {
			return "", fmt.Errorf("%s GIF 인코딩 실패: %w", sf.Spec.Name, err)
		}
		gifDir := filepath.Join(outDir, "gif")
		if err := os.MkdirAll(gifDir, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(gifDir, prefix+stateName+".gif"), gifBytes, 0o644); err != nil {
			return "", err
		}
		// APNG: 풀 알파 지원 (GIF의 1-bit 투명도 한계 보완)
		apngBytes, err := sprite.EncodeAPNG(sf.Frames, sf.Spec.FPS, sf.Spec.Loop)
		if err != nil {
			return "", fmt.Errorf("%s APNG 인코딩 실패: %w", sf.Spec.Name, err)
		}
		apngDir := filepath.Join(outDir, "apng")
		if err := os.MkdirAll(apngDir, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(apngDir, prefix+stateName+".png"), apngBytes, 0o644); err != nil {
			return "", err
		}
	}

	return outDir, nil
}

// RevealInFinder는 내보낸 폴더를 파일 탐색기에서 엽니다.
func (a *App) RevealInFinder(path string) {
	runtime.BrowserOpenURL(a.ctx, "file://"+path)
}

func writePNG(path string, img image.Image) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

var unsafeNameRe = regexp.MustCompile(`[^a-zA-Z0-9가-힣_-]+`)

func sanitizeName(name string) string {
	s := unsafeNameRe.ReplaceAllString(strings.TrimSpace(name), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "character"
	}
	return s
}
