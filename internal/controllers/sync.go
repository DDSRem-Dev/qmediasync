package controllers

import (
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/synccron"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func StartSync(c *gin.Context) {
	// 启动同步
	synccron.StartSyncCron()
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "同步任务已添加到队列", Data: nil})
}

func GetSyncRecords(c *gin.Context) {
	type syncRecordsRequest struct {
		Page     int `form:"page" json:"page" binding:"omitempty,min=1"`           // 页码，默认1
		PageSize int `form:"page_size" json:"page_size" binding:"omitempty,min=1"` // 每页数量，默认50
	}

	var req syncRecordsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: 400, Message: "请求参数错误", Data: nil})
		return
	}
	page := req.Page
	pageSize := req.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}

	// 获取同步记录
	records, total, err := models.GetSyncRecords(page, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: 500, Message: "获取同步记录失败", Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取同步记录成功", Data: map[string]interface{}{
		"records": records,
		"total":   total,
	}})
}

// 返回同步任务的详情
func GetSyncTask(c *gin.Context) {
	type syncTaskRequest struct {
		SyncID uint `form:"sync_id" json:"sync_id" binding:"required"` // 同步任务ID
	}
	var req syncTaskRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: 400, Message: "请求参数错误", Data: nil})
		return
	}

	sync, err := models.GetSyncByID(req.SyncID)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: 500, Message: "获取同步任务失败", Data: nil})
		return
	}

	if sync == nil {
		c.JSON(http.StatusNotFound, APIResponse[any]{Code: 404, Message: "未找到对应的同步任务", Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取同步任务详情成功", Data: sync})
}

// GetSyncPathList 获取同步路径列表
func GetSyncPathList(c *gin.Context) {
	type syncPathListRequest struct {
		Page     int `form:"page" json:"page" binding:"omitempty,min=1"`           // 页码，默认1
		PageSize int `form:"page_size" json:"page_size" binding:"omitempty,min=1"` // 每页数量，默认20
	}
	var req syncPathListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误", Data: nil})
		return
	}
	page := req.Page
	pageSize := req.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	syncPaths, total := models.GetSyncPathList(page, pageSize, false)

	for _, sp := range syncPaths {
		sp.IsRunning = synccron.CheckTaskIsRunning(sp.ID, synccron.SyncTaskTypeStrm)
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取同步路径列表成功", Data: map[string]any{
		"list":      syncPaths,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}})
}

type addSyncPathRequest struct {
	SourceType   models.SourceType `json:"source_type" form:"source_type" binding:"required"` // 来源类型
	AccountId    uint              `json:"account_id" form:"account_id"`                      // 网盘账号ID
	BaseCid      string            `json:"base_cid" form:"base_cid" binding:"required"`       // 来源路径ID或者本地路径
	LocalPath    string            `json:"local_path" form:"local_path" binding:"required"`   // 本地路径
	RemotePath   string            `json:"remote_path" form:"remote_path" binding:"required"` // 同步源路径，115网盘和123网盘需要该字段
	EnableCron   bool              `json:"enable_cron" form:"enable_cron"`                    // 是否启用定时任务
	CustomConfig bool              `json:"custom_config" form:"custom_config"`                // 自定义配置
	models.SyncPathSetting
}

