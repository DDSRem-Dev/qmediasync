package controllers

import (
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/synccron"
	"net/http"

	"github.com/gin-gonic/gin"
)

func UpdateEmby(c *gin.Context) {
	type updateEmbyRequest struct {
		EmbyUrl    string `form:"emby_url" json:"emby_url"`         // Emby Url
		EmbyApiKey string `form:"emby_api_key" json:"emby_api_key"` // Emby API Key
	}
	// 获取请求参数
	var req updateEmbyRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}
	// 更新设置
	if !models.SettingsGlobal.UpdateEmbyUrl(req.EmbyUrl, req.EmbyApiKey) {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "更新Emby Url失败", Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "更新Emby Url成功", Data: nil})
}

func GetEmby(c *gin.Context) {
	// 获取设置
	models.LoadSettings() // 确保设置已加载
	emby := make(map[string]string)
	emby["emby_url"] = models.SettingsGlobal.EmbyUrl
	emby["emby_api_key"] = models.SettingsGlobal.EmbyApiKey
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取Emby设置成功", Data: emby})
}

func ParseEmby(c *gin.Context) {
	if models.SettingsGlobal.EmbyUrl == "" || models.SettingsGlobal.EmbyApiKey == "" {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "Emby Url和Emby API Key没有填写，无法提取媒体信息", Data: nil})
		return
	}
	if synccron.EmbyMediaInfoStart {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "Emby媒体信息解析任务已在运行", Data: nil})
		return
	}
	synccron.StartParseEmbyMediaInfo()
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "解析Emby媒体信息成功", Data: nil})
}

func UpdateTelegram(c *gin.Context) {
	type updateTelegramRequest struct {
		Enabled int    `form:"enabled" json:"enabled"` // 是否启用Telegram通知，"1"表示启用，"0"表示禁用
		Token   string `form:"token" json:"token"`     // Telegram Bot的Token
		ChatId  string `form:"chat_id" json:"chat_id"` // Telegram Chat ID
	}
	// 获取请求参数
	var req updateTelegramRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}
	enabled := req.Enabled == 1
	token := req.Token
	chatId := req.ChatId

	// 如果启用Telegram，则需要验证token和chatId
	if enabled && (token == "" || chatId == "") {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "启用Telegram通知时，Token和Chat ID不能为空", Data: nil})
		return
	}
	// 更新设置
	if !models.SettingsGlobal.UpdateTelegramBot(enabled, token, chatId) {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "更新Telegram Bot设置失败", Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "更新Telegram Bot设置成功", Data: nil})
}

func GetTelegram(c *gin.Context) {
	// 获取设置
	models.LoadSettings() // 确保设置已加载
	telegramBot := make(map[string]string)
	if models.SettingsGlobal.UseTelegram == 1 {
		telegramBot["enabled"] = "1"
	} else {
		telegramBot["enabled"] = "0"
	}
	telegramBot["token"] = models.SettingsGlobal.TelegramBotToken
	telegramBot["chat_id"] = models.SettingsGlobal.TelegramChatId
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取Telegram Bot设置成功", Data: telegramBot})
}

func UpdateHttpProxy(c *gin.Context) {
	type updateHttpProxyRequest struct {
		HttpProxy string `form:"http_proxy" json:"http_proxy"` // HTTP代理地址
	}
	var req updateHttpProxyRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}
	httpProxy := req.HttpProxy
	// 更新设置
	if !models.SettingsGlobal.UpdateHttpProxy(httpProxy) {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "更新HTTP代理设置失败", Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "更新HTTP代理设置成功", Data: nil})
}

func GetHttpProxy(c *gin.Context) {
	// 获取设置
	models.LoadSettings() // 确保设置已加载
	httpProxy := make(map[string]string)
	httpProxy["http_proxy"] = models.SettingsGlobal.HttpProxy
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取HTTP代理设置成功", Data: httpProxy})
}

