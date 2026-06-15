package sprite

import (
	"fmt"
	"strings"
)

// StylePresets는 선택 가능한 스타일 계약 모음입니다.
var StylePresets = map[string]string{
	"pixel": "low-resolution pixel-art game sprite, chunky readable silhouette, " +
		"thick dark 1-2px outline, visible stepped pixel edges, limited color palette, " +
		"flat cel shading with at most one highlight step and one shadow step, " +
		"simple readable face, clear separated limbs. " +
		"Never use painterly rendering, soft gradients, glossy lighting, anti-aliased fine detail, or 3D rendering.",
	"chibi": "cute chibi game sprite with oversized head and small body, " +
		"bold dark outline, flat bright colors, minimal shading, large expressive eyes, " +
		"clean cartoon shapes readable at small size. " +
		"Never use realistic proportions, gradients, or painterly detail.",
	"cartoon": "clean 2D cartoon game sprite, bold uniform outline, flat vivid colors, " +
		"simple two-tone cel shading, smooth rounded shapes, expressive but simple face. " +
		"Never use pixelation, gradients, photo textures, or 3D rendering.",
	"retro16": "16-bit retro console era game sprite, restrained palette of 16-24 colors, " +
		"dark outline, dithering only where needed, compact proportions, " +
		"crisp hard pixel edges like a classic arcade fighter sprite. " +
		"Never use modern smooth shading or high-resolution detail.",
}

// keyColorPhrase는 키잉 배경 묘사 문구입니다 (매팅이 분리하는 색).
const keyColorPhrase = "pure keying magenta (#FF00FF), perfectly uniform edge to edge"

// ResolveStyle은 프리셋 키 또는 커스텀 스타일 텍스트를 스타일 계약으로 변환합니다.
func ResolveStyle(presetKey, custom string) string {
	if strings.TrimSpace(custom) != "" {
		return strings.TrimSpace(custom)
	}
	if s, ok := StylePresets[presetKey]; ok {
		return s
	}
	return StylePresets["pixel"]
}

// canvasContract는 키잉 캔버스 규칙을 반환합니다 (매팅 단계가 의존하는 핵심 계약).
func canvasContract() string {
	var b strings.Builder
	b.WriteString("Keying canvas (the renderer mattes this away — obey exactly):\n")
	b.WriteString("- Fill the ENTIRE background, edge to edge, with " + keyColorPhrase + " — a single flat color touching all four image borders. No gradient, texture, scenery, floor, panel, frame, or border of any kind.\n")
	b.WriteString("- The subject must avoid magenta, pink and purple entirely — clothing, props, highlights and effects included — so the keyer never eats part of the character.\n")
	b.WriteString("- Drop every shadow and contact patch; the ground is implied, never painted.\n")
	return b.String()
}

// rejectClause는 추출을 방해하는 요소를 거부하는 간결한 계약입니다.
func rejectClause() string {
	var b strings.Builder
	b.WriteString("Reject (these break automatic extraction):\n")
	b.WriteString("- ANY frame, border, or decoration around the image or around a pose: no film strip, no sprocket holes or perforations, no photo/polaroid frame, no panel dividers, no outline box, no vignette. The background reaches every edge unbroken.\n")
	b.WriteString("- Motion garnish — streaks, speed lines, blur, after-images, arcs, swooshes, trails.\n")
	b.WriteString("- Free-floating bits — sparkles, stars, dust, smoke puffs, icons, symbols, or any mark not fused to the body.\n")
	b.WriteString("- Text, numbers, captions, grids, rulers, speech or thought bubbles, UI, watermarks.\n")
	b.WriteString("- Any pose that is clipped by the edge, or whose pixels bridge into the neighbouring pose.\n")
	return b.String()
}

