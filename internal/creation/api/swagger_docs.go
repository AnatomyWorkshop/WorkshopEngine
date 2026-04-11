package creation

// ── Cards ────────────────────────────────────────────────────────────────────

// importCardDoc
// @Summary     导入角色卡 PNG
// @Description 上传角色卡 PNG 文件，解析内嵌数据并导入
// @Tags        creation-cards
// @Accept      multipart/form-data
// @Produce     json
// @Param       file formData file true "角色卡 PNG 文件"
// @Success     200 {object} map[string]any
// @Failure     400 {object} map[string]any
// @Failure     422 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/cards/import [post]
func importCardDoc() {}

// listCardsDoc
// @Summary     角色卡列表
// @Description 获取公开角色卡列表
// @Tags        creation-cards
// @Produce     json
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/cards [get]
func listCardsDoc() {}

// getCardDoc
// @Summary     角色卡详情
// @Description 根据 slug 获取角色卡详情
// @Tags        creation-cards
// @Produce     json
// @Param       slug path string true "角色卡 slug"
// @Success     200 {object} map[string]any
// @Failure     404 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/cards/{slug} [get]
func getCardDoc() {}

// deleteCardDoc
// @Summary     删除角色卡
// @Description 根据 slug 删除角色卡
// @Tags        creation-cards
// @Produce     json
// @Param       slug path string true "角色卡 slug"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/cards/{slug} [delete]
func deleteCardDoc() {}

// updateCardDoc
// @Summary     更新角色卡字段
// @Description 部分更新角色卡字段
// @Tags        creation-cards
// @Accept      json
// @Produce     json
// @Param       slug path string true "角色卡 slug"
// @Param       body body map[string]any true "更新字段"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/cards/{slug} [patch]
func updateCardDoc() {}

// ── Lorebook ─────────────────────────────────────────────────────────────────

// listLorebookDoc
// @Summary     世界书词条列表
// @Description 获取指定游戏的世界书词条
// @Tags        creation-lorebook
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/lorebook [get]
func listLorebookDoc() {}

// upsertLorebookDoc
// @Summary     新增/更新世界书词条
// @Description 创建或更新世界书词条
// @Tags        creation-lorebook
// @Accept      json
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       body body map[string]any true "WorldbookEntry"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/lorebook [post]
func upsertLorebookDoc() {}

// deleteLorebookDoc
// @Summary     删除世界书词条
// @Description 删除指定世界书词条
// @Tags        creation-lorebook
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       eid path string true "词条 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/lorebook/{eid} [delete]
func deleteLorebookDoc() {}

// ── Templates ────────────────────────────────────────────────────────────────

// listTemplatesDoc
// @Summary     模板列表
// @Description 获取游戏模板列表
// @Tags        creation-templates
// @Produce     json
// @Param       status query string false "过滤状态: published|all" default(published)
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates [get]
func listTemplatesDoc() {}

// createTemplateDoc
// @Summary     创建模板
// @Description 创建新游戏模板
// @Tags        creation-templates
// @Accept      json
// @Produce     json
// @Param       body body map[string]any true "GameTemplate"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates [post]
func createTemplateDoc() {}

// updateTemplateDoc
// @Summary     更新模板
// @Description 部分更新游戏模板字段
// @Tags        creation-templates
// @Accept      json
// @Produce     json
// @Param       id path string true "模板 ID"
// @Param       body body map[string]any true "更新字段"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id} [patch]
func updateTemplateDoc() {}

// deleteTemplateDoc
// @Summary     删除模板
// @Description 删除游戏模板
// @Tags        creation-templates
// @Produce     json
// @Param       id path string true "模板 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id} [delete]
func deleteTemplateDoc() {}

// ── LLM Profiles ─────────────────────────────────────────────────────────────

// createLLMProfileDoc
// @Summary     创建 LLM 配置
// @Description 创建新的 LLM 配置（含加密 API Key）
// @Tags        creation-llm
// @Accept      json
// @Produce     json
// @Param       body body map[string]any true "name, provider, model_id, base_url, api_key, params"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/llm-profiles [post]
func createLLMProfileDoc() {}

// listLLMProfilesDoc
// @Summary     LLM 配置列表
// @Description 列出当前账户的 LLM 配置
// @Tags        creation-llm
// @Produce     json
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/llm-profiles [get]
func listLLMProfilesDoc() {}

// getLLMProfileDoc
// @Summary     获取单个 LLM 配置
// @Description 根据 ID 获取 LLM 配置详情
// @Tags        creation-llm
// @Produce     json
// @Param       id path string true "配置 ID"
// @Success     200 {object} map[string]any
// @Failure     404 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/llm-profiles/{id} [get]
func getLLMProfileDoc() {}

// updateLLMProfileDoc
// @Summary     更新 LLM 配置
// @Description 部分更新 LLM 配置字段
// @Tags        creation-llm
// @Accept      json
// @Produce     json
// @Param       id path string true "配置 ID"
// @Param       body body map[string]any true "更新字段"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/llm-profiles/{id} [patch]
func updateLLMProfileDoc() {}

