package controllers

import (
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/synccron"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// 基础配置
// TMDB API KEY或者Access Token设置
// 1. TMDB API KEY或者Access Token设置查询和保存
// 2. AI识别设置查询和保存
// 分类规则
// 1 分类列表（分电影和电视剧）
// 2 分类添加和编辑(分电影和电视剧)
// 分类数据
// 1. 电影分类数组
// 2. 电视剧分类数组
// 语言数组
// 1. 语言数组

type TmdbSettings struct {
	TmdbApiKey        string `json:"tmdb_api_key" form:"tmdb_api_key"`
	TmdbAccessToken   string `json:"tmdb_access_token" form:"tmdb_access_token"`
	TmdbUrl           string `json:"tmdb_url" form:"tmdb_url"`
	TmdbImageUrl      string `json:"tmdb_image_url" form:"tmdb_image_url"`
	TmdbLanguage      string `json:"tmdb_language" form:"tmdb_language"`
	TmdbImageLanguage string `json:"tmdb_image_language" form:"tmdb_image_language"`
	TmdbEnableProxy   bool   `json:"tmdb_enable_proxy" form:"tmdb_enable_proxy"`
}

type AiSettings struct {
	EnableAi    models.AiAction `json:"enable_ai" form:"enable_ai"`
	AiApiKey    string          `json:"ai_api_key" form:"ai_api_key"`
	AiBaseUrl   string          `json:"ai_base_url" form:"ai_base_url"`
	AiModelName string          `json:"ai_model_name" form:"ai_model_name"`
	AiPrompt    string          `json:"ai_prompt" form:"ai_prompt"`
	AiTimeout   int             `json:"ai_timeout" form:"ai_timeout"`
}

type MovieCategoryReq struct {
	ID            uint     `json:"id" form:"id"`
	Name          string   `json:"name" form:"name"`
	LanguageArray []string `json:"language_array" form:"language_array"`
	GenreIDArray  []int    `json:"genre_id_array" form:"genre_id_array"`
}

type TvshowCategoryReq struct {
	ID           uint     `json:"id" form:"id"`
	Name         string   `json:"name" form:"name"`
	CountryArray []string `json:"country_array" form:"country_array"`
	GenreIDArray []int    `json:"genre_id_array" form:"genre_id_array"`
}

func GetTmdbSettings(c *gin.Context) {
	tmdbSettings := TmdbSettings{
		TmdbApiKey:        models.GlobalScrapeSettings.TmdbApiKey,
		TmdbAccessToken:   models.GlobalScrapeSettings.TmdbAccessToken,
		TmdbUrl:           models.GlobalScrapeSettings.TmdbUrl,
		TmdbImageUrl:      models.GlobalScrapeSettings.TmdbImageUrl,
		TmdbLanguage:      models.GlobalScrapeSettings.TmdbLanguage,
		TmdbImageLanguage: models.GlobalScrapeSettings.TmdbImageLanguage,
		TmdbEnableProxy:   models.GlobalScrapeSettings.TmdbEnableProxy,
	}
	c.JSON(http.StatusOK, APIResponse[TmdbSettings]{Code: Success, Message: "", Data: tmdbSettings})
}

func SaveTmdbSettings(c *gin.Context) {
	reqData := TmdbSettings{}
	if err := c.ShouldBindJSON(&reqData); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	if err := models.GlobalScrapeSettings.SaveTmdb(reqData.TmdbApiKey, reqData.TmdbAccessToken, reqData.TmdbUrl, reqData.TmdbImageUrl, reqData.TmdbLanguage, reqData.TmdbImageLanguage, reqData.TmdbEnableProxy); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "保存TMDB设置成功", Data: nil})
}

