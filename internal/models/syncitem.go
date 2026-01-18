package models

import (
	"Q115-STRM/internal/db"
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/v115open"
	"bytes"
	"encoding/gob"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type SyncTreeItemMetaAction int

const (
	SyncTreeItemMetaActionKeep   SyncTreeItemMetaAction = iota // 保留元数据
	SyncTreeItemMetaActionUpload                               // 上传元数据
	SyncTreeItemMetaActionDelete                               // 删除元数据
)

type StrmData struct {
	UserId   string `json:"userid"`    // 用户ID
	PickCode string `json:"pick_code"` // 文件ID
	Sign     string `json:"sign"`      // 文件签名
	Path     string `json:"path"`      // 115的路径
	BaseUrl  string `json:"base_url"`  // 115的base_url
}

// 所有的路径, 不包含根目录
type Sync115Path struct {
	BaseModel
	SyncPathId uint   `json:"sync_path_id" gorm:"index:file_id, unique"` // 同步路径ID
	FileId     string `json:"file_id" gorm:"index:file_id, unique"`      // 文件夹ID
	Name       string `json:"name"`
	Path       string `json:"path"`                               // 相对路径，包含Name
	LocalPath  string `json:"local_path" gorm:"index:local_path"` // 本地绝对目录
}

type SyncFile struct {
	BaseModel
	SourceType    SourceType        `json:"source_type"`
	AccountId     uint              `json:"account_id"`
	SyncPathId    uint              `json:"sync_path_id" gorm:"index:sync_path_id"`
	FileId        string            `json:"file_id" gorm:"index:file_id"`
	ParentId      string            `json:"parent_id"`
	FileName      string            `json:"file_name"`
	FileSize      int64             `json:"file_size"`
	FileType      v115open.FileType `json:"file_type"`
	PickCode      string            `json:"pick_code" gorm:"index:pick_code"`
	Sha1          string            `json:"sha1"`
	MTime         int64             `json:"mtime"`                                        // 最后修改时间
	LocalFilePath string            `json:"local_file_path" gorm:"index:local_file_path"` // 本地文件路径，包含文件名
	Path          string            `json:"path"`                                         // 绝对路径，不包含FileName
	SyncPath      *SyncPath         `json:"-" gorm:"-"`                                   // 关联的同步路径
	Sync          *Sync             `json:"-" gorm:"-"`                                   // 关联的同步项
	Account       *Account          `json:"-" gorm:"-"`                                   // 关联的账号
	IsVideo       bool              `json:"is_video"`
	IsMeta        bool              `json:"is_meta"`
	OpenlistSign  string            `json:"openlist_sign"` // openlist会返回sign，用于生成op的文件链接
	Uploaded      bool              `json:"uploaded"`      // 是否上传完成，未上传完成的记录不触发删除
}

func (sf *SyncFile) GetAccount() *Account {
	if sf.Account == nil {
		sf.Account, _ = GetAccountById(sf.SyncPath.AccountId)
	}
	return sf.Account
}

func (sf *SyncFile) Save() error {
	return db.Db.Save(sf).Error
}

// 生成完整的本地路径
func (sf *SyncFile) MakeFullLocalPath() string {
	sf.LocalFilePath = sf.SyncPath.MakeFullLocalPath(sf.Path, sf.FileName)
	return sf.LocalFilePath
}

func (sf *SyncFile) GetRemoteFilePath() string {
	path := sf.SyncPath.RemotePath + "/" + sf.Path + "/" + sf.FileName
	// 将\转成/
	path = strings.ReplaceAll(path, "\\", "/")
	return path
}

func (sf *SyncFile) GetRemoteFilePathUrlEncode() string {
	// 中文保留，只对特殊字符编码
	path := sf.GetRemoteFilePath()
	path = strings.ReplaceAll(path, "/", "%2F")
	path = strings.ReplaceAll(path, "?", "%3F")
	path = strings.ReplaceAll(path, "&", "%26")
	path = strings.ReplaceAll(path, "=", "%3D")
	path = strings.ReplaceAll(path, "+", "%2B")
	path = strings.ReplaceAll(path, "#", "%23")
	path = strings.ReplaceAll(path, "@", "%40")
	path = strings.ReplaceAll(path, "!", "%21")
	path = strings.ReplaceAll(path, "$", "%24")
	path = strings.ReplaceAll(path, " ", "%20")
	return path
}

func (sf *SyncFile) Make115StrmUrl() string {
	// 生成URL
	u, _ := url.Parse(SettingsGlobal.StrmBaseUrl)
	u.Path = "/115/newurl"
	params := url.Values{}
	params.Add("pickcode", sf.PickCode)
	params.Add("userid", sf.GetAccount().UserId.String())
	u.RawQuery = params.Encode()
	urlStr := u.String()
	if sf.SyncPath.GetAddPath() == 1 {
		urlStr += fmt.Sprintf("&path=%s", sf.GetRemoteFilePathUrlEncode())
	}
	return urlStr
}

func (sf *SyncFile) MakeLocalFileStrmUrl() string {
	return sf.FileId
}

func (sf *SyncFile) MakeOpenListStrmUrl() string {
	account := sf.GetAccount()
	if account == nil {
		sf.Sync.Logger.Errorf("获取账号失败: %v", account)
		return ""
	}
	// 去掉BaseUrl末尾的/
	baseUrl := strings.TrimSuffix(account.BaseUrl, "/")
	// 将sf.FileId中的\替换为/
	fileId := strings.ReplaceAll(sf.FileId, "\\", "/")
	// 去掉sf.FileId首尾的/
	fileId = strings.Trim(fileId, "/")
	// 对sf.FileId做Urlencode
	// fileId = url.QueryEscape(fileId)
	url := fmt.Sprintf("%s/d/%s", baseUrl, fileId)
	if sf.OpenlistSign != "" {
		url += "?sign=" + sf.OpenlistSign
	}
	return url
}

// 生成strm文件
// st只能是来源路径，所以需要生成strm文件的路径
func (sf *SyncFile) ProcessStrmFile() bool {
	rs := sf.CompareStrm()
	if rs == 2 {
		// 需要删除strm文件
		os.Remove(sf.LocalFilePath)
		sf.Sync.Logger.Infof("文件 %s 已删除本地strm文件，可能文件大小已不满足最低要求。", sf.LocalFilePath)
		return false
	}
	if rs == 1 {
		sf.Sync.Logger.Infof("[strm已存在] 文件 %s 无需更新strm文件", sf.LocalFilePath)
		return true
	}
	strmFullPath := sf.LocalFilePath
	strmContent := ""
	switch sf.SourceType {
	case SourceType115:
		strmContent = sf.Make115StrmUrl()
	case SourceTypeLocal:
		strmContent = sf.MakeLocalFileStrmUrl()
	case SourceTypeOpenList:
		strmContent = sf.MakeOpenListStrmUrl()
	}
	// // 创建目录
	// strmFilePath := filepath.Dir(strmFullPath)
	// if !helpers.PathExists(strmFilePath) {
	// 	err := os.MkdirAll(strmFilePath, 0777)
	// 	if err != nil {
	// 		sf.Sync.Logger.Errorf("创建目录 %s 失败: %v", strmFilePath, err)
	// 		return false
	// 	}
	// }
	// os.Chmod(strmFilePath, 0777)
	// 写入文件并设置所有者
	err := helpers.WriteFileWithPerm(strmFullPath, []byte(strmContent), 0777)
	if err != nil {
		sf.Sync.Logger.Errorf("写入strm文件并设置所有者失败: %v", err)
	}
	// 修改文件时间
	if sf.MTime > 0 {
		err := os.Chtimes(strmFullPath, time.Unix(sf.MTime, 0), time.Unix(sf.MTime, 0))
		if err != nil {
			sf.Sync.Logger.Errorf("修改strm文件时间失败: %v", err)
		}
	}
	sf.Sync.Logger.Infof("[生成strm] %s => %s", strmFullPath, strmContent)
	sf.Sync.NewStrm++
	return true
}

// 1-无需操作，2-删除，0-更新
func (st *SyncFile) CompareStrm() int {
	if !helpers.PathExists(st.LocalFilePath) {
		return 0
	}
	if st.SourceType == SourceTypeLocal {
		return 1
	}
	account := st.GetAccount()
	if account == nil {
		st.Sync.Logger.Errorf("获取账号失败: %v", account)
		return 0
	}
	// 读取strm文件内容
	strmData := st.LoadDataFromStrm()
	if strmData == nil {
		return 0
	}
	if st.SourceType == SourceTypeOpenList {
		// 比较主机名称是否相同
		if strmData.BaseUrl != account.BaseUrl {
			st.Sync.Logger.Warnf("文件 %s 的STRM内容的主机名称与本地不一致, 本地: %s, 远程: %s", filepath.Join(st.Path, st.FileName), SettingsGlobal.StrmBaseUrl, strmData.BaseUrl)
			return 0
		}
		if strmData.Sign != st.OpenlistSign {
			st.Sync.Logger.Warnf("文件 %s 的STRM内容的签名参数与本地不一致, 本地: %s, 远程: %s", filepath.Join(st.Path, st.FileName), st.OpenlistSign, strmData.Sign)
			return 0
		}
	}
	if st.SourceType == SourceType115 {
		// 比较路径是否相同
		if st.SyncPath.GetAddPath() == 1 {
			if strmData.Path != st.GetRemoteFilePath() {
				st.Sync.Logger.Warnf("文件 %s 的STRM内容的路径与本地不一致, 本地: %s, 远程: %s", filepath.Join(st.Path, st.FileName), st.GetRemoteFilePath(), strmData.Path)
				return 0
			}
		} else {
			if strmData.Path != "" {
				st.Sync.Logger.Warnf("文件 %s 的STRM内容的含有完整路径 %s，但是设置中关闭了添加路径，所以重新生成strm以去掉路径s", filepath.Join(st.Path, st.FileName), strmData.Path)
				return 0
			}
		}
		// 比较主机名称是否相同
		if strmData.BaseUrl != SettingsGlobal.StrmBaseUrl {
			st.Sync.Logger.Warnf("文件 %s 的STRM内容的主机名称与本地不一致, 本地: %s, 远程: %s", filepath.Join(st.Path, st.FileName), SettingsGlobal.StrmBaseUrl, strmData.BaseUrl)
			return 0
		}
		// 如果没有PickCode，则更新以补全
		if strmData.PickCode == "" {
			st.Sync.Logger.Warnf("文件 %s 的STRM内容缺少PickCode: %s, 补全", filepath.Join(st.Path, st.FileName), strmData.PickCode)
			return 0
		}
		if strmData.UserId != account.UserId.String() {
			st.Sync.Logger.Warnf("文件 %s 的STRM内容的用户ID与本地不一致, 本地: %s, 远程: %s", filepath.Join(st.Path, st.FileName), account.UserId.String(), strmData.UserId)
			return 0
		}
	}
	// 比较文件修改时间
	// if st.MTime > 0 {
	// 	// 读取文件修改时间
	// 	fileInfo, err := os.Stat(st.LocalFilePath)
	// 	if err != nil {
	// 		st.Sync.Logger.Errorf("读取文件 %s 信息失败: %v", st.LocalFilePath, err)
	// 		return 0
	// 	}
	// 	// 比较文件修改时间
	// 	if fileInfo.ModTime().Unix() != st.MTime {
	// 		st.Sync.Logger.Warnf("文件 %s 的修改时间与网盘不一致, 本地: %d, 远程: %d", filepath.Join(st.Path, st.FileName), fileInfo.ModTime().Unix(), st.MTime)
	// 		return 0
	// 	}
	// }
	return 1
}

// 解析strm文件内url的参数并返回
func (st *SyncFile) LoadDataFromStrm() *StrmData {
	strmPath := st.LocalFilePath
	if !helpers.PathExists(strmPath) {
		// st.Sync.Logger.Errorf("strm文件不存在: %s", strmPath)
		return nil
	}
	data, err := os.ReadFile(strmPath)
	if err != nil {
		st.Sync.Logger.Errorf("读取strm文件失败: %v", err)
		return nil
	}
	var strmData StrmData
	strmUrl, urlErr := url.Parse(string(data))
	if urlErr != nil {
		st.Sync.Logger.Errorf("解析strm文件失败: %v", urlErr)
		return nil
	}
	queryParams := strmUrl.Query()
	if pickCode := queryParams.Get("pickcode"); pickCode != "" {
		strmData.PickCode = pickCode
	}
	if userId := queryParams.Get("userid"); userId != "" {
		strmData.UserId = userId
	}
	strmData.Sign = ""
	if sign := queryParams.Get("sign"); sign != "" {
		strmData.Sign = sign
	}
	strmData.Path = ""
	if path := queryParams.Get("path"); path != "" {
		strmData.Path = path
	}
	strmData.BaseUrl = fmt.Sprintf("%s://%s", strmUrl.Scheme, strmUrl.Host)
	return &strmData
}

// isSave 是否保存到数据库，如果不保存则返回Sync115Path对象，但是没有id
func AddSync115Path(syncPathId uint, fileId, name, path, localPath string, isSave bool) (*Sync115Path, error) {
	sync115Path := &Sync115Path{
		SyncPathId: syncPathId,
		FileId:     fileId,
		Name:       name,
		Path:       path,
		LocalPath:  localPath,
	}
	if !isSave {
		return sync115Path, nil
	}
	err := db.Db.Create(sync115Path).Error
	if err != nil {
		return nil, err
	}
	return sync115Path, nil
}

func GetSyncFileByFileId(syncPathId uint, fileId string) *SyncFile {
	if fileId == "" {
		return nil
	}
	var db115File *SyncFile
	err := db.Db.Model(&SyncFile{}).Where("sync_path_id = ? AND file_id = ?", syncPathId, fileId).First(&db115File).Error
	if err != nil {
		return nil
	}
	return db115File
}

func CheckSyncPathIdExists(fileId string) bool {
	if fileId == "" {
		return false
	}
	var total int64
	err := db.Db.Model(&Sync115Path{}).Where("file_id = ?", fileId).Count(&total).Error
	if err != nil {
		return false
	}
	return total > 0
}

func GetSyncFileById(id uint) *SyncFile {
	if id == 0 {
		return nil
	}
	var db115File *SyncFile
	err := db.Db.Model(&SyncFile{}).Where("id = ?", id).First(&db115File).Error
	if err != nil {
		return nil
	}
	return db115File
}

func GetSyncFilesByIds(fileIds []uint) []*SyncFile {
	if len(fileIds) == 0 {
		return nil
	}
	var db115Files []*SyncFile
	err := db.Db.Model(&SyncFile{}).Where("id IN ?", fileIds).Find(&db115Files).Error
	if err != nil {
		return nil
	}
	return db115Files
}

func GetFileByLocalFilePath(localFilePath string) *SyncFile {
	if localFilePath == "" {
		return nil
	}
	var db115File *SyncFile
	err := db.Db.Model(&SyncFile{}).Where("local_file_path = ?", localFilePath).First(&db115File).Error
	if err != nil {
		return nil
	}
	return db115File
}

// 删除以excludePathStr开头的所有路径
func DeleteExcludePathFile(excludePathStr string, syncPathId uint, deletePath bool) {
	if excludePathStr == "" {
		return
	}
	ext := filepath.Ext(excludePathStr)
	if runtime.GOOS == "windows" {
		excludePathStr = strings.ReplaceAll(excludePathStr, "\\", "\\\\")
	}
	if ext == "" {
		if deletePath {
			// Delete by ID
			result := db.Db.Exec("DELETE FROM sync115_paths WHERE sync_path_id = ? AND local_path LIKE ?", syncPathId, excludePathStr+"%")
			helpers.AppLogger.Infof("删除路径 %s 下的所有子路径记录，影响行数 %d", excludePathStr, result.RowsAffected)
		}
		// 再删除sync_files下的所有记录
		result := db.Db.Exec("DELETE FROM sync_files WHERE sync_path_id = ? AND local_file_path LIKE ?", syncPathId, excludePathStr+"%")
		helpers.AppLogger.Infof("删除路径 %s 下的所有文件记录，影响行数 %d", excludePathStr, result.RowsAffected)
	} else {
		// 再删除sync_files下的记录
		result := db.Db.Exec("DELETE FROM sync_files WHERE sync_path_id = ? AND local_file_path = ?", syncPathId, excludePathStr)
		helpers.AppLogger.Infof("删除文件 %s 记录，影响行数 %d", excludePathStr, result.RowsAffected)
	}

}

func GetPathByPathId(syncPathId uint, fileId string) string {
	var db115Path *Sync115Path
	err := db.Db.Model(&Sync115Path{}).Where("sync_path_id = ? AND file_id = ?", syncPathId, fileId).First(&db115Path).Error
	if err != nil {
		return ""
	}
	return db115Path.Path
}

func GetPathByLocalPath(localPath string) *Sync115Path {
	if localPath == "" {
		return nil
	}
	// 增加缓存
	cacheKey := fmt.Sprintf("sync115_path:%s", localPath)
	cachedPath := db.Cache.Get(cacheKey)
	if cachedPath != nil {
		// 反序列化
		buf := bytes.NewBuffer(cachedPath)
		decoder := gob.NewDecoder(buf)
		var sync115Path Sync115Path
		err := decoder.Decode(&sync115Path)
		if err != nil {
			helpers.AppLogger.Errorf("反序列化路径详情时出错，路径:%s，错误：%v", localPath, err)
			return nil
		}
		return &sync115Path
	}
	var db115Path *Sync115Path
	err := db.Db.Model(&Sync115Path{}).Where("local_path = ?", localPath).First(&db115Path).Error
	if err != nil {
		helpers.AppLogger.Errorf("查询路径详情时出错，路径:%s，错误：%v", localPath, err)
		return nil
	}
	// 缓存结果
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(db115Path); err != nil {
		return nil
	}
	db.Cache.Set(cacheKey, buf.Bytes(), 300)
	return db115Path
}

func GetFileByPickCode(pickCode string) *SyncFile {
	if pickCode == "" {
		return nil
	}
	var db115File *SyncFile
	err := db.Db.Model(&SyncFile{}).Where("pick_code = ?", pickCode).First(&db115File).Error
	if err != nil {
		return nil
	}
	return db115File
}

func DeleteFileByPickCode(pickCode string) error {
	if pickCode == "" {
		return nil
	}
	// Delete by ID
	result := db.Db.Exec("DELETE FROM sync_files WHERE pick_code = ?", pickCode)
	err := result.Error
	if err != nil {
		return err
	}
	return nil
}

func DeletePathByFileId(fileId string) error {
	if fileId == "" {
		return nil
	}
	// Delete by ID
	result := db.Db.Exec("DELETE FROM sync115_paths WHERE file_id = ?", fileId)
	err := result.Error
	if err != nil {
		return err
	}
	return nil
}

func DeleteAllFileBySyncPathId(syncPathId uint) error {
	if syncPathId == 0 {
		return nil
	}
	// Delete by ID
	result := db.Db.Exec("DELETE FROM sync_files WHERE sync_path_id = ?", syncPathId)
	err := result.Error
	if err != nil {
		return err
	}
	// 清空所有路径表
	result = db.Db.Exec("DELETE FROM sync115_paths WHERE sync_path_id = ?", syncPathId)
	err = result.Error
	if err != nil {
		return err
	}
	// panic("DeleteAllFileBySyncPathId not implemented")
	return nil
}

func AddSyncFile(syncPath *SyncPath, fileId, parentId, fileName, path, localFilePpath string, fileSize int64, pickCode, sha1 string, isMeta, isVideo bool, openlistSign string, uploaded bool, isSave bool, mtime int64) (*SyncFile, error) {
	// 查询syncPath
	if syncPath == nil {
		return nil, fmt.Errorf("syncPath not found")
	}
	if isSave {
		// 检查fileId是否存在
		db115File := GetSyncFileByFileId(syncPath.ID, fileId)
		if db115File != nil {
			return nil, fmt.Errorf("fileId %s already exists", fileId)
		}
	}
	newFile := &SyncFile{
		AccountId:     syncPath.AccountId,
		SourceType:    syncPath.SourceType,
		SyncPathId:    syncPath.ID,
		FileId:        fileId,
		ParentId:      parentId,
		FileName:      fileName,
		FileSize:      fileSize,
		FileType:      v115open.TypeFile,
		PickCode:      pickCode,
		Sha1:          sha1,
		MTime:         mtime,
		LocalFilePath: localFilePpath,
		Path:          path,
		IsVideo:       isVideo,
		IsMeta:        isMeta,
		OpenlistSign:  openlistSign,
		Uploaded:      true,
	}
	if !isMeta {
		newFile.Uploaded = uploaded
	}
	if !isSave {
		return newFile, nil
	}
	err := db.Db.Create(newFile).Error
	if err != nil {
		return nil, err
	}
	return newFile, nil
}

func GetSyncFiles(offset, limit int, syncPathId uint) ([]*SyncFile, error) {
	var files []*SyncFile
	err := db.Db.Model(&SyncFile{}).Where("sync_path_id = ?", syncPathId).Order("id ASC").Offset(offset).Limit(limit).Find(&files).Error
	if err != nil {
		return nil, err
	}
	return files, nil
}

func GetSyncFileTotal(syncPathId uint) (int, error) {
	var count int64
	err := db.Db.Model(&SyncFile{}).Where("sync_path_id = ?", syncPathId).Count(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func Get115Pathes(offset, limit int, syncPathId uint) ([]*Sync115Path, error) {
	var pathes []*Sync115Path
	err := db.Db.Model(&Sync115Path{}).Where("sync_path_id = ?", syncPathId).Offset(offset).Limit(limit).Find(&pathes).Error
	if err != nil {
		return nil, err
	}
	return pathes, nil
}

func GetSyncFilesByPathIdAndLocalPathEmpty(parentId string) ([]*SyncFile, error) {
	var files []*SyncFile
	err := db.Db.Model(&SyncFile{}).Where("parent_id = ? AND local_file_path = ''", parentId).Find(&files).Error
	if err != nil {
		return nil, err
	}
	return files, nil
}
