package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// NotificationManager 通知管理器
type NotificationManager struct {
	telegramEnabled bool
	telegramToken   string
	telegramChatID  string
	proxyURL        string
	MeoWName        string // MeoW昵称，用于发送MeoW消息
}

var GlobalNotificationManager *NotificationManager

// NewNotificationManager 创建新的通知管理器
func NewNotificationManager(telegramEnabled bool, telegramToken, telegramChatID string, MeoWName string) *NotificationManager {
	return &NotificationManager{
		telegramEnabled: telegramEnabled,
		telegramToken:   telegramToken,
		telegramChatID:  telegramChatID,
		proxyURL:        "",
		MeoWName:        MeoWName,
	}
}

// NewNotificationManagerWithProxy 创建带代理的通知管理器
func NewNotificationManagerWithProxy(telegramEnabled bool, telegramToken, telegramChatID, proxyURL string, MeoWName string) *NotificationManager {
	return &NotificationManager{
		telegramEnabled: telegramEnabled,
		telegramToken:   telegramToken,
		telegramChatID:  telegramChatID,
		proxyURL:        proxyURL,
		MeoWName:        MeoWName,
	}
}

// SendSyncNotification 发送媒体库相关通知
func (nm *NotificationManager) SendSyncNotification(action string, name string, details ...string) {
	if !nm.telegramEnabled {
		return
	}

	var message string
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	switch action {
	case "sync_finish":
		message = fmt.Sprintf("✅ <b>%s 同步完成</b>\n\n", name)
		if len(details) > 0 {
			message += fmt.Sprintf("📊 <b>耗时:</b> %s, <b>生成STRM:</b> %s， <b>下载:</b> %s， <b>上传:</b> %s\n", details[0], details[1], details[2], details[3])
		}
		message += fmt.Sprintf("⏰ <b>时间:</b> %s", timestamp)

	case "error":
		message = "❌ <b>同步错误</b>\n\n"
		if len(details) > 0 {
			message += fmt.Sprintf("🔍 <b>错误:</b> %s\n", details[0])
		}
		message += fmt.Sprintf("⏰ <b>时间:</b> %s", timestamp)

	default:
		message = "ℹ️ <b>Q115-STRM通知</b>\n\n"
		message += fmt.Sprintf("📋 <b>动作:</b> %s\n", action)
		if len(details) > 0 {
			message += fmt.Sprintf("📝 <b>详情:</b> %s\n", details[0])
		}
		message += fmt.Sprintf("⏰ <b>时间:</b> %s", timestamp)
	}

	// 发送通知
	err := nm.sendTelegramMessage(message)
	if err != nil {
		AppLogger.Errorf("通知发送失败: %v", err)
	} else {
		AppLogger.Infof("通知发送成功: %s", action)
	}
}

// SendSystemNotification 发送系统相关通知
func (nm *NotificationManager) SendSystemNotification(title, content string) {
	if !nm.telegramEnabled {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf("🖥️ <b>%s</b>\n\n", title)
	message += fmt.Sprintf("📝 %s\n", content)
	message += fmt.Sprintf("⏰ <b>时间:</b> %s", timestamp)

	err := nm.sendTelegramMessage(message)
	if err != nil {
		AppLogger.Errorf("发送系统通知失败: %v", err)
	} else {
		AppLogger.Infof("系统通知发送成功: %s", title)
	}
}

// 发送刮削整理完成通知
func (nm *NotificationManager) SendRenamedNotification(poster, name, category, mediaType, resolution, seasonStr string) {
	if !nm.telegramEnabled {
		return
	}
	if poster == "" {
		return
	}
	// 下载海报
	posterPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s.jpg", name))
	derr := DownloadFile(poster, posterPath, "Q115-STRM")
	if derr != nil {
		AppLogger.Errorf("下载海报失败: %v", derr)
		return
	}
	// 删除临时文件
	defer os.Remove(posterPath)
	message := fmt.Sprintf("✅ <b>%s 刮削整理完成</b>\n\n", name)
	message += fmt.Sprintf("📊 <b>类型:</b> %s, <b>类别:</b> %s, <b>分辨率:</b> %s\n", mediaType, category, resolution)
	if seasonStr != "" {
		message += fmt.Sprintf("📺 <b>季集:</b> %s\n", seasonStr)
	}
	message += fmt.Sprintf("⏰ <b>时间:</b> %s", time.Now().Format("2006-01-02 15:04:05"))

	err := nm.SendCTelegramPhotoMessage(posterPath, message)
	if err != nil {
		AppLogger.Errorf("发送刮削整理完成通知失败: %v", err)
	} else {
		AppLogger.Infof("刮削整理完成通知发送成功: %s", name)
	}
}

// sendTelegramMessage 发送Telegram消息（支持代理和重试）
func (nm *NotificationManager) sendTelegramMessage(message string) error {
	if nm.proxyURL != "" {
		// 使用代理发送消息
		bot, err := NewTelegramBotWithProxy(nm.telegramToken, nm.telegramChatID, nm.proxyURL)
		if err != nil {
			return fmt.Errorf("创建代理Telegram机器人失败: %v", err)
		}
		// 使用重试机制，最多重试3次
		return bot.SendMessageWithRetry(message, 3)
	} else {
		// 不使用代理发送消息，也启用重试
		bot := NewTelegramBot(nm.telegramToken, nm.telegramChatID)
		return bot.SendMessageWithRetry(message, 2) // 不使用代理时重试次数少一些
	}
}

func (nm *NotificationManager) SendCTelegramPhotoMessage(photoURL, message string) error {
	if !nm.telegramEnabled {
		return nil
	}
	// 检查文件是否存在
	if _, err := os.Stat(photoURL); os.IsNotExist(err) {
		return fmt.Errorf("图片文件不存在: %s", photoURL)
	}
	var bot *TelegramBot
	if nm.proxyURL != "" {
		// 使用代理发送消息
		bot, _ = NewTelegramBotWithProxy(nm.telegramToken, nm.telegramChatID, nm.proxyURL)
	} else {
		// 不使用代理发送消息，也启用重试
		bot = NewTelegramBot(nm.telegramToken, nm.telegramChatID)
	}
	if bot == nil {
		return fmt.Errorf("创建Telegram机器人失败")
	}
	// 创建照片消息
	photo := tgbotapi.NewPhoto(StringToInt64(nm.telegramChatID), tgbotapi.FilePath(photoURL))
	photo.Caption = message

	// 支持多种文本格式
	photo.ParseMode = "HTML" // 或者 "MarkdownV2", "Markdown"

	// 发送消息
	_, err := bot.Client.Send(photo)
	if err != nil {
		return fmt.Errorf("发送图片消息失败: %v", err)
	}

	return nil
}
