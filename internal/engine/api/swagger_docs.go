package api

// playTurnDoc
// @Summary     执行游戏回合
// @Description 提交用户输入，执行一回合游戏逻辑
// @Tags        play-engine
// @Accept      json
// @Produce     json
// @Param       id   path string      true "会话 ID"
// @Param       body body TurnRequest true "回合请求"
// @Success     200 {object} map[string]any
// @Failure     400 {object} map[string]any
// @Failure     409 {object} map[string]any "并发生成冲突"
// @Security    BearerAuth
// @Router      /play/sessions/{id}/turn [post]
func playTurnDoc() {}

// playRegenDoc
// @Summary     重新生成上一回合
// @Description 以 isRegen=true 重新生成最后一回合（Swipe）
// @Tags        play-engine
// @Accept      json
// @Produce     json
// @Param       id   path string      true "会话 ID"
// @Param       body body TurnRequest true "回合请求"
// @Success     200 {object} map[string]any
// @Failure     409 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/regen [post]
func playRegenDoc() {}

// getSessionStateDoc
// @Summary     获取会话状态快照
// @Description 返回会话当前完整状态（变量、楼层摘要等）
// @Tags        play-engine
// @Produce     json
// @Param       id path string true "会话 ID"
// @Success     200 {object} map[string]any
// @Failure     404 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/state [get]
func getSessionStateDoc() {}

// streamTurnDoc
// @Summary     SSE 流式回合
// @Description 通过 Server-Sent Events 流式返回 AI 生成内容
// @Tags        play-engine
// @Produce     text/event-stream
// @Param       id       path  string true  "会话 ID"
// @Param       input    query string true  "用户输入"
// @Param       api_key  query string false "自定义 API Key"
// @Param       base_url query string false "自定义 Base URL"
// @Param       model    query string false "自定义模型"
// @Success     200 {string} string "SSE stream"
// @Failure     400 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/stream [get]
func streamTurnDoc() {}

// updateSessionDoc
// @Summary     更新会话信息
// @Description 修改会话标题、状态或公开标志
// @Tags        play-engine
// @Accept      json
// @Produce     json
// @Param       id   path string           true "会话 ID"
// @Param       body body UpdateSessionReq true "更新字段"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id} [patch]
func updateSessionDoc() {}

// deleteSessionDoc
// @Summary     删除会话
// @Description 删除会话及所有关联的楼层、页面和记忆
// @Tags        play-engine
// @Produce     json
// @Param       id path string true "会话 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id} [delete]
func deleteSessionDoc() {}

// getVariablesDoc
// @Summary     获取会话变量
// @Description 返回会话当前变量快照
// @Tags        play-engine
// @Produce     json
// @Param       id path string true "会话 ID"
// @Success     200 {object} map[string]any
// @Failure     404 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/variables [get]
func getVariablesDoc() {}

// patchVariablesDoc
// @Summary     合并更新会话变量
// @Description 以 merge 方式更新会话变量（只覆盖传入的 key）
// @Tags        play-engine
// @Accept      json
// @Produce     json
// @Param       id   path string         true "会话 ID"
// @Param       body body map[string]any true "要合并的变量"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/variables [patch]
func patchVariablesDoc() {}

// listFloorsDoc
// @Summary     楼层列表
// @Description 列出会话所有楼层（含激活页摘要），支持 from/to 范围过滤
// @Tags        play-engine
// @Produce     json
// @Param       id   path  string true  "会话 ID"
// @Param       from query int    false "起始楼层序号"
// @Param       to   query int    false "结束楼层序号"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/floors [get]
func listFloorsDoc() {}

// suggestDoc
// @Summary     AI 帮答
// @Description Impersonate 模式生成建议，不写入 Floor
// @Tags        play-engine
// @Produce     json
// @Param       id path string true "会话 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/suggest [post]
func suggestDoc() {}

// listPagesDoc
// @Summary     Swipe 页列表
// @Description 列出指定楼层的所有 Swipe 页
// @Tags        play-engine
// @Produce     json
// @Param       id  path string true "会话 ID"
// @Param       fid path string true "楼层 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/floors/{fid}/pages [get]
func listPagesDoc() {}

// activatePageDoc
// @Summary     激活 Swipe 页
// @Description 将指定页设为楼层的激活页
// @Tags        play-engine
// @Produce     json
// @Param       id  path string true "会话 ID"
// @Param       fid path string true "楼层 ID"
// @Param       pid path string true "页 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/floors/{fid}/pages/{pid}/activate [patch]
func activatePageDoc() {}

// listMemoriesDoc
// @Summary     列出记忆条目
// @Description 返回会话所有记忆条目
// @Tags        play-engine
// @Produce     json
// @Param       id path string true "会话 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/memories [get]
func listMemoriesDoc() {}

// createMemoryDoc
// @Summary     创建记忆
// @Description 手动创建记忆条目（创作者/调试用）
// @Tags        play-engine
// @Accept      json
// @Produce     json
// @Param       id   path string          true "会话 ID"
// @Param       body body CreateMemoryReq true "记忆内容"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/memories [post]
func createMemoryDoc() {}

// updateMemoryDoc
// @Summary     更新记忆字段
// @Description 更新记忆的 content/importance/type 等字段
// @Tags        play-engine
// @Accept      json
// @Produce     json
// @Param       id   path string         true "会话 ID"
// @Param       mid  path string         true "记忆 ID"
// @Param       body body map[string]any true "要更新的字段"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/memories/{mid} [patch]
func updateMemoryDoc() {}

// deleteMemoryDoc
// @Summary     删除记忆
// @Description 删除记忆条目（?hard=true 物理删除，否则软删除）
// @Tags        play-engine
// @Produce     json
// @Param       id   path  string true  "会话 ID"
// @Param       mid  path  string true  "记忆 ID"
// @Param       hard query bool   false "是否物理删除"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/memories/{mid} [delete]
func deleteMemoryDoc() {}

