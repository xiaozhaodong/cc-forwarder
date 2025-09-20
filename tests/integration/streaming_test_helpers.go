package integration

import (
	"errors"
	"io"
	"time"
)

// 共享的测试助手类型和方法
// 用于避免在多个测试文件中重复声明相同的mock类型

// mockFlusher 模拟HTTP Flusher
type mockFlusher struct{}

func (f *mockFlusher) Flush() {
	// Mock implementation
}

// EOFErrorReader 模拟EOF错误的读取器
type EOFErrorReader struct {
	data     []byte
	position int
	eofAfter int
}

func (r *EOFErrorReader) Read(p []byte) (n int, err error) {
	if r.position >= r.eofAfter {
		return 0, io.ErrUnexpectedEOF
	}

	remaining := len(r.data) - r.position
	if remaining == 0 {
		return 0, io.EOF
	}

	n = copy(p, r.data[r.position:])
	r.position += n

	if r.position >= r.eofAfter {
		return n, io.ErrUnexpectedEOF
	}

	return n, nil
}

func (r *EOFErrorReader) Close() error {
	return nil
}

// NetworkErrorReader 模拟网络错误的读取器
type NetworkErrorReader struct {
	data       []byte
	position   int
	errorAfter int
}

func (r *NetworkErrorReader) Read(p []byte) (n int, err error) {
	if r.position >= r.errorAfter {
		return 0, errors.New("network connection lost")
	}

	remaining := len(r.data) - r.position
	if remaining == 0 {
		return 0, errors.New("network connection lost")
	}

	n = copy(p, r.data[r.position:])
	r.position += n

	if r.position >= r.errorAfter {
		return n, errors.New("network connection lost")
	}

	return n, nil
}

func (r *NetworkErrorReader) Close() error {
	return nil
}

// SlowReader 模拟慢速读取的读取器
type SlowReader struct {
	data      []byte
	position  int
	delayTime time.Duration
}

func (r *SlowReader) Read(p []byte) (n int, err error) {
	// 模拟慢速网络
	time.Sleep(r.delayTime)

	remaining := len(r.data) - r.position
	if remaining == 0 {
		return 0, io.EOF
	}

	n = copy(p, r.data[r.position:])
	r.position += n

	return n, nil
}

func (r *SlowReader) Close() error {
	return nil
}