func TestTmdbSettings(c *gin.Context) {
	reqData := TmdbSettings{}
	if err := c.ShouldBindJSON(&reqData); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	tmpScrapeSetting := &models.ScrapeSettings{
		TmdbApiKey:        reqData.TmdbApiKey,
		TmdbAccessToken:   reqData.TmdbAccessToken,
		TmdbUrl:           reqData.TmdbUrl,
		TmdbImageUrl:      reqData.TmdbImageUrl,
		TmdbLanguage:      reqData.TmdbLanguage,
		TmdbImageLanguage: reqData.TmdbImageLanguage,
		TmdbEnableProxy:   reqData.TmdbEnableProxy,
	}
	testResult := tmpScrapeSetting.TestTmdb()
	c.JSON(http.StatusOK, APIResponse[bool]{Code: Success, Message: "", Data: testResult})
}

func SaveAiSettings(c *gin.Context) {
	reqData := AiSettings{}
	if err := c.ShouldBindJSON(&reqData); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	if err := models.GlobalScrapeSettings.SaveAi(reqData.AiApiKey, reqData.AiBaseUrl, reqData.AiModelName, reqData.AiTimeout); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "保存AI识别设置成功", Data: nil})
}

func TestAiSettings(c *gin.Context) {
	reqData := AiSettings{}
	if err := c.ShouldBindJSON(&reqData); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	if reqData.AiApiKey == "" || reqData.AiBaseUrl == "" || reqData.AiModelName == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "必须配置API Key、接口地址模型名称", Data: nil})
		return
	}
	tmpScrapeSetting := &models.ScrapeSettings{
		AiApiKey:    reqData.AiApiKey,
		AiBaseUrl:   reqData.AiBaseUrl,
		AiModelName: reqData.AiModelName,
	}
	testResult := tmpScrapeSetting.TestAi()
	if testResult != nil {
		c.JSON(http.StatusOK, APIResponse[error]{Code: BadRequest, Message: testResult.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[error]{Code: Success, Message: "测试AI识别成功", Data: nil})
}

func GetAiSettings(c *gin.Context) {
	aiSettings := AiSettings{
		AiApiKey:    models.GlobalScrapeSettings.AiApiKey,
		AiBaseUrl:   models.GlobalScrapeSettings.AiBaseUrl,
		AiModelName: models.GlobalScrapeSettings.AiModelName,
		AiTimeout:   models.GlobalScrapeSettings.AiTimeout,
	}
	c.JSON(http.StatusOK, APIResponse[AiSettings]{Code: Success, Message: "", Data: aiSettings})
}

// 电影分类
func GetMovieGenre(c *gin.Context) {
	movieGenre := helpers.MovieGenres
	c.JSON(http.StatusOK, APIResponse[[]helpers.Genre]{Code: Success, Message: "", Data: movieGenre})
}

// 电视剧分类
func GetTvshowGenre(c *gin.Context) {
	tvshowGenre := helpers.TvshowGenres
	c.JSON(http.StatusOK, APIResponse[[]helpers.Genre]{Code: Success, Message: "", Data: tvshowGenre})
}

// 语言数组
func GetLanguage(c *gin.Context) {
	language := helpers.Languages
	c.JSON(http.StatusOK, APIResponse[[]helpers.Language]{Code: Success, Message: "", Data: language})
}

func GetCountries(c *gin.Context) {
	countries := helpers.Countries
	c.JSON(http.StatusOK, APIResponse[[]helpers.Country]{Code: Success, Message: "", Data: countries})
}

// 电影分类列表
func GetMovieCategories(c *gin.Context) {
	movieGenre := models.GetMovieCategory()
	c.JSON(http.StatusOK, APIResponse[[]*models.MovieCategory]{Code: Success, Message: "", Data: movieGenre})
}

// 电视剧分类列表
func GetTvshowCategories(c *gin.Context) {
	tvshowGenre := models.GetTvshowCategory()
	c.JSON(http.StatusOK, APIResponse[[]*models.TvShowCategory]{Code: Success, Message: "", Data: tvshowGenre})
}

// 保存电影分类
func SaveMovieCategory(c *gin.Context) {
	reqData := MovieCategoryReq{}
	if err := c.ShouldBindJSON(&reqData); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	movieCategory := &models.MovieCategory{
		BaseModel: models.BaseModel{
			ID: reqData.ID,
		},
		Name: reqData.Name,
	}
	if err := movieCategory.Save(reqData.Name, reqData.GenreIDArray, reqData.LanguageArray); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "保存电影分类成功", Data: nil})
}

