package controllers

import (
	"Q115-STRM/internal/db"
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/v115open"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type v115StatusResp struct {
	LoggedIn    bool        `json:"logged_in"`
	UserId      json.Number `json:"user_id"`
	Username    string      `json:"username"`
	UsedSpace   int64       `json:"used_space"`
	TotalSpace  int64       `json:"total_space"`
	MemberLevel string      `json:"member_level"`
	ExpireTime  string      `json:"expire_time"`
}

type KeyLockWithTimeout struct {
	mutexes sync.Map // key -> *sync.Mutex
	global  sync.Mutex
}

// LockWithTimeout 尝试获取锁，如果超时则返回 false
func (kl *KeyLockWithTimeout) LockWithTimeout(key string, timeout time.Duration) bool {
	kl.global.Lock()
	mutex, _ := kl.mutexes.LoadOrStore(key, &sync.Mutex{})
	kl.global.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return false // 超时
		default:
			if mutex.(*sync.Mutex).TryLock() {
				return true // 成功获取锁
			}
			time.Sleep(10 * time.Millisecond) // 短暂等待后重试
		}
	}
}

func (kl *KeyLockWithTimeout) Unlock(key string) {
	kl.global.Lock()
	mutex, ok := kl.mutexes.Load(key)
	kl.global.Unlock()

	if ok {
		mutex.(*sync.Mutex).Unlock()
	}
}

func Get115Status(c *gin.Context) {
	type statusReq struct {
		AccountId uint `json:"account_id" form:"account_id"`
	}
	var req statusReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	account, err := models.GetAccountById(req.AccountId)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号ID不存在", Data: nil})
		return
	}
	client := account.Get115Client(true)
	var resp v115StatusResp
	// 获取用户信息
	userInfo, err := client.UserInfo()
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取115用户信息失败: " + err.Error(), Data: nil})
		return
	}
	resp.LoggedIn = true
	resp.UserId = userInfo.UserId
	resp.Username = userInfo.UserName
	resp.UsedSpace = userInfo.RtSpaceInfo.AllUse.Size
	resp.TotalSpace = userInfo.RtSpaceInfo.AllTotal.Size
	resp.MemberLevel = userInfo.VipInfo.LevelName
	if userInfo.VipInfo.Expire > 0 {
		resp.ExpireTime = helpers.FormatTimestamp(userInfo.VipInfo.Expire)
	} else {
		resp.ExpireTime = "未开通会员"
	}
	// 返回状态信息
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取115状态成功", Data: resp})
}

func GetFileDetail(c *gin.Context) {
	type fileDetailReq struct {
		AccountId uint   `json:"account_id" form:"account_id"`
		FileId    string `json:"file_id" form:"file_id"`
	}
	var req fileDetailReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	account, err := models.GetAccountById(req.AccountId)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号ID不存在", Data: nil})
		return
	}
	fullPath := models.GetPathByPathFileId(account, req.FileId)
	if fullPath == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "文件ID不存在或未找到对应路径", Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取文件详情成功", Data: fullPath})
}

var keyLock KeyLockWithTimeout

