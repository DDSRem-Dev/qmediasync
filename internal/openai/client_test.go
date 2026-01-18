package openai

import (
	"fmt"
	"slices"
	"testing"
)

type MediaInfo struct {
	Name    []string `json:"name"`
	Year    int      `json:"year"`
	Season  int      `json:"season"`
	Episode int      `json:"episode"`
}

type TestCase struct {
	filename          string
	expectedMediaInfo *MediaInfo
}

type TestCases []TestCase

func TestExtractMediaInfo_Movie(t *testing.T) {
	client := NewClient(DEFAULT_API_KEY, DEFAULT_API_BASE_URL, DEFAULT_MODEL_NAME, DEFAULT_TIMEOUT)

	testCases := TestCases{
		{
			filename: "【悠哈璃羽字幕社】[死神千年血战相克谭_Bleach - Thousand-Year Blood War - Soukoku Tan][11][1080p][CHT] [432.3 MB]",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"死神千年血战相克谭"},
				Year:    0,
				Season:  0,
				Episode: 0,
			},
		},

		{
			filename: "【诸神字幕组】[鬼灭之刃_Kimetsu no Yaiba][24][1080p][MP4].mp4",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"鬼灭之刃"},
				Year:    0,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "长安的荔枝[国语配音+中文字幕].The.Lychee.Road.2025.1080p.WEB-DL.H264.AAC-PandaQT",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"长安的荔枝"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},

		{
			filename: "星际穿越[IMAX满屏版][国英多音轨+简繁英字幕].Interstellar.2014.IMAX.2160p.BluRay.x265.10bit.TrueHD5.1-CTRLHD",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"星际穿越", "Interstellar"},
				Year:    2014,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Interstellar.2014.UHD.BluRay.2160p.DTS-HD.MA.5.1.HEVC.REMUX-FraMeSToR",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"interstellar"},
				Year:    2014,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "星际穿越[国英多音轨+中文字幕+特效字幕].Interstellar.2014.2160p.UHD.BluRay.REMUX.HEVC.HDR.DTS-HDMA5.1-DreamHD",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"星际穿越", "Interstellar"},
				Year:    2014,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "星际穿越 Interstellar (2014)",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"星际穿越", "Interstellar"},
				Year:    2014,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "[DBD-Raws][死神/Bleach][OVA][01-02合集][HEVC-10bit][简繁外挂][FLAC][MKV]",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"死神"},
				Year:    0,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "[RU]Caught.Stealing.2025.1080p.MA.WEB-DL.ExKinoRay.mkv",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"caught stealing"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Caught.Stealing.2025.MULTi.VF2.2160p.HDR.DV.WEB-DL.H265.mkv",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"caught stealing"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "机动战士高达：跨时之战.1080p.HD中字.mp4",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"机动战士高达：跨时之战"},
				Year:    0,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "机动战士高达：跨时之战[国语配音+中文字幕].2025.2160p.WEB-DL.H265.HDR.DDP5.1-QuickIO",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"机动战士高达：跨时之战"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "UIndex - Hans.Zimmer.and.Friends.Diamond.in.the.Desert.2025.1080p.WEB.h264-WEBLE",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"hans zimmer and friends diamond in the desert"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Hans Zimmer Friends Diamond In The Desert (2025) [720p] [WEBRip] [YTS.MX]",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"hans zimmer friends diamond in the desert"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "孤独的美食家.剧场版[中文字幕].The.Solitary.Gourmet.2024.1080p.HamiVideo.WEB-DL.AAC2.0.H.264-DreamHD",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"孤独的美食家"},
				Year:    2024,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "戏台.2160p高码版.60fps.HD国语中字无水印.mkv",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"戏台"},
				Year:    0,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "戏台[120帧率版本][国语配音+中文字幕].The.Stage.2025.2160p.WEB-DL.H265.HDR.120fps.DTS5.1-DreamHD",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"戏台", "the stage"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "戏台[杜比视界版本][高码版][国语配音+中文字幕].The.Stage.2025.2160p.HQ.WEB-DL.H265.DV.DTS5.1-DreamHD",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"戏台", "the stage"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The.Stage.2025.WEB.1080p.AC3.Audio.x265-112114119",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the stage"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "[戏台].The.Stage.2025.2160p.WEB-DL.H265.AAC-CMCTV",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"戏台", "the stage"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "《戏台 (2025)》｜4KHDR片源｜黄渤新片｜中字畅享版",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"戏台"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "【戏台 (2025)】【4K+1080P】【国语中字。】【类型：剧情】 【▶️4K精品影视/_▶️】 ✅✅",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"戏台"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "[60帧率版本][国语配音+中文字幕].The.Stage.2025.2160p.WEB-DL.H265.HDR.60fps.AAC-PandaQT.torrent",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the stage"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "[HK][东邪西毒.终极版.Ashes.Of.Time.Redux.2008][日版.1080p.REMUX]国粤配][srt.ass简英字幕.sup简繁][30G]",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"东邪西毒"},
				Year:    2008,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "One And Only 2023 HDTV 1080i MP2 H.264-TPTV",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"one and only"},
				Year:    2023,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Bone Collector 1999 UHD BluRay 2160p HEVC DTS-HD MA5.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the bone collector"},
				Year:    1999,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Bone Collector 1999 BluRay 1080p AVC DTS-HD MA5.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the bone collector"},
				Year:    1999,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Bad Guys 2 2025 BluRay 1080p AVC Atmos TrueHD7.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the bad guys 2"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Bad Guys 2 2025 UHD BluRay 2160p HEVC Atmos TrueHD7.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the bad guys 2"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Longest Nite 1998 BluRay 1080p AVC TrueHD 5.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the longest nite"},
				Year:    1998,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Volunteers To the War 2023 BluRay 1080p AVC  DD5.1 -MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the volunteers to the war"},
				Year:    2023,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Battle of Life and Death 2024 BluRay 1080p AVC DD5.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the battle of life and death"},
				Year:    2024,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Happyend 2024 BluRay 1080p AVC TrueHD 5.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"happyend"},
				Year:    2024,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Nobody 2 2025 UHD BluRay 2160p HEVC Atmos TrueHD7.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"nobody 2"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Nobody 2 2025 BluRay 1080p AVC Atmos TrueHD7.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"nobody 2"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Life of Chuck 2024 UHD BluRay 2160p HEVC DTS-HD MA5.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the life of chuck"},
				Year:    2024,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "F1 The Movie 2025 BluRay 1080p AVC Atmos TrueHD7.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"f1 the movie"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Last Mile 2024 1080p BluRay REMUX AVC DTS-HD MA 5.1-SupaHacka",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"last mile"},
				Year:    2024,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Love of Siam 2007 REMUX 1080p Blu-ray AVC DTS-HD MA 5.1-c0kE",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the love of siam"},
				Year:    2007,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "One And Only 2023 HDTV 1080i MP2 H.264-TPTV",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"one and only"},
				Year:    2023,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Bone Collector 1999 UHD BluRay 2160p HEVC DTS-HD MA5.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the bone collector"},
				Year:    1999,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Bone Collector 1999 BluRay 1080p AVC DTS-HD MA5.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the bone collector"},
				Year:    1999,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Bad Guys 2 2025 BluRay 1080p AVC Atmos TrueHD7.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the bad guys 2"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Bad Guys 2 2025 UHD BluRay 2160p HEVC Atmos TrueHD7.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the bad guys 2"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Longest Nite 1998 BluRay 1080p AVC TrueHD 5.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the longest nite"},
				Year:    1998,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Volunteers To the War 2023 BluRay 1080p AVC  DD5.1 -MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the volunteers to the war"},
				Year:    2023,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Battle of Life and Death 2024 BluRay 1080p AVC DD5.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the battle of life and death"},
				Year:    2024,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Happyend 2024 BluRay 1080p AVC TrueHD 5.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"happyend"},
				Year:    2024,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Nobody 2 2025 UHD BluRay 2160p HEVC Atmos TrueHD7.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"nobody 2"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Nobody 2 2025 BluRay 1080p AVC Atmos TrueHD7.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"nobody 2"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Life of Chuck 2024 UHD BluRay 2160p HEVC DTS-HD MA5.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the life of chuck"},
				Year:    2024,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "F1 The Movie 2025 BluRay 1080p AVC Atmos TrueHD7.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"f1 the movie"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Last Mile 2024 1080p BluRay REMUX AVC DTS-HD MA 5.1-SupaHacka",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"last mile"},
				Year:    2024,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Love of Siam 2007 REMUX 1080p Blu-ray AVC DTS-HD MA 5.1-c0kE",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the love of siam"},
				Year:    2007,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Yeogo goedam 5 Dong ban ja sal AKA A Blood Pledge AKA Whispering Corridors 5 Suicide Pact 2009 DVD5 Remux 480i MPEG-2 DTS",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"yeogo goedam 5 dong ban ja sal aka a blood pledge aka whispering corridors 5 suicide pact"},
				Year:    2009,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Acts of Violence 2018 1080p Blu-ray AVC DTS-HD MA 5.1-Huan@HDSky",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"acts of violence"},
				Year:    2018,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Conjuring: Last Rites 2025 Hybrid 2160p MA WEB-DL DDP 5.1 Atmos DV HDR H.265-HONE",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the conjuring: last rites"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Sirāt 2025 2160p MVSTP WEB-DL DD+5.1 HDR H265-HDZ",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"sirāt"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Crank 2006 GER Extended Cut BluRay 2160p DTS-HDMA5.1 DoVi HDR10 x265 10bit-CHD",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"crank"},
				Year:    2006,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Do the Right Thing 1989 2160p WEB-DL H.264 AAC 2.0-CSWEB",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"do the right thing"},
				Year:    1989,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Boys Next Door 1985 2160p UHD Blu-ray HDR10 HEVC DTS-HD MA 5.1-BLoz",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the boys next door"},
				Year:    1985,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Symphonie pour un massacre 1963 1080p BluRay x264 FLAC 2.0 2Audio-ADE",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"symphonie pour un massacre"},
				Year:    1963,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Warfare 2025 BluRay 2160p TrueHD7.1 DoVi HDR x265 10bit-CHD",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"warfare"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Great Dictator 1940 1080p CC BluRay Remux AVC FLAC 1.0-ADE",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the great dictator"},
				Year:    1940,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Six Assassins 1971 USA Blu-ray 1080p AVC DTS-HD MA 2.0-DIY@Hero",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"six assassins"},
				Year:    1971,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Core 2003 2160p UHD Blu-ray DoVi HDR10 HEVC DTS-HD MA 5.1",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the core"},
				Year:    2003,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Chinese Feast 1995 1080p BluRay Remux AVC TrueHD 5.1 2Audio-ADE",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the chinese feast"},
				Year:    1995,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Fastest Sword 1968 USA Blu-ray 1080p AVC DTS-HD MA 2.0-DIY@Hero",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the fastest sword"},
				Year:    1968,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Christine 1958 1080p AMZN WEB-DL H.264 DDP 2.0-SPWEB",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"christine"},
				Year:    1958,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Altered States 1980 1080p USA Blu-ray AVC DTS-HD MA 2.0 3Audio-TMT",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"altered states"},
				Year:    1980,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Moonrise Kingdom 2012 2160p UHD Blu-ray Remux DV HEVC DTS-HD MA5.1-HDS",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"moonrise kingdom"},
				Year:    2012,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Jurassic World Rebirth 2025 2160p GER UHD Blu-ray HEVC Atmos TrueHD7.1-HDH",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"jurassic world rebirth"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Sinners 2025 2160p EUR UHD Blu-ray HEVC Atmos TrueHD7.1-HDH",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"sinners"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Black Sunday AKA La maschera del demonio 1960 1080p Blu-ray AVC DTS-HD MA 2.0 5Audio-INCUBO",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"black sunday aka la maschera del demonio"},
				Year:    1960,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Thieves Like Us 1974 Blu-ray 1080p AVC DTS-HD MA 2.0-GMA",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"thieves like us"},
				Year:    1974,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Paddington 2 2017 HDTV 1080i MP2 H.264-TPTV",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"paddington 2"},
				Year:    2017,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The NeverEnding Story II The Next Chapter 1990 2160p UHD Blu-ray DoVi HDR10 HEVC DTS-HD MA 5.1 8Audio-DIY@HDSky",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the neverending story ii the next chapter"},
				Year:    1990,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Rocky Horror Picture Show 1975 2160p UHD Blu-ray DoVi HDR10 HEVC TrueHD 7.1-TMT",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the rocky horror picture show"},
				Year:    1975,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "G I  Joe Retaliation 2013 2160p UHD Blu-ray DV TrueHD 7.1 3Audio x265-HDH",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"g i joe retaliation"},
				Year:    2013,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Le Havre 2011 1080p BluRay DTS-HD MA 5.1 x264-HDH",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"le havre"},
				Year:    2011,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Russendisko 2012 1080p GER Blu-ray AVC DTS-HD MA 5.1-SharpHD",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"russendisko"},
				Year:    2012,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Departed 2006 1080p Blu-ray AVC DTS-HD MA 5.1 2Audio-NoGrp",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the departed"},
				Year:    2006,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Bone Collector 1999 2160p UHD Blu-ray HEVC DTS-HD MA5.1-HDH",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the bone collector"},
				Year:    1999,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Anora 2024 2160p BluRay HDR10+ x265 DTS-HD MA 5.1 3Audio-MainFrame",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"anora"},
				Year:    2024,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Hunt 2020 BluRay 1080p x265 10bit DDP7.1 MNHD-FRDS",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the hunt"},
				Year:    2020,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Family 2021 1080p GER Blu-ray MPEG-2 DTS-HD MA 5.1-SharpHD",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the family"},
				Year:    2021,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "L Ultima Volta Che Siamo Stati Bambini 2023 BluRay 1080p x265 10bit DDP5.1 MNHD-FRDS",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"l ultima volta che siamo stati bambini"},
				Year:    2023,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Creed II 2018 USA BluRay 2160p TrueHD7.1 DoVi HDR10 x265 10bit-CHD",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"creed ii"},
				Year:    2018,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Smile 2022 UHD Bluray 2160p DV HDR x265 10bit Atmos TrueHD 7.1 2Audio-UBits",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"smile"},
				Year:    2022,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "FF9 2021 2160p HQ 60fps WEB-DL H.265 HDR AAC 2.0 2Audio-ZmWeb",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"ff9"},
				Year:    2021,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Weird Man 1983 1080p Blu-ray AVC LPCM 2.0-MKu",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the weird man"},
				Year:    1983,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Wind Is Blowing 2020 HDTV 1080i MP2 H.264-TPTV",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the wind is blowing"},
				Year:    2020,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Running Man 1987 2160p FRA UHD Blu-ray DV HDR HEVC DTS-HD MA 5.1-DIY@HDSky",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the running man"},
				Year:    1987,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Dongji Island 2025 2160p HQ WEB-DL H.265 10bit HDR DoVi DDP 5.1-CMCTV",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"dongji island"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Ruth and Boaz 2025 2160p NF WEB-DL DV H.265 DDP5.1 Atmos-ADWeb",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"ruth and boaz"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Uranus 2324 2024 2160p friDay WEB-DL H.265 AAC 2.0-UBWEB",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"uranus 2324"},
				Year:    2024,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "I Swear 2025 2160p HQ WEB-DL H.265 10bit HDR DoVi DDP 5.1-CMCTV",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"i swear"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "F1 The Movie 2025 2160p UHD BluRay x265 10bit DV HDR10 TrueHD 7.1 Atmos-Panda",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"f1 the movie"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "2:37 2006 EUR BluRay AVC LPCM  2Audio-TYZH@HDSky",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"2:37"},
				Year:    2006,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Jolly Monkey 2025 1080p Blu-ray AVC DTS-HD MA 5.1-iFPD",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the jolly monkey"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Sinners 2025 USA BluRay Remux AVC 1080p Atmos TrueHD7.1-CHD",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"sinners"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Man in Black 1950 2160p UHD Blu-ray DoVi HDR10 HEVC DTS-HD MA 5.1-LWRTD",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the man in black"},
				Year:    1950,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Life of Chuck 2024 BluRay 2160p HDR x265 DTS-HD MA 5.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the life of chuck"},
				Year:    2024,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Shimoni 2022 1080p WEB-DL AAC2.0 x264-ZTR",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"shimoni"},
				Year:    2022,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Conjuring Last Rites 2025 2160p iTunes WEB-DL DDP 5.1 Atmos DV H.265-CHDWEB",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the conjuring last rites"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Conjuring Last Rites 2025 2160p iTunes WEB-DL DDP 5.1 Atmos H.265-CHDWEB",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the conjuring last rites"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Fetus 2025 1080p Blu-ray AVC DTS-HD MA 5.1-PtBM",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the fetus"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Fantastic Four: First Steps 2025 2160p BluRay DoVi x265 10bit 3Audios TrueHD Atmos 7.1-WiKi",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the fantastic four: first steps"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Life of Chuck 2024 BluRay 1080p x265 DTS-HD MA 5.1-MTeam",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the life of chuck"},
				Year:    2024,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Habit 2021 BluRay 1080p x265 10bit DDP5.1 MNHD-FRDS",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"habit"},
				Year:    2021,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Million Eyes of Sumuru 1967 2160p UHD Blu-ray DoVi HDR10 HEVC DD 2.0-DIY@HDSky",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the million eyes of sumuru"},
				Year:    1967,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Past 2013 USA 1080p Blu-ray AVC DTS-HD MA 5.1-blucook#792@CHDBits",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the past"},
				Year:    2013,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Smiles of a Summer Night 1955 BFI Blu-ray 1080p AVC LPCM 1.0-blucook#344@CHDBits",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"smiles of a summer night"},
				Year:    1955,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Conjuring Last Rites 2025 2160p iTunes WEB-DL DDP 5.1 Atmos HDR10+ H.265-CHDWEB",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the conjuring last rites"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Naked City 1948 1080p BluRay AVC LPCM 1.0 2Audio-DiY@HDHome",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the naked city"},
				Year:    1948,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Tuche Family 2011 1080p FRA Blu-ray VC-1 DTS-HD MA 5.1-F13@HDSpace",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the tuche family"},
				Year:    2011,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Conjuring Last Rites 2025 1080p WEB-DL HEVC x265 5.1 BONE",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the conjuring last rites"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Litchi Road 2025 2160p WEB-DL H.265 AAC 2.0-CMCTV",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the litchi road"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Dongji Island 2025 2160p WEB-DL H.265 AAC 2.0-CMCTV",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"dongji island"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Noise 2023 1080p NF WEB-DL DDP5.1 Atmos H.264-HHWEB",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"noise"},
				Year:    2023,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "For Our Pure Time 2021 2160p WEB-DL H.265 DDP 2.0 2Audio-HHWEB",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"for our pure time"},
				Year:    2021,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Way of the Househusband: The Cinema 2022 2160p WEB-DL H.264 AAC 2.0 2Audio-CSWEB",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the way of the househusband: the cinema"},
				Year:    2022,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Dog Days 2018 1080p GER Blu-ray AVC DTS-HD MA 5.1.2Audios-PTer",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"dog days"},
				Year:    2018,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Lv Jian Jiang 2020 2160p WEB-DL H.265 DDP 2.0 2Audio5.1-HHWEB",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"lv jian jiang"},
				Year:    2020,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Happyend 2024 BluRay 1080p x265 10bit DDP5.1 MNHD-FRDS",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"happyend"},
				Year:    2024,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Primal Fear 1996 2160p NF WEB-DL DV H.265 DDP 5.1-CHDWEB",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"primal fear"},
				Year:    1996,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "The Black Tulip 1964 HDTV 1080i AAC2.0 H.264-TPTV",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"the black tulip"},
				Year:    1964,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "FF9 2021 2160p HQ WEB-DL H.265 DV AAC 2.0 2Audio-ZmWeb",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"ff9"},
				Year:    2021,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "Matt McCusker A Humble Offering 2025 1080p NF WEB-DL DDP5.1 H.264-MWeb",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"matt mccusker a humble offering"},
				Year:    2025,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "[黒ネズミたち] 妖怪旅馆营业中 贰 / Kakuriyo no Yadomeshi Ni - 01 (CR 1920x1080 AVC AAC MKV)[1080P]",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"妖怪旅馆营业中 贰"},
				Year:    0,
				Season:  0,
				Episode: 0,
			},
		},
		{
			filename: "====== ============ 💯【戏台】【4K高码】 💯【国语】 【中英字幕】 💯====== ============",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"戏台"},
				Year:    0,
				Season:  0,
				Episode: 0,
			},
		},
	}
	i := 1
	for _, tc := range testCases {
		info, err := client.TakeMoiveName(tc.filename, DEFAULT_MOVIE_PROMPT)
		if err != nil {
			t.Fatalf("AI提取视频信息失败： '%s', 错误：%v", tc.filename, err)
			continue
		}
		if info == nil {
			t.Fatalf("AI提取视频信息失败： '%s'", tc.filename)
			continue
		}
		// 验证函数能够正常工作，并且返回的MediaInfo结构有效
		if !slices.Contains(tc.expectedMediaInfo.Name, info.Name) {
			t.Errorf("AI提取视频信息失败： '%s', 识别到的电影名称 %s 与预期名称 '%+v' 不符", tc.filename, info.Name, tc.expectedMediaInfo.Name)
			continue
		}
		if info.Year != tc.expectedMediaInfo.Year {
			t.Errorf("AI提取视频信息失败： '%s', 视频年份 %d 与预期 %d 不符", tc.filename, info.Year, tc.expectedMediaInfo.Year)
			continue
		}
		i++
	}
	fmt.Printf("共测试完成 %d 个电影标题\n", i)
}