// deleteLLMProfileDoc
// @Summary     软删除 LLM 配置
// @Description 将 LLM 配置标记为 deleted
// @Tags        creation-llm
// @Produce     json
// @Param       id path string true "配置 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/llm-profiles/{id} [delete]
func deleteLLMProfileDoc() {}

// discoverModelsDoc
// @Summary     发现可用模型
// @Description 拉取指定 Provider 的可用模型列表
// @Tags        creation-llm
// @Accept      json
// @Produce     json
// @Param       body body map[string]any true "base_url, api_key"
// @Success     200 {object} map[string]any
// @Failure     502 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/llm-profiles/models/discover [post]
func discoverModelsDoc() {}

// testLLMConnectionDoc
// @Summary     测试 LLM 连接
// @Description 向指定 Provider 发送探测消息，返回时延和响应片段
// @Tags        creation-llm
// @Accept      json
// @Produce     json
// @Param       body body map[string]any true "base_url, api_key, model"
// @Success     200 {object} map[string]any
// @Failure     502 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/llm-profiles/models/test [post]
func testLLMConnectionDoc() {}

// activateLLMProfileDoc
// @Summary     绑定配置到作用域
// @Description 将 LLM 配置绑定到指定 scope/slot
// @Tags        creation-llm
// @Accept      json
// @Produce     json
// @Param       id path string true "配置 ID"
// @Param       body body map[string]any true "scope, scope_id, slot, params"
// @Success     200 {object} map[string]any
// @Failure     404 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/llm-profiles/{id}/activate [post]
func activateLLMProfileDoc() {}

// ── Preset Entries ───────────────────────────────────────────────────────────

// listPresetEntriesDoc
// @Summary     预设条目列表
// @Description 列出游戏的预设条目（按 injection_order 升序）
// @Tags        creation-preset
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       enabled query string false "过滤: true|false"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/preset-entries [get]
func listPresetEntriesDoc() {}

// createPresetEntryDoc
// @Summary     创建预设条目
// @Description 新建一条预设条目
// @Tags        creation-preset
// @Accept      json
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       body body map[string]any true "PresetEntry"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/preset-entries [post]
func createPresetEntryDoc() {}

// updatePresetEntryDoc
// @Summary     更新预设条目
// @Description 部分更新预设条目
// @Tags        creation-preset
// @Accept      json
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       eid path string true "条目 identifier"
// @Param       body body map[string]any true "更新字段"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/preset-entries/{eid} [patch]
func updatePresetEntryDoc() {}

// deletePresetEntryDoc
// @Summary     删除预设条目
// @Description 删除指定预设条目
// @Tags        creation-preset
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       eid path string true "条目 identifier"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/preset-entries/{eid} [delete]
func deletePresetEntryDoc() {}

// reorderPresetEntriesDoc
// @Summary     批量调整条目顺序
// @Description 批量更新 injection_order
// @Tags        creation-preset
// @Accept      json
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       body body []map[string]any true "[{identifier, injection_order}]"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/preset-entries/reorder [put]
func reorderPresetEntriesDoc() {}

// ── Preset Tools ─────────────────────────────────────────────────────────────

// listPresetToolsDoc
// @Summary     预设工具列表
// @Description 列出游戏的预设工具
// @Tags        creation-tools
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/tools [get]
func listPresetToolsDoc() {}

// createPresetToolDoc
// @Summary     创建预设工具
// @Description 新建一个预设工具
// @Tags        creation-tools
// @Accept      json
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       body body map[string]any true "PresetTool"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/tools [post]
func createPresetToolDoc() {}

// updatePresetToolDoc
// @Summary     更新预设工具
// @Description 部分更新预设工具
// @Tags        creation-tools
// @Accept      json
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       tid path string true "工具 ID"
// @Param       body body map[string]any true "更新字段"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/tools/{tid} [patch]
func updatePresetToolDoc() {}

// deletePresetToolDoc
// @Summary     删除预设工具
// @Description 删除指定预设工具
// @Tags        creation-tools
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       tid path string true "工具 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/tools/{tid} [delete]
func deletePresetToolDoc() {}

// ── Regex Profiles & Rules ───────────────────────────────────────────────────

// listRegexProfilesDoc
// @Summary     正则配置列表
// @Description 列出游戏的正则替换配置
// @Tags        creation-regex
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/regex-profiles [get]
func listRegexProfilesDoc() {}

// createRegexProfileDoc
// @Summary     创建正则配置
// @Description 新建正则替换配置
// @Tags        creation-regex
// @Accept      json
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       body body map[string]any true "name, enabled"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/regex-profiles [post]
func createRegexProfileDoc() {}

// updateRegexProfileDoc
// @Summary     更新正则配置
// @Description 部分更新正则配置
// @Tags        creation-regex
// @Accept      json
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       pid path string true "配置 ID"
// @Param       body body map[string]any true "更新字段"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/regex-profiles/{pid} [patch]
func updateRegexProfileDoc() {}