// AddSyncPath 添加同步路径
func AddSyncPath(c *gin.Context) {
	var req addSyncPathRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误", Data: nil})
		return
	}
	baseCid := req.BaseCid
	localPath := req.LocalPath
	if req.SourceType != models.SourceTypeLocal {
		// 检查accountId是否存在
		account, accountErr := models.GetAccountById(req.AccountId)
		if accountErr != nil || account == nil {
			c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号不存在", Data: nil})
			return
		}
		// 检查来源类型是否正确
		if req.SourceType != account.SourceType {
			c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号类型与同步源类型不一致", Data: nil})
			return
		}
	}
	remotePath := req.RemotePath
	if req.SourceType != models.SourceTypeLocal {
		remotePath = strings.TrimPrefix(req.RemotePath, "/")
	}
	if req.SourceType == models.SourceTypeOpenList {
		// 将remotepath中的\都替换为/
		remotePath = strings.ReplaceAll(remotePath, "\\", "/")
		baseCid = strings.ReplaceAll(req.BaseCid, "\\", "/")
	}
	// 创建同步路径
	syncPath := models.CreateSyncPath(req.SourceType, req.AccountId, baseCid, localPath, remotePath, req.EnableCron, req.CustomConfig, req.SyncPathSetting)
	if syncPath == nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "创建同步路径失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "添加同步路径成功", Data: syncPath})
}

// UpdateSyncPath 更新同步路径
func UpdateSyncPath(c *gin.Context) {
	type updateSyncPathRequest struct {
		ID uint `json:"id" form:"id"` // 同步路径ID
		addSyncPathRequest
	}
	var req updateSyncPathRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误", Data: nil})
		return
	}
	id := req.ID
	// 获取并更新同步路径
	syncPath := models.GetSyncPathById(uint(id))
	if syncPath == nil {
		c.JSON(http.StatusNotFound, APIResponse[any]{Code: BadRequest, Message: "同步路径不存在", Data: nil})
		return
	}
	if req.SourceType != models.SourceTypeLocal {
		// 检查accountId是否存在
		account, err := models.GetAccountById(syncPath.AccountId)
		if err != nil {
			c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号不存在", Data: nil})
			return
		}
		// 检查来源类型是否正确
		if req.SourceType != account.SourceType {
			c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号类型与同步源类型不一致", Data: nil})
			return
		}
	}
	if req.SourceType == models.SourceTypeOpenList {
		// 将remotepath中的\都替换为/
		req.RemotePath = strings.ReplaceAll(req.RemotePath, "\\", "/")
		req.BaseCid = strings.ReplaceAll(req.BaseCid, "\\", "/")
	}
	success := syncPath.Update(req.SourceType, req.AccountId, req.BaseCid, req.LocalPath, req.RemotePath, req.EnableCron, req.CustomConfig, req.SyncPathSetting)
	if !success {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "更新同步路径失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "更新同步路径成功", Data: syncPath})
}

// DeleteSyncPath 删除同步路径
func DeleteSyncPath(c *gin.Context) {
	type deleteSyncPathRequest struct {
		ID uint `json:"id" binding:"required"` // 同步路径ID
	}
	var req deleteSyncPathRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误", Data: nil})
		return
	}
	id := req.ID
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "id 参数不能为空", Data: nil})
		return
	}
	// 删除同步路径
	success := models.DeleteSyncPathById(id)
	if !success {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "删除同步路径失败", Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "删除同步路径成功", Data: nil})
}

// GetSyncPathById 根据ID获取同步路径详情
func GetSyncPathById(c *gin.Context) {
	type syncPathRequest struct {
		ID uint `form:"id" json:"id" binding:"required"` // 同步路径ID
	}
	var req syncPathRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误", Data: nil})
		return
	}

	id := req.ID
	if id == 0 {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "id 参数格式错误", Data: nil})
		return
	}

	syncPath := models.GetSyncPathById(uint(id))
	if syncPath == nil {
		c.JSON(http.StatusNotFound, APIResponse[any]{Code: BadRequest, Message: "同步路径不存在", Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取同步路径详情成功", Data: syncPath})
}

// 删除完成或失败的同步记录
func DelSyncRecords(c *gin.Context) {
	type delSyncRecordsRequest struct {
		IDs []uint `json:"ids" form:"ids" binding:"required"` // 同步路径ID列表
	}
	var req delSyncRecordsRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误", Data: nil})
		return
	}
	ids := req.IDs
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "没有选择删除的记录", Data: nil})
		return
	}
	for _, id := range ids {
		deleteErr := models.DeleteSyncRecordById(id)
		if deleteErr != nil {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "删除同步记录失败: " + deleteErr.Error(), Data: nil})
			continue
		}
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "删除同步记录成功", Data: nil})
}