func TestHttpProxy(c *gin.Context) {
	type testHttpProxyRequest struct {
		HttpProxy string `form:"http_proxy" json:"http_proxy" binding:"required"` // HTTP代理地址
		Detailed  int    `form:"detailed" json:"detailed"`                        // 是否返回详细测试结果，"1"表示返回，"0"表示不返回
	}
	var req testHttpProxyRequest
	// 获取请求参数
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}
	httpProxy := req.HttpProxy
	detailed := req.Detailed == 1

	// 数据校验
	if httpProxy == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "HTTP代理地址不能为空", Data: nil})
		return
	}

	if detailed {
		// 使用高级测试，返回详细结果
		result, err := helpers.TestHttpProxyAdvanced(httpProxy)
		if err != nil {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "连接失败: " + err.Error(), Data: nil})
			return
		}

		if result.Success {
			c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "HTTP代理连接测试成功", Data: result})
		} else {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "连接失败: " + result.ErrorMessage, Data: nil})
		}
	} else {
		// 使用简单测试
		success, err := helpers.TestHttpProxy(httpProxy)
		if err != nil {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "连接失败: " + err.Error(), Data: nil})
			return
		}

		if success {
			c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "HTTP代理连接测试成功", Data: nil})
		} else {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "HTTP代理连接测试失败", Data: nil})
		}
	}
}

func TestTelegram(c *gin.Context) {
	type testTelegramRequest struct {
		Token  string `form:"token" json:"token" binding:"required"`     // Telegram Bot的Token
		ChatId string `form:"chat_id" json:"chat_id" binding:"required"` // Telegram Chat ID
	}
	// 获取请求参数
	var req testTelegramRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}
	token := req.Token
	chatId := req.ChatId

	// 数据校验
	if token == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "Telegram Bot Token不能为空", Data: nil})
		return
	}
	if chatId == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "Telegram Chat ID不能为空", Data: nil})
		return
	}

	// 测试Telegram机器人连接
	err := helpers.TestTelegramBot(token, chatId, models.SettingsGlobal.HttpProxy)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "连接失败: " + err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "Telegram机器人连接测试成功", Data: nil})
}

func GetStrmConfig(c *gin.Context) {
	// 获取设置
	models.LoadSettings() // 确保设置已加载
	strmConfig := make(map[string]interface{})
	strmConfig["strm_base_url"] = models.SettingsGlobal.StrmBaseUrl
	if models.SettingsGlobal.Cron == "" {
		strmConfig["cron"] = helpers.GlobalConfig.Strm.Cron // 使用默认配置
	} else {
		strmConfig["cron"] = models.SettingsGlobal.Cron
	}
	if models.SettingsGlobal.MetaExt != "" {
		strmConfig["meta_ext"] = models.SettingsGlobal.MetaExtArr
	} else {
		// 从config.yml中读取默认的metaExt
		strmConfig["meta_ext"] = helpers.GlobalConfig.Strm.MetaExt
	}
	if models.SettingsGlobal.VideoExt != "" {
		strmConfig["video_ext"] = models.SettingsGlobal.VideoExtArr
	} else {
		// 从config.yml中读取默认的视频扩展名
		strmConfig["video_ext"] = helpers.GlobalConfig.Strm.VideoExt
	}
	strmConfig["exclude_name"] = models.SettingsGlobal.ExcludeNameArr
	strmConfig["min_video_size"] = models.SettingsGlobal.MinVideoSize
	strmConfig["upload_meta"] = models.SettingsGlobal.UploadMeta
	strmConfig["delete_dir"] = models.SettingsGlobal.DeleteDir
	strmConfig["local_proxy"] = models.SettingsGlobal.LocalProxy
	strmConfig["exclude_name"] = models.SettingsGlobal.ExcludeNameArr
	strmConfig["download_meta"] = models.SettingsGlobal.DownloadMeta
	strmConfig["add_path"] = models.SettingsGlobal.AddPath
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取STRM配置成功", Data: strmConfig})
}

