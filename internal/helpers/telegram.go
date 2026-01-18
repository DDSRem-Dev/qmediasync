package helpers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TelegramBot 结构体用于处理Telegram机器人操作
type TelegramBot struct {
	Token  string
	ChatID string
	Client *tgbotapi.BotAPI
}

// TelegramResponse Telegram API响应结构
type TelegramResponse struct {
	OK          bool        `json:"ok"`
	Result      interface{} `json:"result"`
	ErrorCode   int         `json:"error_code"`
	Description string      `json:"description"`
}

// TelegramMessage 发送消息的结构
type TelegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

// NewTelegramBot 创建新的Telegram机器人实例
func NewTelegramBot(token, chatID string) *TelegramBot {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil
	}
	return &TelegramBot{
		Token:  token,
		ChatID: chatID,
		Client: bot,
	}
}

// NewTelegramBotWithProxy 创建带代理的Telegram机器人实例
func NewTelegramBotWithProxy(token, chatID, proxyURL string) (*TelegramBot, error) {
	// 增加超时时间以适配代理连接
	client := &http.Client{
		Timeout: 120 * time.Second, // 增加总超时时间
	}

	// 如果提供了代理URL，配置代理
	if proxyURL != "" {
		transport, err := createProxyTransport(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("创建代理传输失败: %v", err)
		}
		client.Transport = transport
	}
	bot, err := tgbotapi.NewBotAPIWithClient(token, "https://api.telegram.org/bot%s/%s", client)
	if err != nil {
		return nil, err
	}

	return &TelegramBot{
		Token:  token,
		ChatID: chatID,
		Client: bot,
	}, nil
}

// SendMessage 发送消息到Telegram
func (bot *TelegramBot) SendMessage(text string) error {
	if bot == nil {
		return fmt.Errorf("telegram bot 实例不能为空")
	}
	if bot.Client == nil {
		return fmt.Errorf("telegram bot client 不能为空")
	}
	if bot.Token == "" {
		return fmt.Errorf("telegram bot token不能为空")
	}
	if bot.ChatID == "" {
		return fmt.Errorf("telegram chat ID不能为空")
	}

	msg := tgbotapi.NewMessage(StringToInt64(bot.ChatID), text)
	msg.ParseMode = "HTML"
	_, err := bot.Client.Send(msg)
	if err != nil {
		return fmt.Errorf("发送消息失败: %v", err)
	}
	return nil

	// url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", bot.Token)

	// message := TelegramMessage{
	// 	ChatID:    bot.ChatID,
	// 	Text:      text,
	// 	ParseMode: "HTML",
	// }

	// jsonData, err := json.Marshal(message)
	// if err != nil {
	// 	return fmt.Errorf("序列化消息失败: %v", err)
	// }

	// req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	// if err != nil {
	// 	return fmt.Errorf("创建请求失败: %v", err)
	// }

	// req.Header.Set("Content-Type", "application/json")

	// resp, err := bot.Client.Do(req)
	// if err != nil {
	// 	return fmt.Errorf("发送请求失败: %v", err)
	// }
	// defer resp.Body.Close()

	// body, err := io.ReadAll(resp.Body)
	// if err != nil {
	// 	return fmt.Errorf("读取响应失败: %v", err)
	// }

	// var telegramResp TelegramResponse
	// if err := json.Unmarshal(body, &telegramResp); err != nil {
	// 	return fmt.Errorf("解析响应失败: %v", err)
	// }

	// if !telegramResp.OK {
	// 	return fmt.Errorf("telegram API错误 [%d]: %s", telegramResp.ErrorCode, telegramResp.Description)
	// }

	// return nil
}

// SendMessageWithRetry 带重试机制的发送消息
func (bot *TelegramBot) SendMessageWithRetry(text string, maxRetries int) error {
	var lastError error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// 重试前等待，使用指数退避
			waitTime := time.Duration(attempt*attempt) * time.Second
			AppLogger.Infof("Telegram消息发送失败，%d秒后重试 (第%d次尝试)", waitTime/time.Second, attempt)
			time.Sleep(waitTime)
		}

		err := bot.SendMessage(text)
		if err == nil {
			if attempt > 0 {
				AppLogger.Infof("Telegram消息重试发送成功 (第%d次尝试)", attempt)
			}
			return nil
		}

		lastError = err
		AppLogger.Warnf("Telegram消息发送失败 (第%d次尝试): %v", attempt+1, err)

		// 如果是超时错误，继续重试
		if isTimeoutError(err) {
			continue
		}

		// 如果是其他类型的错误，立即返回
		break
	}

	return fmt.Errorf("经过%d次重试后仍然失败: %v", maxRetries+1, lastError)
}

// isTimeoutError 检查是否是超时错误
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	timeoutKeywords := []string{
		"timeout",
		"tls handshake timeout",
		"context deadline exceeded",
		"connection timeout",
		"dial timeout",
	}

	for _, keyword := range timeoutKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}

	return false
}

// TestConnection 测试Telegram机器人连接
func (bot *TelegramBot) TestConnection() error {
	if bot.Token == "" {
		return fmt.Errorf("telegram bot token不能为空")
	}

	// 如果提供了ChatID，测试发送消息
	if bot.ChatID == "" {
		return fmt.Errorf("telegram chat ID不能为空")
	}
	return bot.SendMessage("🤖 Telegram机器人连接测试成功！\n\n这是一条测试消息，表明您的机器人配置正确。")
}

// TestTelegramBot 测试Telegram机器人连接的便捷函数
func TestTelegramBot(token, chatID, httpProxy string) error {
	if httpProxy == "" {
		bot := NewTelegramBot(token, chatID)
		if bot == nil {
			return fmt.Errorf("创建Telegram机器人失败")
		}
		return bot.TestConnection()
	} else {
		bot, err := NewTelegramBotWithProxy(token, chatID, httpProxy)
		if err != nil {
			return fmt.Errorf("创建带代理的Telegram机器人失败: %v", err)
		}
		return bot.TestConnection()
	}
}
