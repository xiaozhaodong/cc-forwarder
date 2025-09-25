package response

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
)

func TestDecompressStreamReader(t *testing.T) {
	processor := NewProcessor()

	t.Run("无压缩内容", func(t *testing.T) {
		// 构造未压缩的响应
		originalData := "event: message_start\ndata: {\"type\":\"message_start\"}\n\n"
		resp := &http.Response{
			Body:   io.NopCloser(strings.NewReader(originalData)),
			Header: make(http.Header),
		}

		// 获取解压缩读取器
		reader, err := processor.DecompressStreamReader(resp)
		if err != nil {
			t.Fatalf("DecompressStreamReader失败: %v", err)
		}
		defer reader.Close()

		// 验证内容一致性
		result, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("读取解压缩内容失败: %v", err)
		}

		if string(result) != originalData {
			t.Errorf("内容不匹配.\n期望: %q\n实际: %q", originalData, string(result))
		}
	})

	t.Run("gzip压缩内容", func(t *testing.T) {
		// 原始SSE数据（包含token信息）
		originalData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","model":"claude-3-sonnet-20241022"}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: ping
data: {"type": "ping"}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":10,"output_tokens":5}}

event: message_stop
data: {"type":"message_stop"}

`

		// 压缩数据
		var compressedBuffer bytes.Buffer
		gzipWriter := gzip.NewWriter(&compressedBuffer)
		if _, err := gzipWriter.Write([]byte(originalData)); err != nil {
			t.Fatalf("gzip压缩失败: %v", err)
		}
		if err := gzipWriter.Close(); err != nil {
			t.Fatalf("关闭gzip writer失败: %v", err)
		}

		// 构造gzip压缩的响应
		resp := &http.Response{
			Body:   io.NopCloser(bytes.NewReader(compressedBuffer.Bytes())),
			Header: make(http.Header),
		}
		resp.Header.Set("Content-Encoding", "gzip")

		// 获取解压缩读取器
		reader, err := processor.DecompressStreamReader(resp)
		if err != nil {
			t.Fatalf("DecompressStreamReader失败: %v", err)
		}
		defer reader.Close()

		// 流式读取解压缩内容
		var result bytes.Buffer
		buffer := make([]byte, 1024)
		for {
			n, err := reader.Read(buffer)
			if n > 0 {
				result.Write(buffer[:n])
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("流式读取失败: %v", err)
			}
		}

		// 验证解压缩后的内容
		decompressed := result.String()
		if decompressed != originalData {
			t.Errorf("gzip解压缩内容不匹配.\n期望长度: %d\n实际长度: %d\n期望开头: %q\n实际开头: %q",
				len(originalData), len(decompressed),
				originalData[:min(50, len(originalData))],
				decompressed[:min(50, len(decompressed))])
		}

		// 特别验证token相关内容
		if !strings.Contains(decompressed, `"input_tokens":10,"output_tokens":5`) {
			t.Error("解压缩后的内容应包含token使用信息")
		}
		if !strings.Contains(decompressed, `"model":"claude-3-sonnet-20241022"`) {
			t.Error("解压缩后的内容应包含模型信息")
		}
	})

	t.Run("brotli压缩内容", func(t *testing.T) {
		originalData := "event: message_stop\ndata: {\"type\":\"message_stop\",\"usage\":{\"input_tokens\":15,\"output_tokens\":8}}\n\n"

		// 压缩数据
		var compressedBuffer bytes.Buffer
		brotliWriter := brotli.NewWriter(&compressedBuffer)
		if _, err := brotliWriter.Write([]byte(originalData)); err != nil {
			t.Fatalf("brotli压缩失败: %v", err)
		}
		if err := brotliWriter.Close(); err != nil {
			t.Fatalf("关闭brotli writer失败: %v", err)
		}

		// 构造brotli压缩的响应
		resp := &http.Response{
			Body:   io.NopCloser(bytes.NewReader(compressedBuffer.Bytes())),
			Header: make(http.Header),
		}
		resp.Header.Set("Content-Encoding", "br")

		// 获取解压缩读取器
		reader, err := processor.DecompressStreamReader(resp)
		if err != nil {
			t.Fatalf("DecompressStreamReader失败: %v", err)
		}
		defer reader.Close()

		// 读取解压缩内容
		result, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("读取brotli解压缩内容失败: %v", err)
		}

		if string(result) != originalData {
			t.Errorf("brotli解压缩内容不匹配.\n期望: %q\n实际: %q", originalData, string(result))
		}
	})

	t.Run("未知压缩格式回退", func(t *testing.T) {
		originalData := "event: error\ndata: {\"error\":\"unknown encoding\"}\n\n"
		resp := &http.Response{
			Body:   io.NopCloser(strings.NewReader(originalData)),
			Header: make(http.Header),
		}
		resp.Header.Set("Content-Encoding", "unknown-encoding")

		// 应该回退到原始读取器
		reader, err := processor.DecompressStreamReader(resp)
		if err != nil {
			t.Fatalf("DecompressStreamReader应该回退成功: %v", err)
		}
		defer reader.Close()

		// 验证内容未被修改
		result, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("读取回退内容失败: %v", err)
		}

		if string(result) != originalData {
			t.Errorf("未知编码回退内容不匹配.\n期望: %q\n实际: %q", originalData, string(result))
		}
	})

	t.Run("gzip错误数据处理", func(t *testing.T) {
		// 构造无效的gzip数据
		invalidGzipData := []byte("这不是有效的gzip数据")
		resp := &http.Response{
			Body:   io.NopCloser(bytes.NewReader(invalidGzipData)),
			Header: make(http.Header),
		}
		resp.Header.Set("Content-Encoding", "gzip")

		// 应该返回错误
		_, err := processor.DecompressStreamReader(resp)
		if err == nil {
			t.Fatal("期望gzip解压缩错误，但没有收到错误")
		}

		if !strings.Contains(err.Error(), "failed to create gzip stream reader") {
			t.Errorf("错误信息不符合预期: %v", err)
		}
	})
}

// TestStreamingTokenExtraction 测试流式token提取功能
func TestStreamingTokenExtraction(t *testing.T) {
	processor := NewProcessor()

	// 构造包含完整token信息的gzip压缩SSE流
	originalSSE := `event: message_start
data: {"type":"message_start","message":{"id":"msg_test","model":"claude-3-sonnet-20241022"}}

event: content_block_start
data: {"type":"content_block_start","index":0}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Test response"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":25,"output_tokens":12}}

