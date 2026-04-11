package asset

// uploadAssetDoc
// @Summary     上传素材文件
// @Description 上传图片/音频文件到指定游戏 slug 目录（最大 10MB）
// @Tags        assets
// @Accept      multipart/form-data
// @Produce     json
// @Param       slug path string true "游戏 slug"
// @Param       file formData file true "素材文件（png/jpg/gif/webp/mp3/ogg/wav）"
// @Success     200 {object} map[string]any "url + filename + size + mime"
// @Failure     400 {object} map[string]any
// @Failure     415 {object} map[string]any "不支持的文件类型"
// @Security    BearerAuth
// @Router      /assets/{slug}/upload [post]
func uploadAssetDoc() {}
