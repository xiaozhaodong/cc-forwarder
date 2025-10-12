package response

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProcessResponseBody(t *testing.T) {
	processor := NewProcessor()

	t.Run("ProcessResponseBody_无压缩", func(t *testing.T) {
		originalData := `{"type":"message","usage":{"input_tokens":10,"output_tokens":5}}`
		resp := &http.Response{
			Body:   io.NopCloser(strings.NewReader(originalData)),
			Header: make(http.Header),
		}

		result, err := processor.ProcessResponseBody(resp)
		if err != nil {
			t.Fatalf("ProcessResponseBody失败: %v", err)
		}

		if string(result) != originalData {
			t.Errorf("内容不匹配.\n期望: %q\n实际: %q", originalData, string(result))
		}
	})

	t.Run("ProcessResponseBody_gzip压缩", func(t *testing.T) {
		// 原始JSON响应
		originalData := `{
			"type": "message",
			"usage": {"input_tokens": 25, "output_tokens": 15},
			"model": "claude-3-sonnet-20241022"
		}`

		// 压缩数据
		var compressedBuffer bytes.Buffer
		gzipWriter := gzip.NewWriter(&compressedBuffer)
		gzipWriter.Write([]byte(originalData))
		gzipWriter.Close()

		// 构造gzip响应
		resp := &http.Response{
			Body:   io.NopCloser(bytes.NewReader(compressedBuffer.Bytes())),
			Header: make(http.Header),
		}
		resp.Header.Set("Content-Encoding", "gzip")

		// 使用ProcessResponseBody处理
		result, err := processor.ProcessResponseBody(resp)
		if err != nil {
			t.Fatalf("ProcessResponseBody gzip处理失败: %v", err)
		}

		// 验证解压缩结果
		if string(result) != originalData {
			t.Errorf("gzip解压缩内容不匹配.\n期望: %q\n实际: %q", originalData, string(result))
		}

		// 验证包含token信息
		resultStr := string(result)
		if !strings.Contains(resultStr, `"input_tokens": 25`) {
			t.Error("解压缩后应包含input_tokens信息")
		}
		if !strings.Contains(resultStr, `"output_tokens": 15`) {
			t.Error("解压缩后应包含output_tokens信息")
		}
	})

	t.Run("一致性验证_流式vs批量", func(t *testing.T) {
		// 测试同一份gzip数据在两种模式下的结果一致性
		originalData := `{"id":"msg_test","type":"message","model":"claude-3-sonnet","usage":{"input_tokens":30,"output_tokens":20}}`

		// 压缩数据
		var compressedBuffer bytes.Buffer
		gzipWriter := gzip.NewWriter(&compressedBuffer)
		gzipWriter.Write([]byte(originalData))
		gzipWriter.Close()

		compressedData := compressedBuffer.Bytes()

		// 方法1：ProcessResponseBody (批量模式)
		resp1 := &http.Response{
			Body:   io.NopCloser(bytes.NewReader(compressedData)),
			Header: make(http.Header),
		}
		resp1.Header.Set("Content-Encoding", "gzip")

		result1, err := processor.ProcessResponseBody(resp1)
		if err != nil {
			t.Fatalf("ProcessResponseBody失败: %v", err)
		}

		// 方法2：DecompressStreamReader (流式模式)
		resp2 := &http.Response{
			Body:   io.NopCloser(bytes.NewReader(compressedData)),
			Header: make(http.Header),
		}
		resp2.Header.Set("Content-Encoding", "gzip")

		reader, err := processor.DecompressStreamReader(resp2)
		if err != nil {
			t.Fatalf("DecompressStreamReader失败: %v", err)
		}
		defer reader.Close()

		result2, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("流式读取失败: %v", err)
		}

		// 验证两种方法结果一致
		if string(result1) != string(result2) {
			t.Errorf("两种方法结果不一致.\nProcessResponseBody: %q\nDecompressStreamReader: %q", string(result1), string(result2))
		}

		// 验证都解压缩正确
		if string(result1) != originalData {
			t.Errorf("批量模式解压缩错误.\n期望: %q\n实际: %q", originalData, string(result1))
		}

		t.Logf("✅ 两种解压缩方法结果一致，长度: %d", len(result1))
	})
}