// 保存电视剧分类
func SaveTvshowCategory(c *gin.Context) {
	reqData := TvshowCategoryReq{}
	if err := c.ShouldBindJSON(&reqData); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	tvshowCategory := &models.TvShowCategory{
		BaseModel: models.BaseModel{
			ID: reqData.ID,
		},
		Name: reqData.Name,
	}
	if err := tvshowCategory.Save(reqData.Name, reqData.GenreIDArray, reqData.CountryArray); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "保存电视剧分类成功", Data: nil})
}

// 删除电影分类
func DeleteMovieCategory(c *gin.Context) {
	id := helpers.StringToInt(c.Param("id"))

	if err := models.DeleteMovieCategory(uint(id)); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "删除电影分类成功", Data: nil})
}

// 删除电视剧分类
func DeleteTvshowCategory(c *gin.Context) {
	id := helpers.StringToInt(c.Param("id"))
	if err := models.DeleteTvshowCategory(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "删除电视剧分类成功", Data: nil})
}

// 查询同步目录列表
func GetScrapePathes(c *gin.Context) {
	scrapePathes := models.GetScrapePathes()
	for _, scrapePath := range scrapePathes {
		// 检查是否正在运行
		scrapePath.IsTaskRunning = synccron.CheckTaskIsRunning(scrapePath.ID, synccron.SyncTaskTypeScrape)
	}
	c.JSON(http.StatusOK, APIResponse[[]*models.ScrapePath]{Code: Success, Message: "", Data: scrapePathes})
}

// 查询同步目录详情
func GetScrapePath(c *gin.Context) {
	id := helpers.StringToInt(c.Param("id"))
	scrapePath := models.GetScrapePathByID(uint(id))
	if scrapePath == nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "刮削目录不存在", Data: nil})
		return
	}
	if scrapePath.EnableAi == "" {
		scrapePath.EnableAi = models.AiActionOff
	}
	c.JSON(http.StatusOK, APIResponse[*models.ScrapePath]{Code: Success, Message: "", Data: scrapePath})
}

// 添加或编辑同步目录
func SaveScrapePath(c *gin.Context) {
	reqData := models.ScrapePath{}
	if err := c.ShouldBindJSON(&reqData); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	// 如果是115，用ID查询实际的目录
	if reqData.SourceType == models.SourceType115 {
		// 用ID查询实际的目录
		account, err := models.GetAccountById(reqData.AccountId)
		if err != nil {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
			return
		}
		// 用ID查询实际的目录
		sourcePath := models.GetPathByPathFileId(account, reqData.SourcePathId)
		if sourcePath == "" {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "查询来源目录失败", Data: nil})
			return
		}
		reqData.SourcePath = sourcePath
		if reqData.DestPathId != "" {
			destPath := models.GetPathByPathFileId(account, reqData.DestPathId)
			if destPath == "" {
				c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "查询目标目录失败", Data: nil})
				return
			}
			reqData.DestPath = destPath
		}
	}
	helpers.AppLogger.Infof("最大线程数：%d", reqData.MaxThreads)
	if err := reqData.Save(); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "保存刮削目录成功", Data: nil})
}

// 删除同步目录
func DeleteScrapePath(c *gin.Context) {
	id := helpers.StringToInt(c.Param("id"))
	if err := models.DeleteScrapePath(uint(id)); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "删除刮削目录成功", Data: nil})
}

// 启动识别刮削
func ScanScrapePath(c *gin.Context) {
	type ScanScrapePathReq struct {
		ID uint `json:"id" form:"id"`
	}
	reqData := ScanScrapePathReq{}
	if err := c.ShouldBindJSON(&reqData); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	// 查询ScrapePath
	scrapePath := models.GetScrapePathByID(reqData.ID)
	if scrapePath == nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "刮削目录不存在", Data: nil})
		return
	}
	if err := synccron.AddSyncTask(scrapePath.ID, synccron.SyncTaskTypeScrape); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "添加刮削任务失败: " + err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "刮削任务已添加到队列", Data: nil})
}

