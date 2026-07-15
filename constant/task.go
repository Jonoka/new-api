package constant

type TaskPlatform string

const (
	TaskPlatformSuno        TaskPlatform = "suno"
	TaskPlatformMidjourney               = "mj"
	TaskPlatformCanvasImage              = "canvas_image"
	TaskPlatformImage                    = "image"
)

// ImageTaskPlatforms 返回由本服务本地执行并保存结果的异步图片任务平台。
// 返回新切片，避免调用方修改共享状态。
func ImageTaskPlatforms() []TaskPlatform {
	return []TaskPlatform{TaskPlatformCanvasImage, TaskPlatformImage}
}

func IsImageTaskPlatform(platform TaskPlatform) bool {
	return platform == TaskPlatformCanvasImage || platform == TaskPlatformImage
}

const (
	SunoActionMusic  = "MUSIC"
	SunoActionLyrics = "LYRICS"

	TaskActionGenerate          = "generate"
	TaskActionTextGenerate      = "textGenerate"
	TaskActionFirstTailGenerate = "firstTailGenerate"
	TaskActionReferenceGenerate = "referenceGenerate"
	TaskActionRemix             = "remixGenerate"
)

var SunoModel2Action = map[string]string{
	"suno_music":  SunoActionMusic,
	"suno_lyrics": SunoActionLyrics,
}
