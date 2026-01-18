package open123

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// FileUploadCreate 创建文件上传
func (c *Client) FileUploadCreate(ctx context.Context, req *FileUploadCreateRequest) (*RespBase[FileUploadCreateResponse], error) {
	url := fmt.Sprintf("%s/upload/v1/file/create", c.baseURL)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	defer resp.RawBody().Close()

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("request failed with status: %s", resp.Status())
	}

	var result RespBase[FileUploadCreateResponse]
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, err
	}

	return &result, nil
}