// 刮削记录
func GetScrapeRecords(c *gin.Context) {
	page := helpers.StringToInt(c.Query("page"))
	if page == 0 {
		page = 1
	}
	pageSize := helpers.StringToInt(c.Query("pageSize"))
	if pageSize == 0 {
		pageSize = 100
	}
	mediaType := c.Query("type")
	status := c.Query("status")
	name := c.Query("name")
	scrapePathesCache := make(map[uint]*models.ScrapePath)
	total, scrapeRecords := models.GetScrapeMediaFiles(page, pageSize, mediaType, status, name)
	type scrapeMediaResp struct {
		ID              uint   `json:"id"`
		Type            string `json:"type"`
		Path            string `json:"path"`
		FileName        string `json:"file_name"`
		MediaName       string `json:"media_name"`
		OriginalName    string `json:"original_name"`
		Year            int    `json:"year"`
		SeasonNumber    int    `json:"season_number"`
		EpisodeNumber   int    `json:"episode_number"`
		Genre           string `json:"genre"`
		Country         string `json:"country"`
		Language        string `json:"language"`
		Status          string `json:"status"`
		TmdbID          int64  `json:"tmdb_id"`
		NewPath         string `json:"new_path"` // 二级分类 + 文件夹
		NewFile         string `json:"new_file"` // 新文件名
		EpisodeName     string `json:"episode_name"`
		Resolution      string `json:"resolution"`       // 分辨率
		ResolutionLevel string `json:"resolution_level"` // 分辨率等级
		IsHDR           bool   `json:"is_hdr"`           // 是否HDR
		AudioCount      int    `json:"audio_count"`      // 音频轨道数量
		SubtitleCount   int    `json:"subtitle_count"`   // 字幕轨道数量
		CreatedAt       int64  `json:"created_at"`       // 创建时间
		UpdatedAt       int64  `json:"updated_at"`       // 更新时间
		ScrapedAt       int64  `json:"scraped_at"`       // 刮削时间
		ScannedAt       int64  `json:"scanned_at"`       // 扫描时间
		RenamedAt       int64  `json:"renamed_at"`       // 整理时间
		FailedReason    string `json:"failed_reason"`    // 失败原因
		CategoryName    string `json:"category_name"`    // 分类名称
		NewDestPath     string `json:"new_dest_path"`    // 新路径
		NewDestName     string `json:"new_dest_name"`    // 新文件名
		PathIsScraping  bool   `json:"path_is_scraping"` // 是否正在刮削
		PathIsRenaming  bool   `json:"path_is_renaming"` // 是否正在整理
		SourceFullPath  string `json:"source_full_path"` // 原始路径
		DestFullPath    string `json:"dest_full_path"`   // 目标路径
		SourceType      string `json:"source_type"`      // 原始类型
		RenameType      string `json:"rename_type"`      // 重命名类型
		ScrapeType      string `json:"scrape_type"`      // 刮削类型
	}
	type scrapeListResp struct {
		Total int64              `json:"total"`
		List  []*scrapeMediaResp `json:"list"`
	}
	resp := scrapeListResp{
		Total: total,
		List:  make([]*scrapeMediaResp, len(scrapeRecords)),
	}
	for i, scrapeMedia := range scrapeRecords {
		var scrapePath *models.ScrapePath
		var ok bool
		if scrapePath, ok = scrapePathesCache[scrapeMedia.ScrapePathId]; !ok {
			scrapePath = models.GetScrapePathByID(scrapeMedia.ScrapePathId)
			if scrapePath == nil {
				continue
			}
			scrapePathesCache[scrapeMedia.ScrapePathId] = scrapePath
		}
		if scrapePath == nil {
			continue
		}
		sourcePath := scrapeMedia.GetRemoteFullMoviePath()
		destPath := scrapeMedia.GetDestFullMoviePath()
		if scrapeMedia.MediaType == models.MediaTypeTvShow {
			sourcePath = scrapeMedia.GetRemoteFullSeasonPath()
			destPath = scrapeMedia.GetDestFullSeasonPath()
		}
		sourcePath = filepath.Join(sourcePath, scrapeMedia.VideoFilename)
		destPath = filepath.Join(destPath, scrapeMedia.NewVideoBaseName+scrapeMedia.VideoExt)

		resp.List[i] = &scrapeMediaResp{
			Type:            string(scrapeMedia.MediaType),
			ID:              scrapeMedia.ID,
			Path:            scrapeMedia.Path,
			FileName:        scrapeMedia.VideoFilename,
			MediaName:       scrapeMedia.Name,
			Year:            scrapeMedia.Year,
			SeasonNumber:    scrapeMedia.SeasonNumber,
			EpisodeNumber:   scrapeMedia.EpisodeNumber,
			Status:          string(scrapeMedia.Status),
			TmdbID:          scrapeMedia.TmdbId,
			NewPath:         filepath.Join(scrapeMedia.CategoryName, scrapeMedia.NewPathName),
			NewFile:         scrapeMedia.NewVideoBaseName + scrapeMedia.VideoExt,
			Resolution:      scrapeMedia.Resolution,
			ResolutionLevel: scrapeMedia.ResolutionLevel,
			IsHDR:           scrapeMedia.IsHDR,
			AudioCount:      len(scrapeMedia.AudioCodec),
			SubtitleCount:   len(scrapeMedia.SubtitleCodec),
			CreatedAt:       scrapeMedia.CreatedAt,
			UpdatedAt:       scrapeMedia.UpdatedAt,
			ScrapedAt:       scrapeMedia.ScrapeTime,
			ScannedAt:       scrapeMedia.ScanTime,
			RenamedAt:       scrapeMedia.RenameTime,
			CategoryName:    scrapeMedia.CategoryName,
			NewDestPath:     scrapeMedia.NewPathName,
			NewDestName:     scrapeMedia.NewVideoBaseName + scrapeMedia.VideoExt,
			FailedReason:    scrapeMedia.FailedReason,
			SourceFullPath:  sourcePath,
			DestFullPath:    destPath,
			SourceType:      string(scrapePath.SourceType),
			RenameType:      string(scrapePath.RenameType),
			ScrapeType:      string(scrapePath.ScrapeType),
		}
		if scrapeMedia.MediaType == models.MediaTypeTvShow {
			if scrapeMedia.MediaEpisode != nil {
				resp.List[i].EpisodeName = scrapeMedia.MediaEpisode.EpisodeName
			}
			resp.List[i].NewPath = filepath.Join(resp.List[i].NewPath, scrapeMedia.NewSeasonPathName)
		}
		if scrapePath.IsScraping && slices.Contains([]models.ScrapeMediaStatus{models.ScrapeMediaStatusScanned, models.ScrapeMediaStatusScraping}, scrapeMedia.Status) {
			resp.List[i].PathIsScraping = scrapePath.IsScraping
		}
		// if scrapeMedia.Media != nil {
		// 	if len(scrapeMedia.Media.Genres) > 0 {
		// 		for _, genre := range scrapeMedia.Media.Genres {
		// 			resp.List[i].Genre += genre.Name + ", "
		// 		}
		// 	}
		// 	if len(scrapeMedia.Media.OriginCountry) > 0 {
		// 		countries := ""
		// 		for _, country := range scrapeMedia.Media.OriginCountry {
		// 			if countries == "" {
		// 				countries = helpers.GetCountryName(country)
		// 			} else {
		// 				countries += ", " + helpers.GetCountryName(country)
		// 			}

		// 		}
		// 		resp.List[i].Country = countries
		// 	}
		// 	if scrapeMedia.Media.OriginalLanguage != "" {
		// 		resp.List[i].Language = helpers.GetLanguageName(scrapeMedia.Media.OriginalLanguage)
		// 	}
		// 	if scrapeMedia.Media.OriginalName != "" {
		// 		resp.List[i].OriginalName = scrapeMedia.Media.OriginalName
		// 	}
		// }
	}
	c.JSON(http.StatusOK, APIResponse[scrapeListResp]{Code: Success, Message: "", Data: resp})
}

