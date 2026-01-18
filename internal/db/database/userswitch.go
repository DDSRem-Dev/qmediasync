package database

import (
	"Q115-STRM/internal/helpers"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

// UserSwitcher 用户切换器
type UserSwitcher struct {
	uid      string
	username string
}

// NewUserSwitcher 创建用户切换器
func NewUserSwitcher() *UserSwitcher {
	u := &UserSwitcher{}
	u.uid = helpers.Guid
	u.getUsernameByUID()
	return u
}

// 根据 UID 查找用户名
func (u *UserSwitcher) getUsernameByUID() error {
	// 方法1：使用 getent passwd 命令（优先在容器环境中使用）
	cmd := exec.Command("getent", "passwd", u.uid)
	output, err := cmd.Output()
	if err == nil {
		helpers.AppLogger.Infof("使用 getent passwd 查找 UID %s 对应的用户成功: %s\n", u.uid, string(output))
		parts := strings.Split(strings.TrimSpace(string(output)), ":")
		if len(parts) > 0 {
			u.username = parts[0]
			helpers.AppLogger.Infof("使用 getent passwd 解析 UID %s 对应的用户名: %s\n", u.uid, u.username)
			return nil
		}
	}

	// 方法2：使用 user.LookupId（备选方案）
	userInfo, err := user.LookupId(u.uid)
	if err != nil {
		helpers.AppLogger.Infof("使用 user.LookupId 查找 UID %s 对应的用户失败: %v\n", u.uid, err)
	} else {
		u.username = userInfo.Username
		helpers.AppLogger.Infof("使用 user.LookupId 查找 UID %s 对应的用户成功: %s\n", u.uid, u.username)
		return nil
	}

	return fmt.Errorf("找不到 UID %s 对应的用户", u.uid)
}

// RunCommandAsUser 使用 su 命令以指定用户身份运行命令
func (u *UserSwitcher) RunCommandAsUser(command string, args ...string) (string, error) {
	// 构建完整的命令
	fullArgs := []string{"-", u.username, "-c", command + " " + strings.Join(args, " ")}

	cmd := exec.Command("su", fullArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("以用户 %s 执行命令失败: %v, 输出: %s", u.username, err, string(output))
	}

	return string(output), nil
}

// RunCommandAsUserWithEnv 使用 su 命令并设置环境变量
func (u *UserSwitcher) RunCommandAsUserWithEnv(logFilePath string, env map[string]string, command string, args ...string) (*exec.Cmd, error) {
	// 构建环境变量字符串
	envVars := ""
	for key, value := range env {
		envVars += fmt.Sprintf("export %s=%s; ", key, value)
	}

	fullCommand := envVars + command + " " + strings.Join(args, " ")
	fullArgs := []string{"-", u.username, "-c", fullCommand}

	cmd := exec.Command("su", fullArgs...)
	// 设置输出日志
	logFile, err := os.Create(logFilePath)
	if err != nil {
		return nil, fmt.Errorf("创建日志文件失败: %v", err)
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	err = cmd.Start()
	if err != nil {
		output, _ := cmd.Output()
		return nil, fmt.Errorf("以用户 %s 执行命令失败: %v, 输出: %s", u.username, err, string(output))
	}

	return cmd, nil
}
