package controllers

import (
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type LoginRequest struct {
	Username string `json:"username" form:"username"`
	Password string `json:"password" form:"password"`
}

var LoginedUser *models.User = nil

// 登录
func LoginAction(c *gin.Context) {
	user := &models.User{}
	var req LoginRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: fmt.Sprintf("参数错误：%v", err), Data: nil})
		return
	}
	username := req.Username
	password := req.Password
	if username == "" || password == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "用户名或密码不能为空", Data: nil})
		return
	}

	// 查询用户是否存在
	user, userErr := models.CheckLogin(username, password)
	if userErr != nil || user.ID == 0 {
		c.JSON(http.StatusUnauthorized, APIResponse[any]{Code: BadRequest, Message: fmt.Sprintf("用户不存在或者密码错误: %v", userErr), Data: nil})
		return
	}
	claims := &LoginUser{
		ID:       user.ID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(96 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(helpers.GlobalConfig.JwtSecret))
	if err != nil {
		helpers.AppLogger.Errorf("LoginAction: %v", err)
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "登录失败，请重试", Data: nil})
		return
	}
	LoginedUser = user
	res := make(map[string]interface{})
	u := make(map[string]string)
	u["id"] = fmt.Sprintf("%d", user.ID)
	u["username"] = user.Username
	u["email"] = ""
	u["role"] = "admin"
	res["user"] = u
	res["token"] = tokenString
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "登录成功", Data: res})
}

func ChangePassword(c *gin.Context) {
	var req struct {
		Username    string `json:"username" form:"username"`
		NewPassword string `json:"new_password" form:"new_password"`
	}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: fmt.Sprintf("参数错误：%v", err), Data: nil})
		return
	}
	if req.Username == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "用户名不能为空", Data: nil})
		return
	}
	isChange := false
	isChange2 := false
	var err error
	if req.Username != LoginedUser.Username {
		isChange = true
	}
	isChange2, err = LoginedUser.ChangeUsernameAndPassword(req.Username, req.NewPassword)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "修改失败: " + err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "修改成功", Data: isChange || isChange2})
}

func GetUserInfo(c *gin.Context) {
	// 返回当前用户ID和用户名
	respData := make(map[string]string)
	respData["id"] = fmt.Sprintf("%d", LoginedUser.ID)
	respData["username"] = LoginedUser.Username
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取用户信息成功", Data: respData})
}