func TestExtractMediaInfo_Tvshow(t *testing.T) {
	client := NewClient(DEFAULT_API_KEY, DEFAULT_API_BASE_URL, DEFAULT_MODEL_NAME, DEFAULT_TIMEOUT)
	testCases := TestCases{
		{
			filename: "【漫游字幕组】[进击的巨人_Attack on Titan][S04E16][1080p][CHS].mkv",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"进击的巨人"},
				Year:    0,
				Season:  4,
				Episode: 16,
			},
		},
		{
			filename: "【银色子弹字幕组】[名侦探柯南][第74集 死神阵内杀人事件][WEBRIP][简日双语MP4/繁日双语MP4/简繁日多语MKV][1080P]",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"名侦探柯南"},
				Year:    0,
				Season:  1,
				Episode: 74,
			},
		},
		{
			filename: "人民的名义.S01E34.利剑行动开始.mkv",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"人民的名义"},
				Year:    0,
				Season:  1,
				Episode: 34,
			},
		},
		{
			filename: "棋士.Playing.Go.S01E01.2025.2160p.WEB-DL.H265.DV.DDP5.1.Atmos.mp4",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"棋士"},
				Year:    2025,
				Season:  1,
				Episode: 1,
			},
		},
		{
			filename: "知否知否应是绿肥红瘦 66.mp4",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{"知否知否应是绿肥红瘦"},
				Year:    0,
				Season:  1,
				Episode: 66,
			},
		},
		{
			filename: "66.mp4",
			expectedMediaInfo: &MediaInfo{
				Name:    []string{""},
				Year:    0,
				Season:  1,
				Episode: 66,
			},
		},
	}
	i := 1
	for _, tc := range testCases {
		info, err := client.TakeMoiveName(tc.filename, DEFAULT_MOVIE_PROMPT)
		if err != nil {
			t.Fatalf("AI提取视频信息失败： '%s', 错误：%v", tc.filename, err)
			continue
		}
		// 验证函数能够正常工作，并且返回的MediaInfo结构有效
		if !slices.Contains(tc.expectedMediaInfo.Name, info.Name) {
			t.Errorf("AI提取视频信息失败： '%s', 识别到的电视剧名称 %s 与预期名称 '%+v' 不符", tc.filename, info.Name, tc.expectedMediaInfo.Name)
			continue
		}
		if info.Year != tc.expectedMediaInfo.Year {
			t.Errorf("AI提取视频信息失败： '%s', 视频年份 %d 与预期 %d 不符", tc.filename, info.Year, tc.expectedMediaInfo.Year)
			continue
		}
		i++
	}
	fmt.Printf("共测试完成 %d 个电视剧标题\n", i)
}