// deleteRegexProfileDoc
// @Summary     删除正则配置
// @Description 删除正则配置及其所有规则
// @Tags        creation-regex
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       pid path string true "配置 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/regex-profiles/{pid} [delete]
func deleteRegexProfileDoc() {}

// listRegexRulesDoc
// @Summary     正则规则列表
// @Description 列出配置下的正则规则
// @Tags        creation-regex
// @Produce     json
// @Param       pid path string true "配置 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/regex-profiles/{pid}/rules [get]
func listRegexRulesDoc() {}

// createRegexRuleDoc
// @Summary     创建正则规则
// @Description 新建正则替换规则
// @Tags        creation-regex
// @Accept      json
// @Produce     json
// @Param       pid path string true "配置 ID"
// @Param       body body map[string]any true "pattern, replacement, apply_to, order, enabled"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/regex-profiles/{pid}/rules [post]
func createRegexRuleDoc() {}

// updateRegexRuleDoc
// @Summary     更新正则规则
// @Description 部分更新正则规则
// @Tags        creation-regex
// @Accept      json
// @Produce     json
// @Param       pid path string true "配置 ID"
// @Param       rid path string true "规则 ID"
// @Param       body body map[string]any true "更新字段"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/regex-profiles/{pid}/rules/{rid} [patch]
func updateRegexRuleDoc() {}

// deleteRegexRuleDoc
// @Summary     删除正则规则
// @Description 删除指定正则规则
// @Tags        creation-regex
// @Produce     json
// @Param       pid path string true "配置 ID"
// @Param       rid path string true "规则 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/regex-profiles/{pid}/rules/{rid} [delete]
func deleteRegexRuleDoc() {}

// reorderRegexRulesDoc
// @Summary     批量调整规则顺序
// @Description 批量更新规则 order
// @Tags        creation-regex
// @Accept      json
// @Produce     json
// @Param       pid path string true "配置 ID"
// @Param       body body []map[string]any true "[{id, order}]"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/regex-profiles/{pid}/rules/reorder [put]
func reorderRegexRulesDoc() {}

// ── Materials ────────────────────────────────────────────────────────────────

// createMaterialDoc
// @Summary     创建素材
// @Description 新建素材条目
// @Tags        creation-materials
// @Accept      json
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       body body map[string]any true "Material"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/materials [post]
func createMaterialDoc() {}

// listMaterialsDoc
// @Summary     素材列表
// @Description 列出游戏素材（支持多维度过滤）
// @Tags        creation-materials
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       type query string false "素材类型"
// @Param       mood query string false "情绪"
// @Param       style query string false "风格"
// @Param       function_tag query string false "功能标签"
// @Param       enabled query string false "启用状态: true|false"
// @Param       limit query int false "分页大小" default(50)
// @Param       offset query int false "偏移量" default(0)
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/materials [get]
func listMaterialsDoc() {}

// updateMaterialDoc
// @Summary     更新素材
// @Description 部分更新素材字段
// @Tags        creation-materials
// @Accept      json
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       mid path string true "素材 ID"
// @Param       body body map[string]any true "更新字段"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/materials/{mid} [patch]
func updateMaterialDoc() {}

// deleteMaterialDoc
// @Summary     删除素材
// @Description 删除指定素材
// @Tags        creation-materials
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       mid path string true "素材 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/materials/{mid} [delete]
func deleteMaterialDoc() {}

// batchImportMaterialsDoc
// @Summary     批量导入素材
// @Description 批量创建素材条目
// @Tags        creation-materials
// @Accept      json
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       body body []map[string]any true "Material 数组"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/materials/batch [post]
func batchImportMaterialsDoc() {}

// ── Import / Export ──────────────────────────────────────────────────────────

// importSTPresetDoc
// @Summary     导入 ST 预设
// @Description 导入 SillyTavern 预设 JSON，批量写入 PresetEntry
// @Tags        creation-import-export
// @Accept      json
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       body body map[string]any true "ST preset JSON"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/preset/import-st [post]
func importSTPresetDoc() {}

// importSTLorebookDoc
// @Summary     导入 ST 世界书
// @Description 导入 SillyTavern 世界书 JSON，批量写入 WorldbookEntry
// @Tags        creation-import-export
// @Accept      json
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Param       body body map[string]any true "ST worldbook JSON"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/lorebook/import-st [post]
func importSTLorebookDoc() {}

// exportGamePackageDoc
// @Summary     导出游戏包
// @Description 打包导出游戏模板及所有关联数据
// @Tags        creation-import-export
// @Produce     json
// @Param       id path string true "游戏模板 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/{id}/export [get]
func exportGamePackageDoc() {}

// importGamePackageDoc
// @Summary     导入游戏包
// @Description 解包导入游戏，重建所有关联数据
// @Tags        creation-import-export
// @Accept      json
// @Produce     json
// @Param       body body map[string]any true "导出的游戏包 JSON"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/templates/import [post]
func importGamePackageDoc() {}
