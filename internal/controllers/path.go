package controllers

import (
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/v115open"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/disk"
)

type DirResp struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// GetPathList 获取目录列表
// @Summary 获取目录列表
// @Description 按同步源类型获取本地、OpenList或115的目录列表
// @Tags 路径管理
// @Accept json
// @Produce json
// @Param parent_id query string false "父目录ID，仅115使用"
// @Param parent_path query string false "父目录路径，本地或OpenList使用"
// @Param source_type query integer true "同步源类型，0-本地 1-115 2-OpenList"
// @Param account_id query integer false "账号ID，115或OpenList必填"
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /path/list [get]
// @Security JwtAuth
// @Security ApiKeyAuth
func GetPathList(c *gin.Context) {
	type dirListReq struct {
		ParentId   string            `json:"parent_id" form:"parent_id"`
		ParentPath string            `json:"parent_path" form:"parent_path"`
		SourceType models.SourceType `json:"source_type" form:"source_type"`
		AccountId  uint              `json:"account_id" form:"account_id"`
		Page       int               `json:"page" form:"page"`
		PageSize   int               `json:"page_size" form:"page_size"`
	}
	var req dirListReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	var pathes []DirResp
	var err error
	switch req.SourceType {
	case models.SourceTypeLocal:
		pathes, err = GetLocalPath(req.ParentId)
	case models.SourceTypeOpenList:
		pathes, err = GetOpenListPath(req.ParentId, req.AccountId)
	case models.SourceType115:
		pathes, err = Get115PathList(req.ParentId, req.AccountId)
	default:
		// 报错
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "未知的同步源类型", Data: nil})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取目录列表失败: " + err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取目录列表成功", Data: pathes})
}

// 获取本地目录列表
// parentPath string 父目录路径
// windows，如果parentPath为空，则获取盘符列表
// 非windows，如果parentPath为空，则获取根目录/的子目录列表
func GetLocalPath(parentPath string) ([]DirResp, error) {
	pathes := make([]DirResp, 0)
	// windows
	if runtime.GOOS == "windows" {
		if parentPath == "" {
			// 获取盘符列表
			partitions, err := disk.Partitions(false)
			if err != nil {
				helpers.AppLogger.Errorf("获取盘符失败：%v", err)
				return nil, err
			}
			for _, partition := range partitions {
				// helpers.AppLogger.Debugf("盘符: %s", partition.Mountpoint)
				pathes = append(pathes, DirResp{
					Id:   partition.Mountpoint + "\\",
					Name: partition.Mountpoint,
					Path: partition.Mountpoint + "\\",
				})
			}
			return pathes, nil
		}
	} else {
		if parentPath == "" {
			// 获取根目录/的子目录列表
			parentPath = "/"
		}
	}
	if parentPath != "/" && runtime.GOOS != "windows" {
		// 加入返回上级目录
		p := filepath.Dir(parentPath)
		pathes = append(pathes, DirResp{
			Id:   p,
			Name: "..",
			Path: p,
		})
	}
	if runtime.GOOS == "windows" && parentPath != "" {
		var p string
		if len(parentPath) == 3 && string(parentPath[1]) == ":" && string(parentPath[2]) == "\\" {
			p = ""
		} else {
			p = filepath.Dir(parentPath)
		}
		pathes = append(pathes, DirResp{
			Id:   p,
			Name: "..",
			Path: p,
		})
	}
	// helpers.AppLogger.Infof("parentPath: %s", parentPath)
	// 获取子目录列表
	entries, err := os.ReadDir(parentPath)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			// 跳过隐藏目录
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			fullPath := filepath.Join(parentPath, entry.Name())
			pathes = append(pathes, DirResp{
				Id:   fullPath,
				Name: entry.Name(),
				Path: fullPath,
			})
		}
	}
	return pathes, nil
}

func GetOpenListPath(parentPath string, accountId uint) ([]DirResp, error) {
	account, err := models.GetAccountById(accountId)
	if err != nil {
		return nil, err
	}
	// 去掉parentPath末尾的/
	parentPath = strings.TrimSuffix(parentPath, "/")
	parentPath = strings.TrimSuffix(parentPath, "\\")

	helpers.AppLogger.Infof("开始获取OpenList目录列表, 父目录路径: %s", parentPath)
	client := account.GetOpenListClient()
	resp, err := client.FileList(context.Background(), parentPath, 1, 100)
	if err != nil {
		return nil, err
	}
	// 只返回文件夹列表
	folders := make([]DirResp, 0)
	if parentPath != "" && parentPath != "/" && parentPath != "\\" {
		// 加入返回上级目录
		p := filepath.Dir(parentPath)
		folders = append(folders, DirResp{
			Id:   p,
			Name: "..",
			Path: p,
		})
	}
	for _, item := range resp.Content {
		if item.IsDir {
			folders = append(folders, DirResp{
				Id:   parentPath + "/" + item.Name,
				Name: item.Name,
				Path: parentPath + "/" + item.Name,
			})
		}
	}
	return folders, nil
}