// 查询并302跳转到115文件直链
// 请求下载链接的user-agent必须跟访问下载链接的user-agent相同
func Get115FileUrl(c *gin.Context) {
	type fileUrlReq struct {
		AccountId uint   `json:"account_id" form:"account_id"`
		PickCode  string `json:"pick_code" form:"pick_code"`
		Force     int    `json:"force" form:"force"`
	}

	ua := c.Request.UserAgent()
	var req fileUrlReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	if req.PickCode == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "pick_code 参数不能为空", Data: nil})
		return
	}
	helpers.AppLogger.Infof("检查是否具有302播放标记， force=%d", req.Force)
	cacheKey := fmt.Sprintf("115url:%s, ua=%s", req.PickCode, ua)
	helpers.AppLogger.Infof("准备获取115文件下载链接: pickcode=%s, ua=%s，8095播放=%d 加锁10秒", req.PickCode, ua, req.Force)
	if keyLock.LockWithTimeout(cacheKey, 10*time.Second) {
		defer keyLock.Unlock(cacheKey)
		account, err := models.GetAccountById(req.AccountId)
		if err != nil {
			c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号ID不存在", Data: nil})
			return
		}
		client := account.Get115Client(true)

		// helpers.AppLogger.Debugf("是否启用本地代理：%d", models.SettingsGlobal.LocalProxy)
		if req.Force == 0 && models.SettingsGlobal.LocalProxy == 1 {
			// 跳转到本地代理时使用统一的UA
			ua = v115open.DEFAULTUA
			helpers.AppLogger.Infof("因为8095标识=%d, 本地播放代理开关=%d，所以使用默认UA: %s", req.Force, models.SettingsGlobal.LocalProxy, ua)
		}
		cachedUrl := string(db.Cache.Get(cacheKey))
		if cachedUrl == "" {
			cachedUrl = client.GetDownloadUrl(context.Background(), req.PickCode, ua)
			if cachedUrl == "" {
				c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取115下载链接失败", Data: nil})
				return
			}
			helpers.AppLogger.Infof("从接口中查询到115下载链接: pickcode=%s, ua=%s => %s", req.PickCode, ua, cachedUrl)
			// 缓存半小时
			db.Cache.Set(cacheKey, []byte(cachedUrl), 1800)
		} else {
			helpers.AppLogger.Infof("从缓存中查询到115下载链接: pickcode=%s, ua=%s => %s", req.PickCode, ua, cachedUrl)
		}
		if req.Force == 0 {
			if models.SettingsGlobal.LocalProxy == 1 {
				// 跳转到本地代理
				helpers.AppLogger.Infof("通过本地代理访问115下载链接，非302播放: %s", cachedUrl)
				proxyUrl := fmt.Sprintf("/proxy-115?url=%s", url.QueryEscape(cachedUrl))
				c.Redirect(http.StatusFound, proxyUrl)
			} else {
				helpers.AppLogger.Infof("302重定向到115下载链接，非302播放: %s", cachedUrl)
				c.Redirect(http.StatusFound, cachedUrl)
			}
		} else {
			helpers.AppLogger.Infof("302重定向到115下载链接， 302播放: %s", cachedUrl)
			c.Redirect(http.StatusFound, cachedUrl)
		}
	}
}

// 获取开放平台登录二维码链接
func GetLoginQrCodeOpen(c *gin.Context) {
	type qrcodeReq struct {
		AccountId uint `json:"account_id" form:"account_id"`
	}
	var req qrcodeReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	account, err := models.GetAccountById(req.AccountId)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号ID不存在", Data: nil})
		return
	}
	client := account.Get115Client(true)
	qrCodeData, err := client.GetQrCode()
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取二维码失败: " + err.Error(), Data: nil})
		return
	}
	// 轮询二维码状态
	go func(codeData *v115open.QrCodeDataReturn) {
		for {
			status, err := client.QrCodeScanStatus(&codeData.QrCodeData)
			if err != nil {
				helpers.AppLogger.Errorf("刷新二维码状态失败: %v", err)
				break
			}
			// 写入缓存（假设有 db.Cache.Set 方法，缓存60秒）
			db.Cache.Set("qr_status:"+codeData.Uid, []byte(strconv.Itoa(int(status))), 60)
			if status == v115open.QrCodeScanStatusExpired {
				// 二维码已过期，重新获取
				helpers.AppLogger.Info("二维码已过期，重新获取")
				break
			}
			if status == v115open.QrCodeScanStatusConfirmed {
				// 二维码已确认，结束轮询，并且获取token
				helpers.AppLogger.Info("二维码已确认，结束轮询")
				openToken, err := client.GetToken(codeData)
				if err != nil || openToken == nil {
					c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取token失败: " + err.Error(), Data: nil})
					return
				}
				// 保存token
				account.UpdateToken(openToken.AccessToken, openToken.RefreshToken, openToken.ExpiresIn)
				userInfo, err := client.UserInfo()
				if err != nil {
					c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取115用户信息失败: " + err.Error(), Data: nil})
					return
				}
				rs := account.UpdateUser(userInfo.UserId, userInfo.UserName)
				if !rs {
					c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "更新用户信息失败", Data: nil})
					return
				}
				break
			}
			// 每5秒刷新一次
			time.Sleep(5 * time.Second)
		}
	}(qrCodeData)
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取二维码成功", Data: qrCodeData})
}

