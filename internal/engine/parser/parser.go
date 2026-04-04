package parser

import (
	"encoding/json"
	"regexp"
	"strings"
)

// ParsedResponse AI 响应解析结果
// 这是整个引擎中最重要的数据结构之一。
// 它决定了游戏引擎能正确理解 AI 输出的格式。
type ParsedResponse struct {
	// 核心叙事内容（给前端展示）
	Narrative string `json:"narrative"`
	// 选项按钮列表
	Options []string `json:"options"`
	// 变量更新补丁（写入 Page 沙箱，不直接持久化）
	StatePatch map[string]any `json:"state_patch"`
	// 记忆摘要（由 Memory Worker 异步落库）
	Summary string `json:"summary"`

	// 视觉小说专用指令（来自 game_response 格式）
	VN *VNDirectives `json:"vn,omitempty"`

	// 调试信息
	RawContent string `json:"raw_content,omitempty"`
	ParseMode  string `json:"parse_mode"` // "xml_game_response" | "xml_narrative" | "numbered_list" | "fallback"
}

// VNDirectives 视觉小说渲染指令（来自 <game_response> 格式）
type VNDirectives struct {
	BGM     string          `json:"bgm,omitempty"`
	BG      string          `json:"bg,omitempty"`
	CG      string          `json:"cg,omitempty"`
	HideCG  bool            `json:"hide_cg,omitempty"`
	Sprites []SpriteAction  `json:"sprites,omitempty"`
	Lines   []DialogueLine  `json:"lines,omitempty"`
}

// SpriteAction 角色立绘动作
type SpriteAction struct {
	Character string `json:"character"`
	File      string `json:"file"`
	Action    string `json:"action,omitempty"` // shake | jump_up | jump_down
}

// DialogueLine 一行对话
type DialogueLine struct {
	Speaker string `json:"speaker"` // 空字符串 = 旁白
	Sprite  string `json:"sprite,omitempty"`
	Text    string `json:"text"`
}

// Parse 解析 LLM 原始输出，三层回退
func Parse(raw string) *ParsedResponse {
	resp := &ParsedResponse{RawContent: raw, StatePatch: map[string]any{}}

	// 层级 1：尝试解析视觉小说格式 <game_response>
	if parsed := tryParseGameResponse(raw, resp); parsed {
		resp.ParseMode = "xml_game_response"
		return resp
	}

	// 层级 2：尝试解析通用 XML 格式（<Narrative>/<Options>/<UpdateState>/<Summary>）
	if parsed := tryParseNarrativeXML(raw, resp); parsed {
		resp.ParseMode = "xml_narrative"
		return resp
	}

	// 层级 3：降级解析编号列表（应对 LLM 不跟随格式的情况）
	if parsed := tryParseNumberedList(raw, resp); parsed {
		resp.ParseMode = "numbered_list"
		return resp
	}

	// 层级 4：兜底，整段文字作为叙事，选项留空（由调用方按模板配置填充）
	resp.Narrative = strings.TrimSpace(raw)
	resp.ParseMode = "fallback"
	return resp
}

// ──────────────────────────────────────────────────────
// 层级 1：视觉小说 game_response 格式
// ──────────────────────────────────────────────────────

var gameResponseRe = regexp.MustCompile(`(?s)<game_response>(.*?)</game_response>`)
var choiceRe       = regexp.MustCompile(`\[choice\|(.+?)\]`)
var bgRe           = regexp.MustCompile(`\[bg\|(.+?)\]`)
var bgmRe          = regexp.MustCompile(`\[bgm\|(.+?)\]`)
var cgRe           = regexp.MustCompile(`\[cg\|(.+?)\]`)
var hideCGRe       = regexp.MustCompile(`\[hide_cg\]`)
var actionRe       = regexp.MustCompile(`\[action\|([^|]+)\|([^\]]+)\]`)