func UpdateStrmConfig(c *gin.Context) {
	type updateStrmConfigRequest struct {
		StrmBaseUrl  string   `form:"strm_base_url" json:"strm_base_url" binding:"required"` // STRM基础URL
		Cron         string   `form:"cron" json:"cron" binding:"required"`                   // Cron表达式
		MetaExt      []string `form:"meta_ext" json:"meta_ext" binding:"required"`           // 元数据扩展名，JSON数组字符串格式，例如：["nfo","txt"]
		VideoExt     []string `form:"video_ext" json:"video_ext" binding:"required"`         // 视频扩展名，JSON数组字符串格式，例如：["mp4","mkv"]
		MinVideoSize int64    `form:"min_video_size" json:"min_video_size"`                  // 最小视频大小，单位：MB
		UploadMeta   int      `form:"upload_meta" json:"upload_meta"`                        // 是否上传元数据文件，"1"表示上传，"0"表示不上传
		DeleteDir    int      `form:"delete_dir" json:"delete_dir"`                          // 是否删除空目录，"1"表示删除，"0"表示不删除
		LocalProxy   int      `form:"local_proxy" json:"local_proxy"`                        // 是否启用本地代理，"1"表示启用，"0"表示禁用
		ExcludeName  []string `form:"exclude_name" json:"exclude_name"`                      // 排除的文件名，JSON数组字符串格式，例如：["sample","test"]
		DownloadMeta int      `form:"download_meta" json:"download_meta"`                    // 是否下载元数据文件，"1"表示下载，"0"表示不下载
		AddPath      int      `form:"add_path" json:"add_path"`                              // 是否添加路径，1- 表示添加路径， 2-表示不添加路径
	}
	// 获取请求参数
	var req updateStrmConfigRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}
	oldCron := models.SettingsGlobal.Cron
	// 获取请求参数
	strmBaseUrl := req.StrmBaseUrl
	cron := req.Cron
	metaExt := req.MetaExt
	videoExt := req.VideoExt
	minVideoSize := req.MinVideoSize
	uploadMeta := req.UploadMeta
	deleteDir := req.DeleteDir
	localProxy := req.LocalProxy
	excludeName := req.ExcludeName
	downloadMeta := req.DownloadMeta
	addPath := req.AddPath
	// 数据校验
	if strmBaseUrl == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "STRM基础URL不能为空", Data: nil})
		return
	}
	if cron == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "Cron表达式不能为空", Data: nil})
		return
	}
	if minVideoSize < 0 {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "最小视频大小必须大于等于0", Data: nil})
		return
	}
	// 检查cron是否正确，并且不能小于1小时一次
	runTimes := helpers.GetNextTimeByCronStr(cron, 2)
	if runTimes == nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "Cron表达式格式不正确", Data: nil})
		return
	}
	// 更新设置
	if !models.SettingsGlobal.UpdateStrm(strmBaseUrl, cron, metaExt, videoExt, minVideoSize, uploadMeta, deleteDir, localProxy, excludeName, downloadMeta, addPath) {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "更新STRM配置失败", Data: nil})
		return
	}
	if oldCron != models.SettingsGlobal.Cron {
		// 如果Cron发生变化，重启任务
		synccron.InitCron()
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "更新STRM配置成功", Data: nil})
}

func GetCronNextTime(c *gin.Context) {
	type getCronNextTimeRequest struct {
		Cron string `form:"cron" json:"cron" binding:"required"` // Cron表达式
	}
	var req getCronNextTimeRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}
	times := helpers.GetNextTimeByCronStr(req.Cron, 5)
	if times == nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "Cron表达式格式不正确", Data: nil})
		return
	}
	var timeStrs []string
	for _, t := range times {
		timeStrs = append(timeStrs, t.Format("2006-01-02 15:04:05"))
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取下次执行时间成功", Data: timeStrs})
}

func GetThreads(c *gin.Context) {
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取线程数成功", Data: models.SettingsGlobal.GetThreads()})
}

func UpdateThreads(c *gin.Context) {
	type updateThreadsRequest struct {
		DownloadThreads   int `form:"download_threads" json:"download_threads" binding:"required"`       // 下载线程数
		FileDetailThreads int `form:"file_detail_threads" json:"file_detail_threads" binding:"required"` // 查询文件详情的线程数
	}
	var req updateThreadsRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}
	downloadThreads := req.DownloadThreads
	fileDetailThreads := req.FileDetailThreads
	// 更新设置
	if !models.SettingsGlobal.UpdateThreads(downloadThreads, fileDetailThreads) {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "更新线程数失败", Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "更新线程数成功", Data: nil})
}
