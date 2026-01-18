package open123

// RespBase 基础响应结构
type RespBase[T any] struct {
	XTraceID string `json:"x-traceID"`
	Code     int    `json:"code"`
	Message  string `json:"message"`
	Data     T      `json:"data"`
}

// FileUploadCreateRequest 文件上传创建请求
type FileUploadCreateRequest struct {
	ParentFileID int64  `json:"parentFileID"` // 父目录ID，0表示根目录
	Filename     string `json:"filename"`     // 文件名
	Etag         string `json:"etag"`         // 文件MD5
	Size         int64  `json:"size"`         // 文件大小
}

// FileUploadCreateResponse 文件上传创建响应
type FileUploadCreateResponse struct {
	FileID       int64  `json:"fileID"`       // 文件ID
	UploadID     string `json:"uploadID"`     // 上传ID
	PartSize     int64  `json:"partSize"`     // 分片大小
	AlreadyExist bool   `json:"alreadyExist"` // 文件是否已存在
}
