package helpers

import (
	"os"
	"path/filepath"
)

var Version = "0.0.1"
var ReleaseDate = "2025-11-07"

type configLog struct {
	File       string `yaml:"file"`
	Db         string `yaml:"db"`
	V115       string `yaml:"v115"`
	OpenList   string `yaml:"openList"`
	TMDB       string `yaml:"tmdb"`
	Web        string `yaml:"web"`
	SyncLogDir string `yaml:"syncLogDir"` // 同步任务的日志目录，每个同步任务会生成一个日志文件，文件名为任务ID
}
type configDb struct {
	File      string `yaml:"file"`
	CacheSize int    `yaml:"cacheSize"`
}
type configStrm struct {
	VideoExt     []string `yaml:"videoExt"`
	MinVideoSize int64    `yaml:"minVideoSize"` // 最小视频大小，单位字节
	MetaExt      []string `yaml:"metaExt"`
	Cron         string   `yaml:"cron"` // 定时任务表达式
}
type Config struct {
	Log              configLog  `yaml:"log"`
	Db               configDb   `yaml:"db"`
	JwtSecret        string     `yaml:"jwtSecret"`
	WebHost          string     `yaml:"webHost"`
	Strm             configStrm `yaml:"strm"`
	Open115AppId     string     `yaml:"open115AppId"`
	Open115TestAppId string     `yaml:"open115TestAppId"`
}

var GlobalConfig Config
var RootDir string
var IsRelease bool
var Guid string

func InitConfig() error {
	GlobalConfig = Config{
		Log: configLog{
			File:     "config/logs/app.log",
			Db:       "config/logs/db.log",
			V115:     "config/logs/115.log",
			OpenList: "config/logs/openList.log",
			Web:      "config/logs/web.log",
			TMDB:     "config/logs/tmdb.log",
		},
		Db: configDb{
			File:      "config/db.db",
			CacheSize: 134217728,
		},
		JwtSecret: "Q115-STRM-JWT-TOKEN-250706",
		WebHost:   ":12333",
		Strm: configStrm{
			VideoExt:     []string{".mp4", ".mkv", ".avi", ".mov", ".wmv", ".webm", ".flv", ".avi", ".ts", ".m4v"},
			MinVideoSize: 100, // 默认100MB
			MetaExt:      []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".nfo", ".srt", ".ass", ".svg", ".sup", ".lrc"},
			Cron:         "0 * * * *", // 默认每小时执行一次
		},
		Open115AppId:     "", // TODO 开源版本留空
		Open115TestAppId: "", // TODO 开源版本留空
	}
	// 判断config.yml是否存在，如果存在则删除
	configFile := filepath.Join(RootDir, "config/config.yml")
	if PathExists(configFile) {
		os.Remove(configFile)
	}
	return nil
}
