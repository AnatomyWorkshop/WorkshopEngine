package card

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// CharacterCardData 解析后的角色卡数据（兼容 CCv2 和 CCv3）
type CharacterCardData struct {
	Spec        string         `json:"spec"` // chara_card_v2 | chara_card_v3
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Personality string         `json:"personality"`
	Scenario    string         `json:"scenario"`
	FirstMes    string         `json:"first_mes"`
	MesExample  string         `json:"mes_example"`
	SystemPrompt string        `json:"system_prompt"`
	Tags        []string       `json:"tags"`
	Creator     string         `json:"creator"`
	CharacterBook *Lorebook    `json:"character_book,omitempty"`
	Extensions  map[string]any `json:"extensions,omitempty"`
	RawData     map[string]any `json:"-"` // 原始 JSON（保留完整数据入库）
}

// Lorebook 角色卡内嵌世界书
type Lorebook struct {
	Name    string          `json:"name"`
	Entries []LorebookEntry `json:"entries"`
}

// LorebookEntry 世界书条目
type LorebookEntry struct {
	Keys     []string `json:"keys"`
	Content  string   `json:"content"`
	Constant bool     `json:"constant"`
	Priority int      `json:"priority"`
	Enabled  bool     `json:"enabled"`
	Comment  string   `json:"comment"`
}

// ParseGWGamePNG 从 PNG 的 tEXt chunk（keyword = "gw_game"）提取 GW 游戏包 JSON。
// 返回原始 JSON 字节，调用方负责反序列化为具体结构。
func ParseGWGamePNG(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read png: %w", err)
	}
	if len(data) < 8 || !bytes.Equal(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		return nil, fmt.Errorf("not a valid PNG file")
	}
	chunks := extractPNGTextChunks(data[8:])
	raw, ok := chunks["gw_game"]
	if !ok {
		return nil, fmt.Errorf("no gw_game chunk found")
	}
	decoded, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(string(raw))
		if err != nil {
			return nil, fmt.Errorf("base64 decode: %w", err)
		}
	}
	return decoded, nil
}

// ParsePNG 从 PNG 文件流中解析 SillyTavern 角色卡数据
// 支持 tEXt chunk 中的 "chara"（CCv2）和 "ccv3"（CCv3）键
func ParsePNG(r io.Reader) (*CharacterCardData, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read png: %w", err)
	}

	// 验证 PNG 签名
	if len(data) < 8 || !bytes.Equal(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		return nil, fmt.Errorf("not a valid PNG file")
	}

	// 解析 PNG chunks
	textChunks := extractPNGTextChunks(data[8:])

	// 优先读 ccv3（V3 格式），其次 chara（V2 格式）
	for _, key := range []string{"ccv3", "chara"} {
		if raw, ok := textChunks[key]; ok {
			return decodeCharaJSON(raw)
		}
	}

	return nil, fmt.Errorf("no character data found in PNG (expected 'chara' or 'ccv3' tEXt chunk)")
}

// extractPNGTextChunks 提取 PNG 的所有 tEXt chunk（keyword → value）
func extractPNGTextChunks(data []byte) map[string][]byte {
	result := make(map[string][]byte)
	offset := 0
	for offset+8 <= len(data) {
		chunkLen := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		chunkType := string(data[offset+4 : offset+8])
		chunkData := []byte{}
		if offset+8+chunkLen <= len(data) {
			chunkData = data[offset+8 : offset+8+chunkLen]
		}
		offset += 8 + chunkLen + 4 // 跳过 length + type + data + CRC

		if chunkType == "tEXt" {
			// tEXt 格式：keyword\0value
			sepIdx := bytes.IndexByte(chunkData, 0)
			if sepIdx >= 0 {
				keyword := string(chunkData[:sepIdx])
				value := chunkData[sepIdx+1:]
				result[keyword] = value
			}
		}
		if chunkType == "IEND" {
			break
		}
	}
	return result
}

// decodeCharaJSON 将 tEXt chunk 的 base64 值解码为角色卡结构
func decodeCharaJSON(raw []byte) (*CharacterCardData, error) {
	decoded, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		// 尝试 URL-safe 编码
		decoded, err = base64.URLEncoding.DecodeString(string(raw))
		if err != nil {
			return nil, fmt.Errorf("base64 decode: %w", err)
		}
	}

	// 解析为通用 map 保留原始数据
	var rawMap map[string]any
	if err := json.Unmarshal(decoded, &rawMap); err != nil {
		return nil, fmt.Errorf("json parse: %w", err)
	}

	// 提取 spec 字段
	spec, _ := rawMap["spec"].(string)

	// 不同版本数据在 "data" 子字段里（V2/V3）或直接在根（V1 legacy）
	dataMap := rawMap
	if d, ok := rawMap["data"].(map[string]any); ok {
		dataMap = d
	}

	card := &CharacterCardData{
		Spec:         spec,
		Name:         str(dataMap, "name"),
		Description:  str(dataMap, "description"),
		Personality:  str(dataMap, "personality"),
		Scenario:     str(dataMap, "scenario"),
		FirstMes:     str(dataMap, "first_mes"),
		MesExample:   str(dataMap, "mes_example"),
		SystemPrompt: str(dataMap, "system_prompt"),
		Creator:      str(dataMap, "creator"),
		Extensions:   mapVal(dataMap, "extensions"),
		RawData:      rawMap,
	}

	// 解析 tags
	if tags, ok := dataMap["tags"].([]any); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				card.Tags = append(card.Tags, s)
			}
		}
	}

	// 解析内嵌世界书
	if cb, ok := dataMap["character_book"].(map[string]any); ok {
		card.CharacterBook = parseLorebookFromMap(cb)
	}

	return card, nil
}

func parseLorebookFromMap(m map[string]any) *Lorebook {
	lb := &Lorebook{Name: str(m, "name")}
	if entries, ok := m["entries"].([]any); ok {
		for _, e := range entries {
			em, ok := e.(map[string]any)
			if !ok {
				continue
			}
			entry := LorebookEntry{
				Content:  str(em, "content"),
				Constant: boolVal(em, "constant"),
				Priority: intVal(em, "priority"),
				Enabled:  boolValDefault(em, "enabled", true),
				Comment:  str(em, "comment"),
			}
			if keys, ok := em["keys"].([]any); ok {
				for _, k := range keys {
					if s, ok := k.(string); ok && s != "" {
						entry.Keys = append(entry.Keys, s)
					}
				}
			}
			lb.Entries = append(lb.Entries, entry)
		}
	}
	return lb
}

// ── 工具函数 ──────────────────────────────────────────

func str(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func mapVal(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

func boolVal(m map[string]any, key string) bool {
	v, _ := m[key].(bool)
	return v
}

func boolValDefault(m map[string]any, key string, def bool) bool {
	v, ok := m[key].(bool)
	if !ok {
		return def
	}
	return v
}

func intVal(m map[string]any, key string) int {
	v, _ := m[key].(float64)
	return int(v)
}
