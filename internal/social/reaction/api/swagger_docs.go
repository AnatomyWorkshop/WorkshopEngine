package api

// reactionCountsDoc
// @Summary     批量查询 Reaction 计数
// @Description 传入 targets 查询参数（格式：type:id,type:id）批量获取计数
// @Tags        social-reactions
// @Produce     json
// @Param       targets query string true "目标列表（game:uuid,post:uuid）"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/reactions/counts [get]
func reactionCountsDoc() {}

// reactionMineDoc
// @Summary     查询自己的 Reaction 状态
// @Description 查看当前用户对指定目标的 reaction 状态
// @Tags        social-reactions
// @Produce     json
// @Param       target_type path string true "目标类型（game/comment/post）"
// @Param       target_id   path string true "目标 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/reactions/mine/{target_type}/{target_id} [get]
func reactionMineDoc() {}

// reactionAddDoc
// @Summary     添加 Reaction
// @Description 对目标添加点赞/收藏
// @Tags        social-reactions
// @Produce     json
// @Param       target_type path string true "目标类型（game/comment/post）"
// @Param       target_id   path string true "目标 ID"
// @Param       type        path string true "Reaction 类型（like/favorite）"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/reactions/{target_type}/{target_id}/{type} [post]
func reactionAddDoc() {}

// reactionRemoveDoc
// @Summary     移除 Reaction
// @Description 取消对目标的点赞/收藏
// @Tags        social-reactions
// @Produce     json
// @Param       target_type path string true "目标类型（game/comment/post）"
// @Param       target_id   path string true "目标 ID"
// @Param       type        path string true "Reaction 类型（like/favorite）"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/reactions/{target_type}/{target_id}/{type} [delete]
func reactionRemoveDoc() {}