// 启动某个同步目录的同步任务
func StartSyncByPath(c *gin.Context) {
	type startSyncRequest struct {
		ID uint `form:"id" json:"id" binding:"required"` // 同步路径ID
	}
	var req startSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误", Data: nil})
		return
	}
	if req.ID == 0 {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "id 参数不能为空", Data: nil})
		return
	}

	id := req.ID
	syncPath := models.GetSyncPathById(uint(id))
	if syncPath == nil {
		c.JSON(http.StatusNotFound, APIResponse[any]{Code: BadRequest, Message: "同步路径不存在", Data: nil})
		return
	}
	// syncPath.SetIsFullSync(false)
	if err := synccron.AddSyncTask(syncPath.ID, synccron.SyncTaskTypeStrm); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "添加同步任务失败: " + err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "同步任务已添加到队列", Data: nil})
}

func StopSyncByPath(c *gin.Context) {
	type startSyncRequest struct {
		ID uint `form:"id" json:"id" binding:"required"` // 同步路径ID
	}
	var req startSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误", Data: nil})
		return
	}
	if req.ID == 0 {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "id 参数不能为空", Data: nil})
		return
	}

	id := req.ID
	syncPath := models.GetSyncPathById(uint(id))
	if syncPath == nil {
		c.JSON(http.StatusNotFound, APIResponse[any]{Code: BadRequest, Message: "同步路径不存在", Data: nil})
		return
	}
	// syncPath.SetIsFullSync(false)
	synccron.StopSyncTask(syncPath.ID, synccron.SyncTaskTypeStrm)

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "同步任务已添加到队列", Data: nil})
}

// 关闭或开启同步目录的定时同步
func ToggleSyncByPath(c *gin.Context) {
	type stopSyncRequest struct {
		ID uint `form:"id" json:"id" binding:"required"` // 同步路径ID
	}
	var req stopSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误", Data: nil})
		return
	}
	if req.ID == 0 {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "id 参数不能为空", Data: nil})
		return
	}
	// 将enable参数设置为false
	syncPath := models.GetSyncPathById(req.ID)
	if syncPath == nil {
		c.JSON(http.StatusNotFound, APIResponse[any]{Code: BadRequest, Message: "同步路径不存在", Data: nil})
		return
	}
	syncPath.ToggleCron()
	if syncPath.EnableCron {
		c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "定时同步已开启", Data: nil})
	} else {
		c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "定时同步已关闭", Data: nil})
	}

}

// 启动115全量同步，删除本地缓存文件触发全量同步
func FullStart115Sync(c *gin.Context) {
	type startSyncRequest struct {
		ID uint `form:"id" json:"id" binding:"required"` // 同步路径ID
	}
	var req startSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误", Data: nil})
		return
	}
	if req.ID == 0 {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "id 参数不能为空", Data: nil})
		return
	}
	id := req.ID
	syncPath := models.GetSyncPathById(uint(id))
	if syncPath == nil {
		c.JSON(http.StatusNotFound, APIResponse[any]{Code: BadRequest, Message: "同步路径不存在", Data: nil})
		return
	}
	// 删除所有的数据库记录，重新查询接口
	if syncPath.SourceType == models.SourceType115 {
		// 清空数据表
		if err := models.DeleteAllFileBySyncPathId(syncPath.ID); err != nil {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "清空同步目录的数据表失败: " + err.Error(), Data: nil})
			return
		}
	}
	syncPath.SetIsFullSync(true)
	if err := synccron.AddSyncTask(syncPath.ID, synccron.SyncTaskTypeStrm); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "添加同步任务失败: " + err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "同步任务已添加到队列", Data: nil})
}
