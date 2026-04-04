// Package scheduled 实现 ScheduledTurn（自动回合）触发逻辑。
//
// MVP 模式：variable_threshold
//   - 每次 PlayTurn 完成后，引擎在 TurnResponse 中返回 scheduled_input（若触发）。
//   - 前端收到非空 scheduled_input 后，再次调用 PlayTurn 完成自动回合。
//   - 触发规则存储于 GameTemplate.Config.scheduled_turns，游戏设计师纯配置，无需写代码。
//
// 规则 JSON 示例：
//
//	{
//	  "id":               "hostile_npc",
//	  "mode":             "variable_threshold",
//	  "condition_var":    "emotion.tension",
//	  "threshold":        70,
//	  "probability":      0.75,
//	  "cooldown_floors":  3,
//	  "user_input":       "[SYSTEM: 紧张状态自主触发，生成一条 NPC 敌对内容]"
//	}
//
// 冷却记录写入变量沙箱，键名格式：__sched.<id>.last_floor
// （与 emotion_config 等其它系统变量共用变量沙箱，无额外存储）
package scheduled

import (
	"encoding/json"
	"math/rand"
	"strings"
)

// TriggerRule 一条自动回合触发规则。
type TriggerRule struct {
	// ID 规则唯一标识（用于冷却记录的 key 名，建议英文无空格）
	ID string `json:"id"`

	// Mode 触发模式。MVP 只支持 "variable_threshold"（变量阈值）。
	// 留空时默认视为 "variable_threshold"。
	Mode string `json:"mode"`

	// ConditionVar 被监测的变量路径（点分格式，支持嵌套）。
	// 例："emotion.tension"、"npc.夜歌.trust"、"viral_heat"
	ConditionVar string `json:"condition_var"`

	// Threshold 触发阈值：ConditionVar 的值 >= Threshold 时满足条件。
	Threshold float64 `json:"threshold"`

	// Probability 触发概率 [0, 1]。<= 0 时视为 1.0（必然触发）。
	Probability float64 `json:"probability"`

	// CooldownFloors 两次成功触发之间的最少楼层间隔。
	// 0 表示无冷却（每回合都可触发）。
	CooldownFloors int `json:"cooldown_floors"`

	// UserInput 触发时注入给 PlayTurn 的用户输入文本。
	// 与 EventPool 互斥：若 EventPool 非空，则忽略此字段，从 EventPool 随机抽取。
	UserInput string `json:"user_input"`

	// EventPool 随机事件池（模拟刷新机制）。
	// 非空时，每次触发从池中均匀随机抽取一条作为 user_input，实现世界事件的随机涌现。
	// 适合"世界刷新"类场景：突发新闻、随机 NPC 发帖、季节事件等。
	EventPool []string `json:"event_pool,omitempty"`
}

// CooldownKey 返回在变量沙箱中存储此规则冷却记录的键名。
// 格式：__sched.<ruleID>.last_floor
func CooldownKey(ruleID string) string {
	return "__sched." + ruleID + ".last_floor"
}

// Evaluate 对 rules 列表逐条检查，返回第一条满足所有条件的规则；无匹配返回 nil。
//
// 参数：
//   - variables: sb.Flatten() 输出的当前会话变量完整快照（支持任意层级嵌套 map）
//   - currentFloor: 本回合提交后的累计楼层数（session.IncrFloorCount 的返回值）
//   - rng: 外部注入的 [0, 1) 随机数（便于单元测试确定性覆盖）
//
// 冷却逻辑：若 CooldownFloors > 0，读取变量沙箱中的 CooldownKey(r.ID)，
// 确保距上次触发已过 CooldownFloors 个楼层。
func Evaluate(rules []TriggerRule, variables map[string]any, currentFloor int, rng float64) *TriggerRule {
	for i := range rules {
		r := &rules[i]
		if r.Mode != "" && r.Mode != "variable_threshold" {
			continue // MVP 只处理 variable_threshold
		}

		// 1. 冷却检查
		if r.CooldownFloors > 0 {
			if last, ok := GetFloat(variables, CooldownKey(r.ID)); ok {
				if currentFloor-int(last) < r.CooldownFloors {
					continue
				}
			}
		}

		// 2. 变量阈值检查
		val, ok := GetFloat(variables, r.ConditionVar)
		if !ok || val < r.Threshold {
			continue
		}

		// 3. 概率掷骰（rng 在 [0, prob) 区间内才触发）
		prob := r.Probability
		if prob <= 0 {
			prob = 1.0
		}
		if rng >= prob {
			continue
		}

		return r
	}
	return nil
}

// PickInput 返回规则本次触发应使用的 user_input 文本。
// 若 EventPool 非空，均匀随机抽取一条；否则返回 UserInput 字段。
func (r *TriggerRule) PickInput() string {
	if len(r.EventPool) > 0 {
		return r.EventPool[rand.Intn(len(r.EventPool))]
	}
	return r.UserInput
}

// GetFloat 从（可能嵌套的）变量 map 中按点分路径读取数值。
//
// 支持任意深度路径：
//   - "tension"            → vars["tension"]
//   - "emotion.tension"    → vars["emotion"].(map)["tension"]
//   - "npc.夜歌.trust"     → vars["npc"].(map)["夜歌"].(map)["trust"]
func GetFloat(vars map[string]any, path string) (float64, bool) {
	dot := strings.IndexByte(path, '.')
	if dot < 0 {
		// 叶子节点：直接转换
		return toFloat(vars[path])
	}
	head, tail := path[:dot], path[dot+1:]
	nested, ok := vars[head].(map[string]any)
	if !ok {
		return 0, false
	}
	return GetFloat(nested, tail)
}

// toFloat 将 json.Unmarshal 可能产出的数值类型统一转为 float64。
func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}
