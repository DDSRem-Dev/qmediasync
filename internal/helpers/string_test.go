package helpers

import (
	"testing"
)

func TestMD5Hash(t *testing.T) {
	// 测试用例
	tests := []struct {
		input    string
		expected string
	}{
		{"test", "d8578edf8458ce06fbc5bb76a58c5ca4"},
		{"hello world", "5eb63bbbe01eeed093cb22bb8f5acdc3"},
		{"", "d41d8cd98f00b204e9800998ecf8427e"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := MD5Hash(tt.input)
			if result != tt.expected {
				t.Errorf("MD5Hash(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}