func tryParseGameResponse(raw string, resp *ParsedResponse) bool {
	m := gameResponseRe.FindStringSubmatch(raw)
	if m == nil {
		return false
	}
	body := m[1]
	vn := &VNDirectives{}

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		switch {
		case bgRe.MatchString(line):
			vn.BG = bgRe.FindStringSubmatch(line)[1]
		case bgmRe.MatchString(line):
			vn.BGM = bgmRe.FindStringSubmatch(line)[1]
		case cgRe.MatchString(line):
			vn.CG = cgRe.FindStringSubmatch(line)[1]
		case hideCGRe.MatchString(line):
			vn.HideCG = true
		case choiceRe.MatchString(line):
			parts := strings.Split(choiceRe.FindStringSubmatch(line)[1], "|")
			resp.Options = append(resp.Options, parts...)
		default:
			// 尝试解析对话行（格式：角色名|立绘文件|台词 或 旁白||文字）
			parts := strings.SplitN(line, "|", 3)
			if len(parts) == 3 {
				speaker, sprite, text := parts[0], parts[1], parts[2]
				// 提取 [action] 指令
				action := ""
				if am := actionRe.FindStringSubmatch(text); am != nil {
					action = am[2]
					text = strings.TrimSpace(actionRe.ReplaceAllString(text, ""))
				}
				vn.Lines = append(vn.Lines, DialogueLine{
					Speaker: speaker, Sprite: sprite, Text: text,
				})
				if sprite != "" && action != "" {
					vn.Sprites = append(vn.Sprites, SpriteAction{
						Character: speaker, File: sprite, Action: action,
					})
				}
				// 旁白和对话都累积到 Narrative 里给不支持 VN 渲染的前端降级用
				if speaker == "旁白" || speaker == "" {
					resp.Narrative += text + " "
				}
			} else {
				resp.Narrative += line + " "
			}
		}
	}

	resp.VN = vn

	// 从 raw 中也尝试提取 <Summary>（视觉小说卡可能同时输出）
	resp.Summary = extractXMLTag(raw, "Summary")
	resp.StatePatch = extractStatePatch(raw)
	return true
}

// ──────────────────────────────────────────────────────
// 层级 2：通用 XML 格式
// ──────────────────────────────────────────────────────

var optionTagRe = regexp.MustCompile(`<option[^>]*>([^<]+)</option>`)

func tryParseNarrativeXML(raw string, resp *ParsedResponse) bool {
	narrative := extractXMLTag(raw, "Narrative")
	if narrative == "" {
		// 兼容中文标签
		narrative = extractXMLTag(raw, "叙事")
	}
	if narrative == "" {
		return false
	}

	resp.Narrative = strings.TrimSpace(narrative)

	// 解析选项
	optionsBlock := extractXMLTag(raw, "Options")
	for _, m := range optionTagRe.FindAllStringSubmatch(optionsBlock, -1) {
		if text := strings.TrimSpace(m[1]); text != "" {
			resp.Options = append(resp.Options, text)
		}
	}

	resp.Summary = extractXMLTag(raw, "Summary")
	resp.StatePatch = extractStatePatch(raw)
	return true
}

// ──────────────────────────────────────────────────────
// 层级 3：编号列表降级
// ──────────────────────────────────────────────────────

var numberedOptRe = regexp.MustCompile(`(?m)^[①②③④⑤1-5][.、．]\s*(.+)$`)

func tryParseNumberedList(raw string, resp *ParsedResponse) bool {
	opts := numberedOptRe.FindAllStringSubmatch(raw, -1)
	if len(opts) < 2 {
		return false
	}
	// 找到第一个编号行之前的部分作为叙事
	idx := numberedOptRe.FindStringIndex(raw)
	if idx != nil {
		resp.Narrative = strings.TrimSpace(raw[:idx[0]])
	}
	for _, m := range opts {
		resp.Options = append(resp.Options, strings.TrimSpace(m[1]))
	}
	return true
}

// ──────────────────────────────────────────────────────
// 工具函数
// ──────────────────────────────────────────────────────

func extractXMLTag(text, tag string) string {
	re := regexp.MustCompile(`(?s)<` + tag + `>(.*?)</` + tag + `>`)
	if m := re.FindStringSubmatch(text); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// extractStatePatch 从 <UpdateState>{...}</UpdateState> 解析 JSON 变量补丁
func extractStatePatch(raw string) map[string]any {
	block := extractXMLTag(raw, "UpdateState")
	if block == "" {
		block = extractXMLTag(raw, "StatePatch")
	}
	if block == "" {
		return map[string]any{}
	}
	var patch map[string]any
	if err := json.Unmarshal([]byte(block), &patch); err != nil {
		return map[string]any{}
	}
	return patch
}