event: message_stop
data: {"type":"message_stop"}

`

	// 压缩SSE数据
	var compressedBuffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressedBuffer)
	gzipWriter.Write([]byte(originalSSE))
	gzipWriter.Close()

	// 创建压缩响应
	resp := &http.Response{
		Body:   io.NopCloser(bytes.NewReader(compressedBuffer.Bytes())),
		Header: make(http.Header),
	}
	resp.Header.Set("Content-Encoding", "gzip")

	// 获取解压缩读取器
	reader, err := processor.DecompressStreamReader(resp)
	if err != nil {
		t.Fatalf("创建解压缩读取器失败: %v", err)
	}
	defer reader.Close()

	// 逐行读取并验证关键token信息能被正确提取
	var allContent bytes.Buffer
	buffer := make([]byte, 512)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			allContent.Write(buffer[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("流式读取失败: %v", err)
		}
	}

	content := allContent.String()

	// 验证关键token信息存在且可读
	checks := []struct {
		name     string
		expected string
	}{
		{"模型信息", `"model":"claude-3-sonnet-20241022"`},
		{"输入token", `"input_tokens":25`},
		{"输出token", `"output_tokens":12`},
		{"消息停止事件", `event: message_stop`},
		{"内容块停止事件", `event: content_block_stop`},
	}

	for _, check := range checks {
		if !strings.Contains(content, check.expected) {
			t.Errorf("解压缩后的内容缺少%s: %q", check.name, check.expected)
		}
	}

	// 验证内容是可读文本而不是二进制乱码
	if bytes.Contains(allContent.Bytes(), []byte{0x1f, 0x8b}) {
		t.Error("解压缩后的内容仍包含gzip头部，表明解压缩未生效")
	}

	t.Logf("✅ 成功解压缩并提取token信息，内容长度: %d", len(content))
}

// min 辅助函数（兼容老版本Go）
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