// 查询二维码扫码状态
func GetQrCodeStatus(c *gin.Context) {
	type qrcodeReq struct {
		Uid       string `json:"uid" form:"uid"`
		AccountId uint   `json:"account_id" form:"account_id"`
	}
	var req qrcodeReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	uid := req.Uid
	s := v115open.QrCodeScanStatusNotScanned
	cachedStatus := db.Cache.Get("qr_status:" + uid)
	if cachedStatus != nil {
		statusInt, err := strconv.Atoi(string(cachedStatus))
		if err == nil {
			s = v115open.QrCodeScanStatus(statusInt)
		}
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "", Data: map[string]v115open.QrCodeScanStatus{"status": s}})
}

func Get115UrlByPickCode(c *gin.Context) {
	type fileIdReq struct {
		UserId   string `json:"userid" form:"userid"`
		PickCode string `json:"pickcode" form:"pickcode"`
		Force    int    `json:"force" form:"force"`
	}
	var req fileIdReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	pickCode := req.PickCode
	userId := req.UserId
	var account *models.Account
	if userId == "" {
		// 查询SyncFile
		syncFile := models.GetFileByPickCode(pickCode)
		if syncFile == nil {
			c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "文件PickCode不存在", Data: nil})
			return
		}
		var err error
		account, err = models.GetAccountById(syncFile.AccountId)
		if err != nil {
			c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号ID不存在", Data: nil})
			return
		}
		helpers.AppLogger.Infof("通过PickCode查询到115账号: %s", account.Username)
	} else {
		var err error
		// 通过userId查询账号
		account, err = models.GetAccountByUserId(json.Number(userId))
		if err != nil {
			c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "用户ID不存在", Data: nil})
			return
		}
		helpers.AppLogger.Infof("通过用户ID查询到115账号: %s", account.Username)
	}
	ua := c.Request.UserAgent()
	client := account.Get115Client(true)
	helpers.AppLogger.Infof("检查是否具有直链播放标记， force=%d", req.Force)
	cacheKey := fmt.Sprintf("115url:%s, ua=%s", pickCode, ua)
	helpers.AppLogger.Infof("准备获取115文件下载链接: pickcode=%s, ua=%s，8095播放=%d 加锁10秒", pickCode, ua, req.Force)
	if keyLock.LockWithTimeout(cacheKey, 10*time.Second) {
		defer keyLock.Unlock(cacheKey)
		// helpers.AppLogger.Debugf("是否启用本地代理：%d", models.SettingsGlobal.LocalProxy)
		if req.Force == 0 && models.SettingsGlobal.LocalProxy == 1 {
			// 跳转到本地代理时使用统一的UA
			ua = v115open.DEFAULTUA
			helpers.AppLogger.Infof("因为直链标识=%d, 本地播放代理开关=%d，所以使用默认UA: %s", req.Force, models.SettingsGlobal.LocalProxy, ua)
		}
		cachedUrl := string(db.Cache.Get(cacheKey))
		if cachedUrl == "" {
			cachedUrl = client.GetDownloadUrl(context.Background(), pickCode, ua)
			if cachedUrl == "" {
				c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取115下载链接失败", Data: nil})
				return
			}
			helpers.AppLogger.Infof("从接口中查询到115下载链接: pickcode=%s, ua=%s => %s", pickCode, ua, cachedUrl)
			// 缓存半小时
			db.Cache.Set(cacheKey, []byte(cachedUrl), 1800)
		} else {
			helpers.AppLogger.Infof("从缓存中查询到115下载链接: pickcode=%s, ua=%s => %s", pickCode, ua, cachedUrl)
		}
		if req.Force == 0 {
			if models.SettingsGlobal.LocalProxy == 1 {
				// 跳转到本地代理
				helpers.AppLogger.Infof("通过本地代理访问115下载链接，非qms 8095播放: %s", cachedUrl)
				proxyUrl := fmt.Sprintf("/proxy-115?url=%s", url.QueryEscape(cachedUrl))
				c.Redirect(http.StatusFound, proxyUrl)
			} else {
				helpers.AppLogger.Infof("302重定向到115下载链接，非直链qms 8095播放: %s", cachedUrl)
				c.Redirect(http.StatusFound, cachedUrl)
			}
		} else {
			helpers.AppLogger.Infof("302重定向到115下载链接， 直链qms 8095播放: %s", cachedUrl)
			c.Redirect(http.StatusFound, cachedUrl)
		}
	}
}