func Get115PathList(parentId string, accountId uint) ([]DirResp, error) {
	// 获取115目录列表
	account, err := models.GetAccountById(accountId)
	if err != nil {
		return nil, err
	}
	client := account.Get115Client()
	helpers.AppLogger.Infof("开始获取115目录列表, 父目录ID: %s", parentId)
	ctx := context.Background()
	resp, err := client.GetFsList(ctx, parentId, true, true, true, 0, 200)
	if err != nil {
		helpers.AppLogger.Warnf("获取115目录列表失败: 父目录：%s, 错误:%v", parentId, err)
		return nil, err
	}
	helpers.AppLogger.Infof("成功获取115目录列表, 父目录ID: %s, 文件数量: %d", parentId, len(resp.Data))
	folders := make([]DirResp, 0)
	if parentId != "0" {
		// 需要加入回到上一级
		if len(resp.Path) == 1 {
			// 如果只有一级直接定义上一级为根目录
			folders = append(folders, DirResp{
				Id:   "0",
				Name: "..",
				Path: "",
			})
		} else {
			parentPathStr := ""
			parentPathId := "0"
			for idx, p := range resp.Path {
				if p.FileId == "0" {
					// 跳过根目录
					continue
				}
				if idx == len(resp.Path)-1 {
					// 最后一个是当前目录，跳过
					break
				}
				parentPathStr = filepath.Join(parentPathStr, p.Name)
				parentPathId = string(p.FileId)
			}
			folders = append(folders, DirResp{
				Id:   parentPathId,
				Name: "..",
				Path: parentPathStr,
			})
		}
	}
	// 构建Path
	for _, item := range resp.Data {
		parentPath := resp.PathStr
		if parentPath == "" {
			parentPath = ""
		}
		helpers.AppLogger.Infof("遍历 %s 的115目录列表, 路径: %s", parentPath, item.FileName)
		if item.FileCategory == v115open.TypeDir {
			folders = append(folders, DirResp{
				Id:   item.FileId,
				Name: item.FileName,
				Path: filepath.ToSlash(filepath.Join(parentPath, item.FileName)),
			})
		}
	}
	return folders, nil
}

type FileItem struct {
	Id          string `json:"id"`
	IsDirectory bool   `json:"is_directory"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ModifiedAt  int64  `json:"modified_time"`
}

// 返回目录和文件列表
func GetNetFileList(c *gin.Context) {
	type dirListReq struct {
		ParentId  string `json:"parent_id" form:"path"`
		AccountId uint   `json:"account_id" form:"account_id"`
		Page      int    `json:"page" form:"page"`
		PageSize  int    `json:"page_size" form:"page_size"`
	}
	var req dirListReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 100
	}
	account, err := models.GetAccountById(req.AccountId)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取账号信息失败: " + err.Error(), Data: nil})
		return
	}
	var list []*FileItem
	switch account.SourceType {
	case models.SourceTypeOpenList:
		list, err = GetOpenListFiles(req.ParentId, account, req.Page, req.PageSize)
	case models.SourceType115:
		list, err = Get115Files(req.ParentId, account, req.Page, req.PageSize)
	default:
		// 报错
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "未知的网盘类型", Data: nil})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取目录列表失败: " + err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[[]*FileItem]{Code: Success, Message: "", Data: list})
}

func GetOpenListFiles(parentPath string, account *models.Account, page, pageSize int) ([]*FileItem, error) {
	// 去掉parentPath末尾的/
	parentPath = strings.TrimSuffix(parentPath, "/")
	parentPath = strings.TrimSuffix(parentPath, "\\")
	helpers.AppLogger.Infof("开始获取OpenList目录列表, 父目录路径: %s", parentPath)
	client := account.GetOpenListClient()
	resp, err := client.FileList(context.Background(), parentPath, page, pageSize)
	if err != nil {
		return nil, err
	}
	// 只返回文件夹列表
	items := make([]*FileItem, 0)
	for _, item := range resp.Content {
		t, err := time.Parse(time.RFC3339, item.Modified)
		var mtime int64
		if err != nil {
			mtime = 0 // 错误时使用默认值
		} else {
			mtime = t.Unix() // 转换为Unix时间戳（秒）
		}
		items = append(items, &FileItem{
			Id:          parentPath + "/" + item.Name,
			IsDirectory: item.IsDir,
			Name:        item.Name,
			Size:        item.Size,
			ModifiedAt:  mtime,
		})
	}
	return items, nil
}

func Get115Files(parentId string, account *models.Account, page, pageSize int) ([]*FileItem, error) {
	client := account.Get115Client()
	ctx := context.Background()
	resp, err := client.GetFsList(ctx, parentId, true, true, true, 0, 200)
	if err != nil {
		helpers.AppLogger.Warnf("获取115目录列表失败: 父目录：%s, 错误:%v", parentId, err)
		return nil, err
	}
	helpers.AppLogger.Infof("成功获取115文件列表, 父目录ID: %s, 文件数量: %d", parentId, len(resp.Data))
	items := make([]*FileItem, 0)
	// 构建Path
	for _, item := range resp.Data {
		items = append(items, &FileItem{
			Id:          item.FileId,
			IsDirectory: item.FileCategory == v115open.TypeDir,
			Name:        item.FileName,
			Size:        item.FileSize,
			ModifiedAt:  item.Ptime,
		})
	}
	return items, nil

}