// consolidateMemoryDoc
// @Summary     触发记忆整合
// @Description 立即同步触发记忆整合（调试用）
// @Tags        play-engine
// @Produce     json
// @Param       id path string true "会话 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/memories/consolidate [post]
func consolidateMemoryDoc() {}

// forkSessionDoc
// @Summary     分叉会话
// @Description 从指定楼层分叉出新会话（平行时间线/存档点）
// @Tags        play-engine
// @Accept      json
// @Produce     json
// @Param       id   path string         true "会话 ID"
// @Param       body body ForkSessionReq true "分叉参数"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/fork [post]
func forkSessionDoc() {}

// promptPreviewDoc
// @Summary     Prompt 预览
// @Description Prompt dry-run，不调用 LLM，供创作者调试
// @Tags        play-engine
// @Produce     json
// @Param       id    path  string true  "会话 ID"
// @Param       input query string false "用户输入"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/prompt-preview [get]
func promptPreviewDoc() {}

// floorSnapshotDoc
// @Summary     楼层 Prompt 快照
// @Description 返回 Verifier 结果和命中词条
// @Tags        play-engine
// @Produce     json
// @Param       id  path string true "会话 ID"
// @Param       fid path string true "楼层 ID"
// @Success     200 {object} map[string]any
// @Failure     404 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/floors/{fid}/snapshot [get]
func floorSnapshotDoc() {}

// listToolExecutionsDoc
// @Summary     工具执行记录
// @Description 查询会话的工具执行记录
// @Tags        play-engine
// @Produce     json
// @Param       id       path  string true  "会话 ID"
// @Param       floor_id query string false "按楼层过滤"
// @Param       limit    query int    false "返回条数（默认 50）"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/tool-executions [get]
func listToolExecutionsDoc() {}

// listMemoryEdgesDoc
// @Summary     列出记忆关系边
// @Description 列出会话所有记忆关系边，支持按 relation 过滤
// @Tags        play-engine
// @Produce     json
// @Param       id       path  string true  "会话 ID"
// @Param       relation query string false "关系类型过滤（updates|contradicts|supports|resolves）"
// @Param       limit    query int    false "返回条数（默认 50）"
// @Param       offset   query int    false "偏移量"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/memory-edges [get]
func listMemoryEdgesDoc() {}

// listMemoryEdgesByMemoryDoc
// @Summary     列出单条记忆的关系边
// @Description 列出指定记忆的所有双向关系边
// @Tags        play-engine
// @Produce     json
// @Param       id  path string true "会话 ID"
// @Param       mid path string true "记忆 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/memories/{mid}/edges [get]
func listMemoryEdgesByMemoryDoc() {}

// createMemoryEdgeDoc
// @Summary     创建记忆关系边
// @Description 手动创建记忆间的关系边（创作者调试用）
// @Tags        play-engine
// @Accept      json
// @Produce     json
// @Param       id   path string true "会话 ID"
// @Param       body body object true "关系边（from_id, to_id, relation）"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/memory-edges [post]
func createMemoryEdgeDoc() {}

// updateMemoryEdgeDoc
// @Summary     更新关系边
// @Description 修改关系边的 relation 类型
// @Tags        play-engine
// @Accept      json
// @Produce     json
// @Param       id  path string true "会话 ID"
// @Param       eid path string true "边 ID"
// @Param       body body object true "新 relation"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/memory-edges/{eid} [patch]
func updateMemoryEdgeDoc() {}

// deleteMemoryEdgeDoc
// @Summary     删除关系边
// @Description 删除指定的记忆关系边
// @Tags        play-engine
// @Produce     json
// @Param       id  path string true "会话 ID"
// @Param       eid path string true "边 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/memory-edges/{eid} [delete]
func deleteMemoryEdgeDoc() {}

// listBranchesDoc
// @Summary     列出会话分支
// @Description 列出会话所有分支（含 main）
// @Tags        play-engine
// @Produce     json
// @Param       id path string true "会话 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/branches [get]
func listBranchesDoc() {}

// createBranchDoc
// @Summary     从楼层创建分支
// @Description 从指定楼层创建新分支
// @Tags        play-engine
// @Produce     json
// @Param       id  path string true "会话 ID"
// @Param       fid path string true "楼层 ID"
// @Success     200 {object} map[string]any
// @Failure     400 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/floors/{fid}/branch [post]
func createBranchDoc() {}

// deleteBranchDoc
// @Summary     删除分支
// @Description 删除指定分支（不能删除 main 分支）
// @Tags        play-engine
// @Produce     json
// @Param       id  path string true "会话 ID"
// @Param       bid path string true "分支 ID"
// @Success     200 {object} map[string]any
// @Failure     400 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/branches/{bid} [delete]
func deleteBranchDoc() {}

// exportSessionDoc
// @Summary     导出会话
// @Description 导出会话为 .thchat（WE 原生，无损）或 .jsonl（ST 兼容，有损）格式
// @Tags        play-engine
// @Produce     json
// @Param       id     path  string false "会话 ID"
// @Param       format query string false "导出格式：thchat（默认）| jsonl"
// @Success     200 {object} ThchatExport
// @Failure     404 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/{id}/export [get]
func exportSessionDoc() {}

// importSessionDoc
// @Summary     导入会话
// @Description 从 .thchat JSON 导入会话（所有 ID 重映射为新 UUID）
// @Tags        play-engine
// @Accept      json
// @Produce     json
// @Param       body body ThchatExport true ".thchat 导入数据"
// @Success     200 {object} map[string]any "session_id"
// @Failure     400 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions/import [post]
func importSessionDoc() {}

