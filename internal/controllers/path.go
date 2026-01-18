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

	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/disk"
)

type DirResp struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func GetPathList(c *gin.Context) {
	type dirListReq struct {
		ParentId   string            `json:"parent_id" form:"parent_id"`
		ParentPath string            `json:"parent_path" form:"parent_path"`
		SourceType models.SourceType `json:"source_type" form:"source_type"`
		AccountId  uint              `json:"account_id" form:"account_id"`
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
		pathes, err = Get115Path(req.ParentId, req.ParentPath, req.AccountId)
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

func Get115Path(parentId, parentPath string, accountId uint) ([]DirResp, error) {
	// 只返回文件夹列表
	folders := make([]DirResp, 0)
	account, err := models.GetAccountById(accountId)
	if err != nil {
		return nil, err
	}
	client := account.Get115Client(true)
	currentParentId := parentId
	if parentPath != "" && parentPath != "." && parentPath != "/" {
		// 先将parentPath转换为115的file_id
		helpers.AppLogger.Infof("开始使用当前路径 %s 查询115文件夹ID", parentPath)
		fileDetail, pathErr := client.GetFsDetailByCid(context.Background(), parentId)
		if pathErr != nil {
			return nil, pathErr
		}
		helpers.AppLogger.Infof("成功获取当前路径 %s 的ID: %s", parentPath, fileDetail.FileId)
		currentParentId = fileDetail.FileId
		if len(fileDetail.Paths) > 1 {
			// 用/分割parentPath，获取上级目录
			p := ""
			parentPathParts := strings.Split(parentPath, "/")
			if len(parentPathParts) > 1 {
				// 删除最后一个元素
				parentPathParts = parentPathParts[:len(parentPathParts)-1]
				p = strings.Join(parentPathParts, "/")
			}
			// 取fileDetail.Paths的最后一个
			parentPathDetail := fileDetail.Paths[len(fileDetail.Paths)-1]
			folders = append(folders, DirResp{
				Id:   parentPathDetail.FileId,
				Name: "..",
				Path: p,
			})
		} else {
			// 根目录
			folders = append(folders, DirResp{
				Id:   "0",
				Name: "..",
				Path: "",
			})
		}
	}
	helpers.AppLogger.Infof("开始获取115目录列表, 父目录ID: %s", currentParentId)
	ctx := context.Background()
	resp, err := client.GetFsList(ctx, currentParentId, true, false, true, 0, 100)
	if err != nil {
		helpers.AppLogger.Warnf("获取115目录列表失败: 父目录：%s, 错误:%v", parentPath, err)
		return nil, err
	}
	helpers.AppLogger.Infof("成功获取115目录列表, 父目录ID: %s, 目录数量: %d", currentParentId, len(resp.Data))
	for _, item := range resp.Data {
		path := item.FileName
		if parentPath != "" {
			path = parentPath + "/" + item.FileName
		}
		helpers.AppLogger.Infof("遍历 %s 的115目录列表, 路径: %s", parentPath, path)
		if item.FileCategory == v115open.TypeDir {
			folders = append(folders, DirResp{
				Id:   item.FileId,
				Name: item.FileName,
				Path: path,
			})
		}
	}
	return folders, nil
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
