package api

// listPostsDoc
// @Summary     帖子列表
// @Description 论坛帖子列表（支持游戏标签/排序/搜索）
// @Tags        social-forum
// @Produce     json
// @Param       game_tag query string false "按游戏标签过滤"
// @Param       sort     query string false "排序：hot（默认）| new"
// @Param       q        query string false "全文搜索"
// @Param       limit    query int    false "每页数量"
// @Param       offset   query int    false "偏移量"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/posts [get]
func listPostsDoc() {}

// createPostDoc
// @Summary     发帖
// @Description 创建论坛帖子
// @Tags        social-forum
// @Accept      json
// @Produce     json
// @Param       body body object{title=string,content=string,game_tag=string} true "帖子内容"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/posts [post]
func createPostDoc() {}

// getPostDoc
// @Summary     帖子详情
// @Description 按 ID 或 slug 获取帖子详情
// @Tags        social-forum
// @Produce     json
// @Param       id path string true "帖子 ID 或 slug"
// @Success     200 {object} map[string]any
// @Failure     404 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/posts/{id} [get]
func getPostDoc() {}

// editPostDoc
// @Summary     编辑帖子
// @Description 编辑自己的帖子
// @Tags        social-forum
// @Accept      json
// @Produce     json
// @Param       id   path string true "帖子 ID"
// @Param       body body object{title=string,content=string} true "更新内容"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/posts/{id} [patch]
func editPostDoc() {}

// deletePostDoc
// @Summary     删除帖子
// @Description 软删除帖子（仅作者）
// @Tags        social-forum
// @Produce     json
// @Param       id path string true "帖子 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/posts/{id} [delete]
func deletePostDoc() {}

// listPostRepliesDoc
// @Summary     帖子回复列表
// @Description 获取帖子的盖楼回复
// @Tags        social-forum
// @Produce     json
// @Param       id     path  string false "帖子 ID"
// @Param       limit  query int    false "每页数量"
// @Param       offset query int    false "偏移量"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/posts/{id}/replies [get]
func listPostRepliesDoc() {}

// replyPostDoc
// @Summary     回复帖子
// @Description 在帖子下盖楼
// @Tags        social-forum
// @Accept      json
// @Produce     json
// @Param       id   path string true "帖子 ID"
// @Param       body body object{content=string} true "回复内容"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/posts/{id}/replies [post]
func replyPostDoc() {}

// votePostDoc
// @Summary     点赞帖子
// @Tags        social-forum
// @Produce     json
// @Param       id path string true "帖子 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/posts/{id}/vote [post]
func votePostDoc() {}

// unvotePostDoc
// @Summary     取消点赞帖子
// @Tags        social-forum
// @Produce     json
// @Param       id path string true "帖子 ID"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /social/posts/{id}/vote [delete]
func unvotePostDoc() {}