func GetTmpImageUrl(path string, mediaType models.MediaType) string {
	return fmt.Sprintf("/scrape/tmp-image?path=%s&type=%s", url.QueryEscape(path), url.QueryEscape(string(mediaType)))
}

// 读取临时静态文件返回给客户端
// 主要是图片
func ScrapeTmpImage(c *gin.Context) {
	imagePath := c.Query("path")
	mediaType := models.MediaType(c.Query("type"))
	if imagePath == "" {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "路径不能为空", Data: nil})
		return
	}
	imageRootPath := filepath.Join(helpers.RootDir, "config/tmp/刮削临时文件")
	if mediaType == models.MediaTypeTvShow {
		imageRootPath = filepath.Join(imageRootPath, "电视剧")
	} else {
		imageRootPath = filepath.Join(imageRootPath, "电影或其他")
	}
	imagePath = filepath.Join(imageRootPath, imagePath)
	// 读取文件
	file, err := os.Open(imagePath)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "读取文件失败: " + err.Error(), Data: nil})
		return
	}
	defer file.Close()
	// 读取文件内容
	fileContent, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "读取文件内容失败: " + err.Error(), Data: nil})
		return
	}
	// 返回文件内容
	c.Data(http.StatusOK, "image/jpeg", fileContent)
}

