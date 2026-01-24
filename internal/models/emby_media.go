package models

import (
	"Q115-STRM/internal/db"
	embyclientrestgo "Q115-STRM/internal/embyclient-rest-go"
	"Q115-STRM/internal/helpers"
	"context"
	"path/filepath"
	"strings"
	"time"
)

// EmbyMediaItem 同步下来的Emby媒体项
type EmbyMediaItem struct {
	BaseModel
	ItemId            string `json:"item_id" gorm:"uniqueIndex:idx_emby_item_id"`
	ServerId          string `json:"server_id" gorm:"index:idx_emby_server_id"`
	Name              string `json:"name"`
	Type              string `json:"type" gorm:"index:idx_emby_type"`
	ParentId          string `json:"parent_id" gorm:"index:idx_emby_parent_id"`
	SeriesId          string `json:"series_id" gorm:"index:idx_emby_series_id"`
	SeriesName        string `json:"series_name"`
	SeasonId          string `json:"season_id" gorm:"index:idx_emby_season_id"`
	SeasonName        string `json:"season_name"`
	LibraryId         string `json:"library_id" gorm:"index:idx_emby_library_id"`
	Path              string `json:"path"`
	PickCode          string `json:"pick_code" gorm:"index:idx_emby_pick_code"`
	MediaSourcePath   string `json:"media_source_path"`
	IndexNumber       int    `json:"index_number"`
	ParentIndexNumber int    `json:"parent_index_number"`
	ProductionYear    int    `json:"production_year"`
	PremiereDate      string `json:"premiere_date"`
	DateCreated       string `json:"date_created"`
	DateModified      string `json:"date_modified"`
	IsFolder          bool   `json:"is_folder"`
	EmbyData          string `json:"emby_data" gorm:"type:text"`
}

func (*EmbyMediaItem) TableName() string {
	return "emby_media_items"
}

// EmbyMediaSyncFile 关联表（多对多）
type EmbyMediaSyncFile struct {
	BaseModel
	EmbyItemId uint   `json:"emby_item_id" gorm:"index:idx_emby_media_item_id"`
	SyncFileId uint   `json:"sync_file_id" gorm:"index:idx_emby_sync_file_id"`
	PickCode   string `json:"pick_code" gorm:"index:idx_emby_sf_pick_code"`
}

func (*EmbyMediaSyncFile) TableName() string {
	return "emby_media_sync_files"
}

// EmbyLibrarySyncPath 媒体库与SyncPath关联（多对多允许重复库对应多个路径）
type EmbyLibrarySyncPath struct {
	BaseModel
	LibraryId   string `json:"library_id" gorm:"uniqueIndex:idx_lib_sync_path,priority:1"`
	SyncPathId  uint   `json:"sync_path_id" gorm:"uniqueIndex:idx_lib_sync_path,priority:2"`
	LibraryName string `json:"library_name"`
}

func (*EmbyLibrarySyncPath) TableName() string {
	return "emby_library_sync_paths"
}

// CreateOrUpdateEmbyMediaItem upsert by ItemId
func CreateOrUpdateEmbyMediaItem(item *EmbyMediaItem) error {
	existing := &EmbyMediaItem{}
	err := db.Db.Where("item_id = ?", item.ItemId).First(existing).Error
	if err != nil {
		return db.Db.Create(item).Error
	}
	item.ID = existing.ID
	return db.Db.Model(existing).Updates(item).Error
}