// viewClause는 시점(정면/측면/¾)별 자세·구도 지시를 반환합니다.
// side(측면)는 횡스크롤 걷기/달리기 애니메이션에 맞는 옆모습 캐릭터를 만듭니다.
func viewClause(view string) (opening, framing string) {
	switch view {
	case "side":
		return "Produce one complete game-character reference sprite as a STRICT SIDE PROFILE facing RIGHT (a side-scroller side view).",
			"- Pure side view: the camera is exactly to the character's side; we see one eye, the profile of the face, and the body from the side. Near arm/leg overlap the far ones — do not splay them out.\n" +
				"- Relaxed standing side stance, head to feet, vertically centered, ~3/4 of canvas height, breathing room all sides.\n" +
				"- One continuous silhouette facing right — nothing detached.\n\n"
	case "threequarter":
		return "Produce one complete game-character reference sprite in a 3/4 front view (turned slightly to one side), relaxed standing pose.",
			"- 3/4 view: front-ish but angled, showing some side depth.\n" +
				"- Head to feet, vertically centered, ~3/4 of canvas height, breathing room all sides.\n" +
				"- One continuous silhouette — nothing detached.\n\n"
	default: // front
		return "Produce one complete game-character reference sprite in a relaxed, front-facing standing pose.",
			"- A single figure, head to feet, vertically centered, occupying about three quarters of the canvas height with generous breathing room on every side.\n" +
				"- Symmetric A-pose: arms eased away from the torso, feet level and shoulder-width, weight balanced.\n" +
				"- One continuous silhouette — nothing detached, no trailing accessories or particles.\n\n"
	}
}

// BuildCharacterPrompt는 텍스트 설명 → 베이스 캐릭터 이미지 생성 프롬프트를 만듭니다.
// view: "front"(기본) | "side"(횡스크롤 옆모습) | "threequarter".
func BuildCharacterPrompt(description, style, view string) string {
	opening, framing := viewClause(view)
	var b strings.Builder
	b.WriteString(opening + "\n\n")
	fmt.Fprintf(&b, "Subject: %s.\n\n", strings.TrimSpace(description))
	fmt.Fprintf(&b, "Render contract (obey strictly): %s\n\n", style)
	b.WriteString("Framing:\n")
	b.WriteString(framing)
	b.WriteString(canvasContract())
	return b.String()
}

// BuildEditPrompt는 첨부 이미지를 부분 편집(인페인팅 성격)하는 프롬프트입니다.
// 원본 이미지는 refImages[0]로 전달되어야 합니다.
func BuildEditPrompt(instruction, style string, transparent bool) string {
	var b strings.Builder
	b.WriteString("Edit the attached image. Apply ONLY this change: ")
	b.WriteString(strings.TrimSpace(instruction))
	b.WriteString(".\nKeep EVERYTHING else identical — same subject identity, same pose and composition, same framing, same art style, same color palette and pixel density. Do not redraw, restyle, recolor, or move any unrelated part.\n\n")
	fmt.Fprintf(&b, "Render contract (obey strictly): %s\n\n", style)
	if transparent {
		b.WriteString(canvasContract())
	} else {
		b.WriteString("Opaque output: fill every pixel, no transparency. No text, no border, no frame, no watermark.\n")
	}
	return b.String()
}

// BuildCharacterRefPrompt는 레퍼런스 이미지의 화풍을 따라 "다른" 캐릭터를 만드는 프롬프트입니다.
// 레퍼런스 이미지는 refImages[0]로 함께 전달되어야 합니다.
func BuildCharacterRefPrompt(description, style, view string) string {
	opening, framing := viewClause(view)
	var b strings.Builder
	b.WriteString(opening + "\n\n")
	b.WriteString("Style reference (top priority): The attached image is a STYLE reference, NOT the subject. ")
	b.WriteString("Match its art style EXACTLY — pixel density, outline weight, shading steps, color palette range, level of detail and overall proportions/scale. ")
	b.WriteString("But draw a DIFFERENT character as described below; do NOT copy the reference's species, identity, colors-as-meaning, or silhouette.\n\n")
	fmt.Fprintf(&b, "Subject (the new character to draw): %s.\n\n", strings.TrimSpace(description))
	fmt.Fprintf(&b, "Render contract (obey strictly): %s\n\n", style)
	b.WriteString("Framing:\n")
	b.WriteString(framing)
	b.WriteString(canvasContract())
	return b.String()
}