func ExportScrapeRecords(c *gin.Context) {
	ids := c.Query("ids")
	if ids == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请选择要导出的记录", Data: nil})
		return
	}
	// ids用,分隔
	idList := strings.Split(ids, ",")
	// 转成uint数组
	idUintList := make([]uint, 0)
	for _, id := range idList {
		idUint, _ := strconv.ParseUint(id, 10, 32)
		idUintList = append(idUintList, uint(idUint))
	}
	scrapeRecords := models.GetScrapeMediaFilesByIds(idUintList)
	if len(scrapeRecords) == 0 {
		c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "没有找到要导出的记录", Data: nil})
		return
	}
	// 生成txt文件内容
	type exportMediaResult struct {
		Path          string `json:"path"`
		FileName      string `json:"filename"`
		TvPath        string `json:"tv_path"`
		Name          string `json:"name"`
		Year          int    `json:"year"`
		SeasonNumber  int    `json:"season_number"`
		EpisodeNumber int    `json:"episode_number"`
	}
	exportMediaList := make([]exportMediaResult, 0)
	for _, scrapeMedia := range scrapeRecords {
		exportMediaList = append(exportMediaList, exportMediaResult{
			Path:          scrapeMedia.Path,
			FileName:      scrapeMedia.VideoFilename,
			TvPath:        scrapeMedia.TvshowPath,
			Name:          scrapeMedia.Name,
			Year:          scrapeMedia.Year,
			SeasonNumber:  scrapeMedia.SeasonNumber,
			EpisodeNumber: scrapeMedia.EpisodeNumber,
		})
	}
	// json 格式化exportMediaList
	exportMediaListJson, err := json.MarshalIndent(exportMediaList, "", "  ")
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "格式化导出记录失败: " + err.Error(), Data: nil})
		return
	}
	c.Header("Content-Disposition", "attachment; filename=刮削记录.json")
	// 返回json文件内容,触发浏览器下载
	c.Data(http.StatusOK, "application/json", exportMediaListJson)
}