// GetEmbyMediaItemsPaginated 简单分页过滤
func GetEmbyMediaItemsPaginated(page, pageSize int, libraryId, itemType string) ([]*EmbyMediaItem, int64, error) {
	var items []*EmbyMediaItem
	var total int64
	q := db.Db.Model(&EmbyMediaItem{})
	if libraryId != "" {
		q = q.Where("library_id = ?", libraryId)
	}
	if itemType != "" {
		q = q.Where("type = ?", itemType)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if err := q.Order("id DESC").Offset(offset).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func GetEmbyMediaItemsCount() (int64, error) {
	var total int64
	return total, db.Db.Model(&EmbyMediaItem{}).Count(&total).Error
}

func CleanupOrphanedEmbyMediaItems(validItemIds []string) error {
	if len(validItemIds) == 0 {
		return db.Db.Where("1 = 1").Delete(&EmbyMediaItem{}).Error
	}

	// 当validItemIds很多时，分批处理以避免SQL语句过长
	// 每批处理1000个ID，这是一个安全的数量
	const batchSize = 1000

	if len(validItemIds) <= batchSize {
		// 数量不多，直接使用IN操作符
		return db.Db.Where("item_id NOT IN ?", validItemIds).Delete(&EmbyMediaItem{}).Error
	}

	// 数量很多，使用分批删除逻辑
	// 先获取所有的item_id，然后分批删除不在validItemIds中的记录
	validItemSet := make(map[string]bool)
	for _, itemId := range validItemIds {
		validItemSet[itemId] = true
	}

	// 获取数据库中所有的item_id，然后找出需要删除的
	var allItems []string
	if err := db.Db.Model(&EmbyMediaItem{}).Pluck("item_id", &allItems).Error; err != nil {
		return err
	}

	// 找出需要删除的item_id
	var itemsToDelete []string
	for _, itemId := range allItems {
		if !validItemSet[itemId] {
			itemsToDelete = append(itemsToDelete, itemId)
		}
	}

	if len(itemsToDelete) == 0 {
		return nil
	}

	// 分批删除
	for i := 0; i < len(itemsToDelete); i += batchSize {
		end := i + batchSize
		if end > len(itemsToDelete) {
			end = len(itemsToDelete)
		}

		batch := itemsToDelete[i:end]
		if err := db.Db.Where("item_id IN ?", batch).Delete(&EmbyMediaItem{}).Error; err != nil {
			return err
		}
	}

	return nil
}

// CreateEmbyMediaSyncFile 创建关联（存在则跳过）
func CreateEmbyMediaSyncFile(embyItemId string, syncFileId uint, pickCode string) error {
	var count int64
	embyItemIdInt := helpers.StringToInt(embyItemId)
	if err := db.Db.Model(&EmbyMediaSyncFile{}).
		Where("emby_item_id = ? AND sync_file_id = ?", uint(embyItemIdInt), syncFileId).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	relation := &EmbyMediaSyncFile{EmbyItemId: uint(embyItemIdInt), SyncFileId: syncFileId, PickCode: pickCode}
	return db.Db.Create(relation).Error
}

// CreateOrUpdateEmbyLibrarySyncPath 创建或更新关联（存在则跳过）
func CreateOrUpdateEmbyLibrarySyncPath(libraryId string, syncPathId uint, libraryName string) error {
	var count int64
	if err := db.Db.Model(&EmbyLibrarySyncPath{}).
		Where("library_id = ? AND sync_path_id = ?", libraryId, syncPathId).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	relation := &EmbyLibrarySyncPath{LibraryId: libraryId, SyncPathId: syncPathId, LibraryName: libraryName}
	return db.Db.Create(relation).Error
}

// DeleteEmbyLibrarySyncPathsBySyncPathID 按同步路径删除关联
func DeleteEmbyLibrarySyncPathsBySyncPathID(syncPathId uint) error {
	return db.Db.Where("sync_path_id = ?", syncPathId).Delete(&EmbyLibrarySyncPath{}).Error
}

// DeleteEmbyMediaSyncFilesBySyncFileID 按SyncFile删除关联
func DeleteEmbyMediaSyncFilesBySyncFileID(syncFileId uint) error {
	return db.Db.Where("sync_file_id = ?", syncFileId).Delete(&EmbyMediaSyncFile{}).Error
}

// DeleteEmbyMediaSyncFilesByPickCode 按PickCode删除关联
func DeleteEmbyMediaSyncFilesByPickCode(pickCode string) error {
	if pickCode == "" {
		return nil
	}
	return db.Db.Where("pick_code = ?", pickCode).Delete(&EmbyMediaSyncFile{}).Error
}

// UpdateLastSyncTime 更新最后同步时间戳
func UpdateLastSyncTime() error {
	config := &EmbyConfig{}
	if err := db.Db.First(config).Error; err != nil {
		return err
	}
	return db.Db.Model(config).Update("last_sync_time", time.Now().Unix()).Error
}

// 使用SyncPath查询关联的Emby LibraryId->LibraryName列表
func GetEmbyLibraryIdsBySyncPathId(syncPathId uint) map[string]string {
	var relations []EmbyLibrarySyncPath
	if err := db.Db.Where("sync_path_id = ?", syncPathId).Find(&relations).Error; err != nil {
		return nil
	}
	var libraryIds map[string]string = make(map[string]string)
	for _, rel := range relations {
		libraryIds[rel.LibraryId] = rel.LibraryName
	}
	return libraryIds
}

// 刷新Emby媒体库通过SyncPathId
func RefreshEmbyLibraryBySyncPathId(syncPathId uint) error {
	if GlobalEmbyConfig == nil || GlobalEmbyConfig.EmbyUrl == "" || GlobalEmbyConfig.EmbyApiKey == "" || GlobalEmbyConfig.EnableRefreshLibrary == 0 {
		helpers.AppLogger.Infof("Emby未配置或未启用刷新媒体库，跳过刷新")
		return nil
	}
	// 创建一个新的 Emby 客户端
	client := embyclientrestgo.NewClient(GlobalEmbyConfig.EmbyUrl, GlobalEmbyConfig.EmbyApiKey)
	libraryIds := GetEmbyLibraryIdsBySyncPathId(syncPathId)
	for libId, libName := range libraryIds {
		if err := client.RefreshLibrary(libId, libName); err != nil {
			return err
		}
		helpers.AppLogger.Infof("已触发Emby媒体库 %s (%s) 刷新", libName, libId)
	}
	return nil
}

// 联动删除网盘的电影
func DeleteNetdiskMovieByEmbyItemId(itemId string) error {
	itemIdUint := uint(helpers.StringToInt(itemId))
	embyItem := &EmbyMediaSyncFile{}
	if err := db.Db.Where("emby_item_id = ?", itemIdUint).First(embyItem).Error; err != nil {
		helpers.AppLogger.Errorf("Emby Item %s 没有关联的网盘文件", itemId)
		return err
	}
	syncFile := SyncFile{}
	if err := db.Db.Where("id = ?", embyItem.SyncFileId).Find(&syncFile).Error; err != nil {
		helpers.AppLogger.Errorf("查询Emby Item %s 关联的网盘文件 %d 失败: %v", itemId, embyItem.SyncFileId, err)
		return err
	}
	// 查找syncFile.Path下是否只有一个视频文件
	files := []SyncFile{}
	if err := db.Db.Where("path = ?", syncFile.Path).Find(&files).Error; err != nil {
		helpers.AppLogger.Errorf("查询网盘路径 %s 下的文件失败: %v", syncFile.Path, err)
		return err
	}
	helpers.AppLogger.Infof("准备删除Emby Item %s 关联的网盘文件 %s", itemId, syncFile.Path+syncFile.FileName)
	// 检查是否只有一个视频文件
	videoFileCount := 0
	// 顺便遍历出视频文件对应的元数据文件，以视频文件basename开头的元数据文件
	ext := filepath.Ext(syncFile.FileName)
	baseName := strings.TrimSuffix(syncFile.FileName, ext)
	metaFiles := []SyncFile{}
	for _, f := range files {
		if f.IsVideo {
			videoFileCount++
		}
		if f.IsMeta && strings.HasPrefix(f.FileName, baseName) {
			// 记录文件
			metaFiles = append(metaFiles, f)
		}
	}
	// 查询path的file_id
	path := Sync115Path{}
	if err := db.Db.Where("path = ?", syncFile.Path).First(&path).Error; err != nil {
		helpers.AppLogger.Errorf("查询网盘路径 %s 失败: %v", syncFile.Path, err)
		return err
	}
	// 调用115接口删除文件
	account, err := GetAccountById(syncFile.AccountId)
	if err != nil {
		helpers.AppLogger.Errorf("获取网盘账号 %d 失败: %v", syncFile.AccountId, err)
		return err
	}
	client := account.Get115Client(true)
	success := false
	delErr := error(nil)
	if videoFileCount == 1 {
		// 删除整个目录
		pathParent := filepath.Dir(syncFile.Path)
		pathParentId := ""
		pathParentStr := ""
		if pathParent == "" || pathParent == "." || pathParent == "/" {
			// 到了根目录，取SyncPath.SourcePathId
			syncPath := GetSyncPathById(syncFile.SyncPathId)
			if syncPath == nil {
				helpers.AppLogger.Errorf("查询SyncPath %d 失败", syncFile.SyncPathId)
				return nil
			}
			pathParentId = syncPath.BaseCid
			pathParentStr = syncPath.RemotePath
		} else {
			// 查询pathParent的file_id
			parentPath := Sync115Path{}
			if err := db.Db.Where("path = ?", pathParent).First(&parentPath).Error; err != nil {
				helpers.AppLogger.Errorf("查询电影文件夹的父路径 %s 失败: %v", pathParent, err)
				return err
			}
			pathParentId = parentPath.FileId
			pathParentStr = parentPath.Path
		}

		success, delErr = client.Del(context.Background(), []string{path.FileId}, pathParentId)
		if delErr != nil {
			helpers.AppLogger.Errorf("删除Emby Item %s 关联的网盘电影目录 %s=>%s失败: %v", itemId, pathParentId, pathParentStr, delErr)
			return delErr
		}
		helpers.AppLogger.Infof("删除Emby Item %s 关联的网盘电影目录 %s=>%s 成功", itemId, pathParentId, pathParentStr)
	} else {
		// 只删除视频文件+元数据
		// 整理要删除的文件ID列表
		fileIdsToDelete := []string{syncFile.FileId}
		for _, mf := range metaFiles {
			fileIdsToDelete = append(fileIdsToDelete, mf.FileId)
		}
		success, delErr = client.Del(context.Background(), fileIdsToDelete, path.FileId)
		if delErr != nil {
			helpers.AppLogger.Errorf("删除Emby Item %s 关联的网盘视频文件+元数据失败: %v", itemId, delErr)
			return delErr
		}
		helpers.AppLogger.Infof("删除Emby Item %s 关联的网盘视频文件+元数据成功: %v", itemId, success)
	}
	// 删除EmbyMediaSyncFile数据
	// 删除EmbyMediaItem数据
	if success {
		if err := db.Db.Where("emby_item_id = ?", itemIdUint).Delete(&EmbyMediaSyncFile{}).Error; err != nil {
			helpers.AppLogger.Errorf("删除Emby Item %s 关联的EmbyMediaSyncFile记录失败: %v", itemId, err)
			return err
		}
		if err := db.Db.Where("item_id = ?", itemId).Delete(&EmbyMediaItem{}).Error; err != nil {
			helpers.AppLogger.Errorf("删除Emby Item %s 关联的EmbyMediaItem记录失败: %v", itemId, err)
			return err
		}
	}

	return nil
}

// 联动删除网盘的集
func DeleteNetdiskEpisodeByEmbyItemId(itemId string) error {
	itemIdUint := uint(helpers.StringToInt(itemId))
	embyItem := &EmbyMediaSyncFile{}
	if err := db.Db.Where("emby_item_id = ?", itemIdUint).First(embyItem).Error; err != nil {
		helpers.AppLogger.Errorf("Emby Item %s 没有关联的网盘文件", itemId)
		return err
	}
	syncFile := SyncFile{}
	if err := db.Db.Where("id = ?", embyItem.SyncFileId).Find(&syncFile).Error; err != nil {
		helpers.AppLogger.Errorf("查询Emby Item %s 关联的网盘文件 %d 失败: %v", itemId, embyItem.SyncFileId, err)
		return err
	}
	files := []SyncFile{}
	if err := db.Db.Where("path = ?", syncFile.Path).Find(&files).Error; err != nil {
		helpers.AppLogger.Errorf("查询网盘路径 %s 下的文件失败: %v", syncFile.Path, err)
		return err
	}
	helpers.AppLogger.Infof("准备删除Emby Item %s 关联的网盘文件 %s", itemId, syncFile.Path+syncFile.FileName)
	// 顺便遍历出视频文件对应的元数据文件，以视频文件basename开头的元数据文件
	ext := filepath.Ext(syncFile.FileName)
	baseName := strings.TrimSuffix(syncFile.FileName, ext)
	fileIdsToDelete := []string{syncFile.FileId}
	for _, f := range files {
		if f.IsMeta && strings.HasPrefix(f.FileName, baseName) {
			// 记录文件
			fileIdsToDelete = append(fileIdsToDelete, f.FileId)
		}
	}
	// 查询path的file_id
	path := Sync115Path{}
	if err := db.Db.Where("path = ?", syncFile.Path).First(&path).Error; err != nil {
		helpers.AppLogger.Errorf("查询网盘路径 %s 失败: %v", syncFile.Path, err)
		return err
	}
	// 调用115接口删除文件
	account, err := GetAccountById(syncFile.AccountId)
	if err != nil {
		helpers.AppLogger.Errorf("获取网盘账号 %d 失败: %v", syncFile.AccountId, err)
		return err
	}
	client := account.Get115Client(true)
	success := false
	delErr := error(nil)
	success, delErr = client.Del(context.Background(), fileIdsToDelete, path.FileId)
	if delErr != nil {
		helpers.AppLogger.Errorf("删除Emby Item %s 关联的网盘集视频文件+元数据失败: %v", itemId, delErr)
		return delErr
	}
	helpers.AppLogger.Infof("删除Emby Item %s 关联的网盘集视频文件+元数据成功: %v", itemId, success)
	// 删除EmbyMediaSyncFile数据
	// 删除EmbyMediaItem数据
	if success {
		if err := db.Db.Where("emby_item_id = ?", itemIdUint).Delete(&EmbyMediaSyncFile{}).Error; err != nil {
			helpers.AppLogger.Errorf("删除Emby Item %s 关联的EmbyMediaSyncFile记录失败: %v", itemId, err)
			return err
		}
		if err := db.Db.Where("item_id = ?", itemId).Delete(&EmbyMediaItem{}).Error; err != nil {
			helpers.AppLogger.Errorf("删除Emby Item %s 关联的EmbyMediaItem记录失败: %v", itemId, err)
			return err
		}
	}
	return nil
}

// 联动删除网盘的季
func DeleteNetdiskSeasonByItemId(itemId string) error {
	// 根据itemId先查找到所有的EmbyMediaItem记录
	var embyItems []EmbyMediaItem
	if err := db.Db.Where("season_id = ?", itemId).Find(&embyItems).Error; err != nil {
		helpers.AppLogger.Errorf("查询SeasonId %s 关联的EmbyMediaItem记录失败: %v", itemId, err)
		return err
	}
	// 拿到所有关联的SyncFileId
	syncFileIds := []uint{}
	for _, embyItem := range embyItems {
		var embyMediaSyncFiles []EmbyMediaSyncFile
		if err := db.Db.Where("emby_item_id = ?", embyItem.ID).Find(&embyMediaSyncFiles).Error; err != nil {
			helpers.AppLogger.Errorf("查询Emby Item %s 关联的EmbyMediaSyncFile记录失败: %v", embyItem.ItemId, err)
			continue
		}
		for _, rel := range embyMediaSyncFiles {
			syncFileIds = append(syncFileIds, rel.SyncFileId)
		}
	}
	// 取第一个SyncFileId对应的SyncFile.Path作为季目录来处理
	if len(syncFileIds) == 0 {
		helpers.AppLogger.Infof("SeasonId %s 没有关联的网盘文件", itemId)
		return nil
	}
	syncFile := SyncFile{}
	if err := db.Db.Where("id = ?", syncFileIds[0]).Find(&syncFile).Error; err != nil {
		helpers.AppLogger.Errorf("查询SeasonId %s 关联的网盘文件 %d 失败: %v", itemId, syncFileIds[0], err)
		return err
	}
	seasonPath := syncFile.Path
	// 检查季目录是否为单独的目录
	seasonNumber := helpers.ExtractSeasonsFromSeasonPath(filepath.Base(seasonPath))
	if seasonNumber >= 0 {
		// 是单独的季目录，删除整个目录
		// 查询path的file_id
		path := Sync115Path{}
		if err := db.Db.Where("path = ?", seasonPath).First(&path).Error; err != nil {
			helpers.AppLogger.Errorf("查询网盘路径 %s 失败: %v", seasonPath, err)
			return err
		}
		// 查找seasonPath的父目录ID
		tvshowPath := filepath.Dir(seasonPath)
		tvshowPathId := ""
		if tvshowPath == "" || tvshowPath == "." || tvshowPath == "/" {
			// 到了根目录，取SyncPath.SourcePathId
			syncPath := GetSyncPathById(syncFile.SyncPathId)
			if syncPath == nil {
				helpers.AppLogger.Errorf("查询SyncPath %d 失败", syncFile.SyncPathId)
				return nil
			}
			tvshowPathId = syncPath.BaseCid
		} else {
			// 查询tvshowPath的file_id
			tvshowSync115Path := Sync115Path{}
			if err := db.Db.Where("path = ?", tvshowPath).First(&tvshowSync115Path).Error; err != nil {
				helpers.AppLogger.Errorf("查询季文件夹的父路径 %s 失败: %v", tvshowPath, err)
				return err
			}
			tvshowPathId = tvshowSync115Path.FileId
		}
		// 调用115接口删除文件
		account, err := GetAccountById(syncFile.AccountId)
		if err != nil {
			helpers.AppLogger.Errorf("获取网盘账号 %d 失败: %v", syncFile.AccountId, err)
			return err
		}
		client := account.Get115Client(true)
		_, delErr := client.Del(context.Background(), []string{path.FileId}, tvshowPathId)
		if delErr != nil {
			helpers.AppLogger.Errorf("删除Emby Item %s 关联的网盘电视剧 季目录 %s=>%s失败: %v", itemId, path.FileId, path.Path, delErr)
			return delErr
		}
		helpers.AppLogger.Infof("删除Emby Item %s 关联的网盘电视剧 季目录 %s=>%s 成功", itemId, path.FileId, path.Path)
	} else {
		// 不是单独的季目录，仅删除季下所有集对应的视频文件+元数据（nfo、封面)
		for _, embyItem := range embyItems {
			if err := DeleteNetdiskEpisodeByEmbyItemId(embyItem.ItemId); err != nil {
				continue
			}
		}
		helpers.AppLogger.Infof("删除Emby Item %s 关联的网盘电视剧 季下的所有集成功", itemId)
	}
	// 删除EmbyMediaItem数据
	if err := db.Db.Where("season_id = ?", itemId).Delete(&EmbyMediaItem{}).Error; err != nil {
		helpers.AppLogger.Errorf("删除SeasonId %s 关联的EmbyMediaItem记录失败: %v", itemId, err)
		return err
	}
	// 删除EmbyMediaSyncFile数据
	for _, syncFileId := range syncFileIds {
		if err := db.Db.Where("sync_file_id = ?", syncFileId).Delete(&EmbyMediaSyncFile{}).Error; err != nil {
			helpers.AppLogger.Errorf("删除SeasonId %s 关联的EmbyMediaSyncFile记录失败: %v", itemId, err)
			return err
		}
	}
	return nil
}

// 联动删除网盘的剧
func DeleteNetdiskTvshowByItemId(itemId string) error {
	// 根据itemId先查找到所有的EmbyMediaItem记录
	var embyItems []EmbyMediaItem
	if err := db.Db.Where("series_id = ?", itemId).Find(&embyItems).Error; err != nil {
		helpers.AppLogger.Errorf("查询SeriesId %s 关联的EmbyMediaItem记录失败: %v", itemId, err)
		return err
	}
	// 拿到所有关联的SyncFileId
	syncFileIds := []uint{}
	for _, embyItem := range embyItems {
		var embyMediaSyncFiles []EmbyMediaSyncFile
		if err := db.Db.Where("emby_item_id = ?", embyItem.ItemId).Find(&embyMediaSyncFiles).Error; err != nil {
			helpers.AppLogger.Errorf("查询Emby Item %s 关联的EmbyMediaSyncFile记录失败: %v", embyItem.ItemId, err)
			continue
		}
		for _, rel := range embyMediaSyncFiles {
			syncFileIds = append(syncFileIds, rel.SyncFileId)
		}
	}
	// 取第一个SyncFileId对应的SyncFile.Path作为剧目录来处理
	if len(syncFileIds) == 0 {
		helpers.AppLogger.Infof("SeriesId %s 没有关联的网盘文件", itemId)
		return nil
	}
	syncFile := SyncFile{}
	if err := db.Db.Where("id = ?", syncFileIds[0]).Find(&syncFile).Error; err != nil {
		helpers.AppLogger.Errorf("查询SeriesId %s 关联的网盘文件 %d 失败: %v", itemId, syncFileIds[0], err)
		return err
	}
	// 检查目录是否为季目录
	seasonNumber := helpers.ExtractSeasonsFromSeasonPath(filepath.Base(syncFile.Path))
	tvshowPath := ""
	tvshowPathId := ""
	if seasonNumber >= 0 {
		// 是季目录，取父目录作为剧目录来删除
		tvshowPath = filepath.Dir(syncFile.Path)
	} else {
		// 不是季目录，直接使用当前目录
		tvshowPath = syncFile.Path
	}
	// 查询tvshowPath的file_id
	if tvshowPath == "" || tvshowPath == "." || tvshowPath == "/" {
		// 到了根目录，不能删除
		helpers.AppLogger.Errorf("删除Emby Item %s 关联的网盘电视剧 目录失败: 已到达根目录 %s", itemId, tvshowPath)
		return nil
	} else {
		// 查询tvshowPath的file_id
		tvshowSync115Path := Sync115Path{}
		if err := db.Db.Where("path = ?", tvshowPath).First(&tvshowSync115Path).Error; err != nil {
			helpers.AppLogger.Errorf("查询剧文件夹的路径 %s 失败: %v", tvshowPath, err)
			return err
		}
		tvshowPathId = tvshowSync115Path.FileId
	}
	// 查找tvshowPath的父目录ID
	tvshowParentPath := filepath.Dir(tvshowPath)
	tvshowParentPathId := ""
	if tvshowParentPath == "" || tvshowParentPath == "." || tvshowParentPath == "/" {
		// 到了根目录，取SyncPath.SourcePathId
		syncPath := GetSyncPathById(syncFile.SyncPathId)
		if syncPath == nil {
			helpers.AppLogger.Errorf("查询SyncPath %d 失败", syncFile.SyncPathId)
			return nil
		}
		tvshowParentPathId = syncPath.BaseCid
	} else {
		// 查询tvshowParentPath的file_id
		tvshowParentSync115Path := Sync115Path{}
		if err := db.Db.Where("path = ?", tvshowParentPath).First(&tvshowParentSync115Path).Error; err != nil {
			helpers.AppLogger.Errorf("查询剧文件夹的父路径 %s 失败: %v", tvshowParentPath, err)
			return err
		}
		tvshowParentPathId = tvshowParentSync115Path.FileId
	}

	// 调用115接口删除文件
	account, err := GetAccountById(syncFile.AccountId)
	if err != nil {
		helpers.AppLogger.Errorf("获取网盘账号 %d 失败: %v", syncFile.AccountId, err)
		return err
	}
	client := account.Get115Client(true)
	_, delErr := client.Del(context.Background(), []string{tvshowPathId}, tvshowParentPathId)
	if delErr != nil {
		helpers.AppLogger.Errorf("删除Emby Item %s 关联的网盘电视剧 目录 %s=>%s失败: %v", itemId, tvshowPathId, tvshowPath, delErr)
		return delErr
	}
	helpers.AppLogger.Infof("删除Emby Item %s 关联的网盘电视剧 目录 %s=>%s 成功", itemId, tvshowPathId, tvshowPath)
	// 删除EmbyMediaItem数据
	if err := db.Db.Where("series_id = ?", itemId).Delete(&EmbyMediaItem{}).Error; err != nil {
		helpers.AppLogger.Errorf("删除SeriesId %s 关联的EmbyMediaItem记录失败: %v", itemId, err)
		return err
	}
	// 删除EmbyMediaSyncFile数据
	for _, syncFileId := range syncFileIds {
		if err := db.Db.Where("sync_file_id = ?", syncFileId).Delete(&EmbyMediaSyncFile{}).Error; err != nil {
			helpers.AppLogger.Errorf("删除SeriesId %s 关联的EmbyMediaSyncFile记录失败: %v", itemId, err)
			return err
		}
	}
	return nil
}
