package open123

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	client := NewClient("test-token")
	if client == nil {
		t.Fatal("Failed to create client")
	}

	if client.accessToken != "test-token" {
		t.Errorf("Expected access token 'test-token', got '%s'", client.accessToken)
	}

	if client.baseURL != OPEN_BASE_URL {
		t.Errorf("Expected base URL '%s', got '%s'", OPEN_BASE_URL, client.baseURL)
	}

	if client.clientID != CLIENT_ID {
		t.Errorf("Expected client ID '%s', got '%s'", CLIENT_ID, client.clientID)
	}

	if client.clientSecret != CLIENT_SECRET {
		t.Errorf("Expected client secret '%s', got '%s'", CLIENT_SECRET, client.clientSecret)
	}

	if client.ua != DEFAULTUA {
		t.Errorf("Expected user agent '%s', got '%s'", DEFAULTUA, client.ua)
	}
}

func TestSetRateLimit(t *testing.T) {
	client := NewClient("test-token")
	if client == nil {
		t.Fatal("Failed to create client")
	}

	// 设置QPS限制
	client.SetRateLimit("/upload/v1/file/create", 5)

	// 验证限制器是否已设置
	client.limiterLock.RLock()
	_, exists := client.limiters["/upload/v1/file/create"]
	client.limiterLock.RUnlock()

	if !exists {
		t.Error("Rate limiter was not set")
	}
}

func TestFileUploadCreate(t *testing.T) {
	// 创建客户端实例
	client := NewClient("test-token")

	// 创建文件上传请求
	req := &FileUploadCreateRequest{
		ParentFileID: 0,
		Filename:     "测试文件.txt",
		Etag:         "0a05e3dcd8ba1d14753597bc8611d0a1",
		Size:         44321,
	}

	// 由于需要真实的API访问，这里只测试请求是否能正确构建
	url := client.baseURL + "/upload/v1/file/create"
	if url != "https://open-api.123pan.com/upload/v1/file/create" {
		t.Errorf("Expected URL 'https://open-api.123pan.com/upload/v1/file/create', got '%s'", url)
	}

	// 验证请求参数是否正确
	if req.ParentFileID != 0 {
		t.Errorf("Expected ParentFileID 0, got %d", req.ParentFileID)
	}

	if req.Filename != "测试文件.txt" {
		t.Errorf("Expected Filename '测试文件.txt', got '%s'", req.Filename)
	}

	if req.Etag != "0a05e3dcd8ba1d14753597bc8611d0a1" {
		t.Errorf("Expected Etag '0a05e3dcd8ba1d14753597bc8611d0a1', got '%s'", req.Etag)
	}

	if req.Size != 44321 {
		t.Errorf("Expected Size 44321, got %d", req.Size)
	}
}