// 重新刮削某个失败的记录，使用用户输入的名称和年份
func ReScrape(c *gin.Context) {
	type reScrapeReq struct {
		ID      uint   `json:"id"`
		Name    string `json:"name"`
		Year    int    `json:"year"`
		TmdbId  int64  `json:"tmdb_id"`
		Season  int    `json:"season"`
		Episode int    `json:"episode"`
	}
	var req reScrapeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}
	// 使用ID查询ScrapeMediaFile
	scrapeMedia := models.GetScrapeMediaFileById(req.ID)
	if scrapeMedia == nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "没有找到要重新刮削的记录", Data: nil})
		return
	}
	scrapePath := models.GetScrapePathByID(scrapeMedia.ScrapePathId)
	if scrapePath == nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "没有找到要重新刮削的记录的刮削目录", Data: nil})
		return
	}
	oldStatus := scrapeMedia.Status
	err := scrapeMedia.ReScrape(req.Name, req.Year, req.TmdbId, req.Season, req.Episode)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "重新刮削失败: " + err.Error(), Data: nil})
		return
	}
	if oldStatus == models.ScrapeMediaStatusRenamed {
		synccron.StartScrapeRollbackCron() // 触发一次
		c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "操作成功，已将文件移动并重命名到源目录，下次扫描时会使用新的名称和年份进行刮削", Data: nil})
	} else {
		data := make(map[string]any)
		data["name"] = scrapeMedia.Name
		data["year"] = scrapeMedia.Year
		data["tmdb_id"] = scrapeMedia.TmdbId
		c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "操作成功，下次扫描时会使用新的名称和年份进行刮削", Data: data})
	}
}

// 清除所有刮削失败的记录
func ClearFailedScrapeRecords(c *gin.Context) {
	err := models.ClearFailedScrapeRecords([]uint{})
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "操作成功，所有失败的刮削记录已清除", Data: nil})
}

// 将整理中的记录标记为已整理
// 检查源文件是否存在
// 检查目标文件是否存在
func FinishScrapeMediaFile(c *gin.Context) {
	type finishScrapeReq struct {
		ID uint `json:"id"`
	}
	var req finishScrapeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}
	// 使用ID查询ScrapeMediaFile
	scrapeMedia := models.GetScrapeMediaFileById(req.ID)
	if scrapeMedia == nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "没有找到要整理的记录", Data: nil})
		return
	}
	scrapeMedia.FinishFromRenaming()
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "操作成功，记录已标记为已整理", Data: nil})
}

// 删除选定的刮削记录
func DeleteScrapeMediaFile(c *gin.Context) {
	ids := c.Query("ids")
	if ids == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请选择要导出的记录", Data: nil})
		return
	}
	// ids用,分隔
	idList := strings.Split(ids, ",")
	// 转成uint数组
	idUintList := make([]uint, 0)
	for _, id := range idList {
		idUint, _ := strconv.ParseUint(id, 10, 32)
		idUintList = append(idUintList, uint(idUint))
	}
	// 删除记录
	err := models.ClearFailedScrapeRecords(idUintList)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "删除记录失败: " + err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "操作成功，记录已删除", Data: nil})
}

// 将所选刮削记录标记为待整理
func RenameFailedScrapeMediaFile(c *gin.Context) {
	ids := c.Query("ids")
	if ids == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请选择要导出的记录", Data: nil})
		return
	}
	// ids用,分隔
	idList := strings.Split(ids, ",")
	// 转成uint数组
	idUintList := make([]uint, 0)
	for _, id := range idList {
		idUint, _ := strconv.ParseUint(id, 10, 32)
		idUintList = append(idUintList, uint(idUint))
	}
	// 将这些ID对应的记录标记为待整理
	err := models.RenameFailedScrapeRecords(idUintList)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "标记记录失败: " + err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "操作成功，所选记录已标记为待整理", Data: nil})
}

func ToggleScrapePathCron(c *gin.Context) {
	type toggleScrapePathCronReq struct {
		ID uint `json:"id"`
	}
	var req toggleScrapePathCronReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}
	// 使用ID查询ScrapePath
	scrapePath := models.GetScrapePathByID(req.ID)
	if scrapePath == nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "没有找到要操作的记录", Data: nil})
		return
	}
	// 切换定时刮削
	err := scrapePath.ToggleCron()
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "切换定时刮削失败: " + err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "操作成功，定时刮削已切换", Data: nil})
}

func StopScrape(c *gin.Context) {
	type ScanScrapePathReq struct {
		ID uint `json:"id" form:"id"`
	}
	var req ScanScrapePathReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}
	synccron.StopSyncTask(req.ID, synccron.SyncTaskTypeScrape)
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "操作成功，刮削任务已停止", Data: nil})
}
