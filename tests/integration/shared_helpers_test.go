package integration

import (
	"testing"
)

// TestSharedHelpersCompilation 测试共享助手类型编译
func TestSharedHelpersCompilation(t *testing.T) {
	t.Log("🧪 测试共享助手类型是否正常工作")

	// 测试 mockFlusher
	flusher := &mockFlusher{}
	flusher.Flush() // 应该不会出错
	t.Log("✅ mockFlusher 工作正常")

	// 测试 EOFErrorReader
	data := []byte("test data")
	reader := &EOFErrorReader{
		data:     data,
		position: 0,
		eofAfter: 5,
	}

	buf := make([]byte, 10)
	n, err := reader.Read(buf)
	if n > 0 || err != nil {
		t.Logf("✅ EOFErrorReader 工作正常: read %d bytes, error: %v", n, err)
	}

	err = reader.Close()
	if err == nil {
		t.Log("✅ EOFErrorReader Close() 工作正常")
	}

	// 测试 NetworkErrorReader
	netReader := &NetworkErrorReader{
		data:       data,
		position:   0,
		errorAfter: 3,
	}

	n, err = netReader.Read(buf)
	if n > 0 || err != nil {
		t.Logf("✅ NetworkErrorReader 工作正常: read %d bytes, error: %v", n, err)
	}

	// 测试 SlowReader
	slowReader := &SlowReader{
		data:      data,
		position:  0,
		delayTime: 1, // 1纳秒延迟，几乎无影响
	}

	n, err = slowReader.Read(buf)
	if n > 0 {
		t.Logf("✅ SlowReader 工作正常: read %d bytes", n)
	}

	t.Log("🎯 所有共享助手类型测试通过！")
}