// BuildItemPrompt는 단일 아이템/오브젝트(무기·물약·코인 등) 스프라이트 프롬프트를 만듭니다.
// 캐릭터가 아니라 사물이며, 매팅을 위해 마젠타 배경을 채웁니다(결과는 투명).
func BuildItemPrompt(description, style string) string {
	var b strings.Builder
	b.WriteString("Produce one single game ITEM/OBJECT sprite (a prop, not a character).\n\n")
	fmt.Fprintf(&b, "Subject: %s.\n\n", strings.TrimSpace(description))
	fmt.Fprintf(&b, "Render contract (obey strictly): %s\n\n", style)
	b.WriteString("Framing:\n")
	b.WriteString("- A single object, centered, occupying about two thirds of the canvas with even margins.\n")
	b.WriteString("- One connected object as a clean icon-like sprite. No character, no hands holding it, no scene.\n\n")
	b.WriteString(canvasContract())
	b.WriteString("\n")
	b.WriteString(rejectClause())
	return b.String()
}

// BuildBackgroundPrompt는 게임 배경/씬 한 장(불투명, 와이드) 프롬프트를 만듭니다.
// 키잉/투명 없음 — 가장자리까지 꽉 찬 불투명 이미지.
func BuildBackgroundPrompt(description, style string) string {
	var b strings.Builder
	b.WriteString("Produce one complete game BACKGROUND / scene illustration, filling the entire frame edge to edge.\n\n")
	fmt.Fprintf(&b, "Scene: %s.\n\n", strings.TrimSpace(description))
	fmt.Fprintf(&b, "Render contract (obey strictly): %s\n\n", style)
	b.WriteString("Rules:\n")
	b.WriteString("- This is an OPAQUE background — fill every pixel, no transparency, no alpha, no checkerboard.\n")
	b.WriteString("- Side-scroller friendly composition: clear sky/upper area and a ground/horizon band, with parallax depth (far, mid, near layers).\n")
	b.WriteString("- No characters, no UI, no text, no watermark, no frame or border. Just the environment.\n")
	return b.String()
}

// BuildTilePrompt는 이음새가 맞는(seamless) 정사각 타일 프롬프트를 만듭니다.
func BuildTilePrompt(description, style string) string {
	var b strings.Builder
	b.WriteString("Produce one SEAMLESS, perfectly TILEABLE square terrain/material tile for a 2D game.\n\n")
	fmt.Fprintf(&b, "Material: %s.\n\n", strings.TrimSpace(description))
	fmt.Fprintf(&b, "Render contract (obey strictly): %s\n\n", style)
	b.WriteString("Tiling rules (critical):\n")
	b.WriteString("- The pattern must wrap seamlessly: the left edge continues into the right edge, and the top edge into the bottom edge, with NO visible seam when repeated in a grid.\n")
	b.WriteString("- Even, repeating texture across the whole square. No single focal object, no character, no scene, no lighting gradient, no vignette.\n")
	b.WriteString("- Opaque, fill every pixel. No text, no border, no frame, no watermark.\n")
	return b.String()
}

