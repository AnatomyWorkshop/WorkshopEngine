package api

// listCommentsDoc
// @Summary     评论列表
// @Description 获取游戏的顶层评论（支持 linear/nested 模式）
// @Tags        social-comments
// @Produce     json
// @Param       id     path  string false "游戏 ID"
// @Param       mode   query string false "模式：linear（默认）| nested"
// @Param       limit  query int    false "每页数量"
// @Param       offset query int    false "偏移量"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/games/{id}/comments [get]
func listCommentsDoc() {}

// createCommentDoc
// @Summary     发表评论
// @Description 在游戏下发表主楼评论
// @Tags        social-comments
// @Accept      json
// @Produce     json
// @Param       id   path string true "游戏 ID"
// @Param       body body object{content=string} true "评论内容"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/games/{id}/comments [post]
func createCommentDoc() {}

// listRepliesDoc
// @Summary     回复列表
// @Description 获取评论的回复列表
// @Tags        social-comments
// @Produce     json
// @Param       id path string true "评论 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/comments/{id}/replies [get]
func listRepliesDoc() {}

// replyCommentDoc
// @Summary     回复评论
// @Description 回复指定评论
// @Tags        social-comments
// @Accept      json
// @Produce     json
// @Param       id   path string true "评论 ID"
// @Param       body body object{content=string} true "回复内容"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/comments/{id}/replies [post]
func replyCommentDoc() {}

// editCommentDoc
// @Summary     编辑评论
// @Description 编辑自己的评论（5 分钟内）
// @Tags        social-comments
// @Accept      json
// @Produce     json
// @Param       id   path string true "评论 ID"
// @Param       body body object{content=string} true "新内容"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/comments/{id} [patch]
func editCommentDoc() {}

// deleteCommentDoc
// @Summary     删除评论
// @Description 软删除评论（作者或游戏设计者）
// @Tags        social-comments
// @Produce     json
// @Param       id path string true "评论 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/comments/{id} [delete]
func deleteCommentDoc() {}

// voteCommentDoc
// @Summary     点赞评论
// @Tags        social-comments
// @Produce     json
// @Param       id path string true "评论 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/comments/{id}/vote [post]
func voteCommentDoc() {}

// unvoteCommentDoc
// @Summary     取消点赞评论
// @Tags        social-comments
// @Produce     json
// @Param       id path string true "评论 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/comments/{id}/vote [delete]
func unvoteCommentDoc() {}

// getCommentConfigDoc
// @Summary     获取评论配置
// @Description 获取游戏的评论区配置（default_mode 等）
// @Tags        social-comments
// @Produce     json
// @Param       id path string true "游戏 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/games/{id}/comment-config [get]
func getCommentConfigDoc() {}

// updateCommentConfigDoc
// @Summary     更新评论配置
// @Description 更新游戏的评论区配置
// @Tags        social-comments
// @Accept      json
// @Produce     json
// @Param       id   path string true "游戏 ID"
// @Param       body body object{default_mode=string} true "配置"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /create/games/{id}/comment-config [patch]
func updateCommentConfigDoc() {}
