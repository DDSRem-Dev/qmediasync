package controllers

import (
	"Q115-STRM/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 上传队列列表
func UploadList(ctx *gin.Context) {
	type uploadListReq struct {
		Status   models.UploadStatus `json:"status" form:"status"`
		Page     int                 `json:"page" form:"page"`
		PageSize int                 `json:"page_size" form:"page_size"`
	}
	var req uploadListReq
	if err := ctx.ShouldBind(&req); err != nil {
		ctx.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "请求参数错误", Data: nil})
		return
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 100
	}
	// 从请求中获取文件列表
	// 从model/upload.go中查询上传队列列表
	type uploadQueueResp struct {
		Total     int                    `json:"total"`
		Uploading int                    `json:"uploading"`
		List      []*models.DbUploadTask `json:"list"`
	}
	// 从请求中获取文件列表
	// 从model/upload.go中查询上传队列列表
	uploadList, total := models.GetUploadTaskList(req.Status, req.Page, req.PageSize)
	ctx.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "上传队列列表查询成功", Data: uploadQueueResp{
		Total:     int(total),
		Uploading: int(models.GetUploadingCount()),
		List:      uploadList,
	}})
}

// 清除上传队列中所有未开始的任务
func ClearPendingUploadTasks(ctx *gin.Context) {
	// 调用全局上传队列的ClearPendingTasks方法
	err := models.ClearPendingUploadTasks()
	if err != nil {
		ctx.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "清除待上传任务失败", Data: nil})
		return
	}

	// 返回结果
	ctx.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "成功清除待上传任务", Data: nil})
}

func ClearUploadSuccessAndFailedTasks(ctx *gin.Context) {
	// 调用全局上传队列的DeleteSuccessAndFailed方法
	err := models.ClearUploadSuccessAndFailed()
	if err != nil {
		ctx.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "删除成功和失败任务失败", Data: nil})
		return
	}

	// 返回结果
	ctx.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "成功删除成功和失败任务", Data: nil})
}

// 启动上传队列
func StartUploadQueue(ctx *gin.Context) {
	// 调用全局上传队列的Start方法
	models.GlobalUploadQueue.Restart()

	// 返回结果
	ctx.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "下载队列已启动", Data: nil})
}

func StopUploadQueue(ctx *gin.Context) {
	// 调用全局上传队列的Stop方法
	models.GlobalUploadQueue.Stop()

	// 返回结果
	ctx.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "上传队列已停止", Data: nil})
}

func UploadQueueStatus(ctx *gin.Context) {
	// 调用全局上传队列的GetStatus方法
	status := models.GlobalUploadQueue.IsRunning()

	// 返回结果
	ctx.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "下载队列状态查询成功", Data: status})
}

// 下载队列列表
func DownloadList(ctx *gin.Context) {
	type downloadListReq struct {
		Status   models.DownloadStatus `json:"status" form:"status"`
		Page     int                   `json:"page" form:"page"`
		PageSize int                   `json:"page_size" form:"page_size"`
	}
	type downloadQueueResp struct {
		Total       int64                    `json:"total"`
		Downloading int64                    `json:"downloading"`
		List        []*models.DbDownloadTask `json:"list"`
	}
	var req downloadListReq
	if err := ctx.ShouldBind(&req); err != nil {
		ctx.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "请求参数错误", Data: nil})
		return
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 100
	}
	// 从请求中获取文件列表
	// 从model/download.go中查询下载队列列表
	downloadList, total := models.GetDownloadTaskList(req.Status, req.Page, req.PageSize)
	ctx.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "下载队列列表查询成功", Data: downloadQueueResp{
		Total:       total,
		Downloading: models.GetDownloadingCount(),
		List:        downloadList,
	}})
}

// 清除下载队列中所有已完成和失败的任务
func ClearPendingDownloadTasks(ctx *gin.Context) {
	// 调用全局下载队列的ClearPendingTasks方法
	err := models.ClearDownloadPendingTasks()
	if err != nil {
		ctx.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "清除下载任务失败", Data: nil})
		return
	}

	// 返回结果
	ctx.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "成功清除下载任务", Data: nil})
}

// 启动下载队列
func StartDownloadQueue(ctx *gin.Context) {
	// 调用全局下载队列的Start方法
	models.GlobalDownloadQueue.Restart()

	// 返回结果
	ctx.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "下载队列已启动", Data: nil})
}

func StopDownloadQueue(ctx *gin.Context) {
	// 调用全局下载队列的Stop方法
	models.GlobalDownloadQueue.Stop()

	// 返回结果
	ctx.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "下载队列已停止", Data: nil})
}

func DownloadQueueStatus(ctx *gin.Context) {
	// 调用全局下载队列的GetStatus方法
	status := models.GlobalDownloadQueue.IsRunning()

	// 返回结果
	ctx.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "下载队列状态查询成功", Data: status})
}

func ClearDownloadSuccessAndFailedTasks(ctx *gin.Context) {
	// 调用全局下载队列的DeleteSuccessAndFailed方法
	err := models.ClearDownloadSuccessAndFailed()
	if err != nil {
		ctx.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "删除成功和失败任务失败", Data: nil})
		return
	}

	// 返回结果
	ctx.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "成功删除成功和失败任务", Data: nil})
}