// BuildStripPrompt는 상태별 가로 스트립 생성 프롬프트를 만듭니다.
func BuildStripPrompt(description, style string, spec StateSpec, feedback string) string {
	var b strings.Builder
	n := spec.Frames

	fmt.Fprintf(&b, "Draw a single horizontal row of exactly %d game-sprite poses of one character for the \"%s\" animation, ordered left to right. This is raw sprite art, not a photo or a film — draw only the character poses on a flat background.\n\n", n, spec.Name)

	b.WriteString("Subject lock (top priority):\n")
	b.WriteString("- The FIRST attached image is the canonical character — treat it as the single source of truth. Copy its identity pixel-for-pixel: silhouette, proportions, face, hairstyle, build, outfit, accessories. The ONLY thing allowed to change between frames is the body pose; the character itself must look like the exact same drawing in every frame.\n")
	b.WriteString("- The attached image is the canonical character. Match it exactly across every pose: face, hairstyle, build, outfit, accessories.\n")
	b.WriteString("- Palette is binding. Re-sample each region's hue, saturation and value from the reference — skin, hair, every garment, every piece of gear. Do not re-tint, re-light, brighten, darken, or substitute a similar shade.\n")
	b.WriteString("- Hold one fixed camera and facing. The figure never rotates, mirrors, ages, or restyles between poses — only the body moves.\n\n")

	if d := strings.TrimSpace(description); d != "" {
		fmt.Fprintf(&b, "Subject notes: %s.\n\n", d)
	}
	fmt.Fprintf(&b, "Render contract (obey strictly): %s\n\n", style)

	if sec := FacingPromptSection(spec.Facing); sec != "" {
		b.WriteString(sec)
		b.WriteString("\n")
	}

	action := strings.TrimSpace(spec.Action)
	if action == "" {
		action = spec.Name
	}
	fmt.Fprintf(&b, "Movement: %s.\n", action)
	hint := strings.TrimSpace(spec.Choreography) // 사용자가 편집한 안무 우선
	if hint == "" {
		hint = MotionHint(spec.Name)
	}
	if hint != "" {
		fmt.Fprintf(&b, "Choreography: %s\n", hint)
	}
	fmt.Fprintf(&b, "Treat the %d poses as evenly timed beats of one continuous motion — pose k is phase k of %d, and neighbours read as smooth in-betweens, never unrelated stances.\n", n, n)

	// 보편 애니메이션 원칙 — 모든 동작이 자연스럽게 읽히도록 (12원칙 요약)
	b.WriteString("Animation craft (make the motion read naturally, apply to every pose):\n")
	b.WriteString("- Anticipation: before the main action, a small opposite wind-up (load before a throw, dip before a jump).\n")
	b.WriteString("- Clear key pose: one frame holds the most extreme, readable pose of the action; the others build toward and away from it.\n")
	b.WriteString("- Follow-through & overlapping action: loose parts (hair, cloth, tail, weapon) lag and trail the body, settling a beat later, not frozen stiff.\n")
	b.WriteString("- Weight & balance: shift the center of mass and counter-pose the body so it never looks weightless or sliding; feet plant believably.\n")
	b.WriteString("- Arcs: limbs and the body travel along curved arcs between poses, not straight robotic lines.\n")
	b.WriteString("- Ease & spacing: cluster poses tighter at the slow start/end and spread them wider through the fast middle, so speed is felt.\n")
	if spec.Loop {
		b.WriteString("It loops SEAMLESSLY: every pose must be DIFFERENT — do NOT repeat or near-duplicate the first pose as the last frame, and never hold the same pose across two adjacent frames. The last pose is the in-between one step BEFORE the first, so wrapping from last to first is a single smooth step with no held/double frame and no hitch anywhere in the cycle.\n\n")
	} else {
		b.WriteString("It plays once: give it a clear anticipation, a peak, and a settle that comes to rest.\n\n")
	}

	b.WriteString("Sprite-sheet grid (place each pose by an explicit grid — this is how it is cut later, obey exactly):\n")
	fmt.Fprintf(&b, "- Divide the canvas into exactly %d equal-width vertical cells side by side (a 1×%d grid), each the same size. Draw exactly one pose per cell — %d poses total, no more, no fewer. Count them before finishing.\n", n, n, n)
	b.WriteString("- CENTER each pose inside its own cell: the body's center of mass sits on that cell's vertical centre line, with equal empty margin on the left and right of the pose within the cell.\n")
	b.WriteString("- Each pose stays FULLY inside its own cell with clear empty margins on all four sides; nothing — limb, foot, hair, cape, weapon, effect — crosses a cell boundary into a neighbour. If a pose would reach over, scale the whole figure down so it fits with margin.\n")
	b.WriteString("- Every pose is the SAME size at one shared scale, each filling about 70-80% of the cell height. No pose larger, smaller, or set further back than the others.\n")
	b.WriteString("- Each pose is ONE whole, connected body — never split into separate pieces; poses never touch, overlap, or merge.\n")
	b.WriteString("- All poses stand on one common ground baseline near the bottom of the cells, unless the action leaves the ground (a jump), then vary height to show the arc.\n\n")

	b.WriteString(canvasContract())
	b.WriteString("\n")
	b.WriteString(rejectClause())
	b.WriteString("- Favor changes of pose, weight and expression over decoration; any effect must be opaque, hard-edged, and fused to the body.\n")
	b.WriteString("- Keep every pose legible at thumbnail size: bold silhouette, clear limbs, no detail that vanishes when shrunk.\n")

	if f := strings.TrimSpace(feedback); f != "" {
		fmt.Fprintf(&b, "\nArtist revision (apply over everything above): %s\n", f)
	}
	return b.String()
}

// AspectForFrames는 프레임 수에 맞는 생성 종횡비를 고릅니다.
func AspectForFrames(frames int) string {
	switch {
	case frames <= 1:
		return "1:1"
	case frames <= 3:
		return "16:9"
	default:
		return "21:9"
	}
}
