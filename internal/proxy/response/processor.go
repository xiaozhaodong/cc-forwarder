package response

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/lzw"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/andybalholm/brotli"
)

// Processor handles response processing including decompression
type Processor struct{}

// NewProcessor creates a new response processor
func NewProcessor() *Processor {
	return &Processor{}
}

// ReadAndDecompressResponse reads and decompresses the response body based on Content-Encoding
func (p *Processor) ReadAndDecompressResponse(ctx context.Context, resp *http.Response, endpointName string) ([]byte, error) {
	// Read the raw response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check Content-Encoding header
	contentEncoding := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	if contentEncoding == "" {
		// No encoding, return as is
		return bodyBytes, nil
	}

	// Handle different compression methods
	switch contentEncoding {
	case "gzip":
		return p.decompressGzip(ctx, bodyBytes, endpointName)
	case "deflate":
		return p.decompressDeflate(ctx, bodyBytes, endpointName)
	case "br":
		return p.decompressBrotli(ctx, bodyBytes, endpointName)
	case "compress":
		return p.decompressLZW(ctx, bodyBytes, endpointName)
	case "identity":
		// Identity means no encoding
		return bodyBytes, nil
	default:
		// Unknown encoding, log warning and return as is
		slog.WarnContext(ctx, fmt.Sprintf("⚠️ [压缩] 未知的编码方式，端点: %s, 编码: %s", endpointName, contentEncoding))
		return bodyBytes, nil
	}
}

// decompressGzip decompresses gzip encoded content
func (p *Processor) decompressGzip(ctx context.Context, bodyBytes []byte, endpointName string) ([]byte, error) {
	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [GZIP] 检测到gzip编码响应，端点: %s, 压缩长度: %d字节", endpointName, len(bodyBytes)))
	
	gzipReader, err := gzip.NewReader(bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	decompressedBytes, err := io.ReadAll(gzipReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress gzip content: %w", err)
	}

	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [GZIP] 解压完成，端点: %s, 解压后长度: %d字节", endpointName, len(decompressedBytes)))
	return decompressedBytes, nil
}

// decompressDeflate decompresses deflate encoded content
func (p *Processor) decompressDeflate(ctx context.Context, bodyBytes []byte, endpointName string) ([]byte, error) {
	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [DEFLATE] 检测到deflate编码响应，端点: %s, 压缩长度: %d字节", endpointName, len(bodyBytes)))
	
	deflateReader := flate.NewReader(bytes.NewReader(bodyBytes))
	defer deflateReader.Close()

	decompressedBytes, err := io.ReadAll(deflateReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress deflate content: %w", err)
	}

	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [DEFLATE] 解压完成，端点: %s, 解压后长度: %d字节", endpointName, len(decompressedBytes)))
	return decompressedBytes, nil
}

// decompressBrotli decompresses Brotli encoded content
func (p *Processor) decompressBrotli(ctx context.Context, bodyBytes []byte, endpointName string) ([]byte, error) {
	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [BROTLI] 检测到br编码响应，端点: %s, 压缩长度: %d字节", endpointName, len(bodyBytes)))
	
	brotliReader := brotli.NewReader(bytes.NewReader(bodyBytes))

	decompressedBytes, err := io.ReadAll(brotliReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress brotli content: %w", err)
	}

	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [BROTLI] 解压完成，端点: %s, 解压后长度: %d字节", endpointName, len(decompressedBytes)))
	return decompressedBytes, nil
}

// decompressLZW decompresses LZW (compress) encoded content
func (p *Processor) decompressLZW(ctx context.Context, bodyBytes []byte, endpointName string) ([]byte, error) {
	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [LZW] 检测到compress编码响应，端点: %s, 压缩长度: %d字节", endpointName, len(bodyBytes)))
	
	// LZW reader with MSB order (standard for HTTP compress)
	lzwReader := lzw.NewReader(bytes.NewReader(bodyBytes), lzw.MSB, 8)
	defer lzwReader.Close()

	decompressedBytes, err := io.ReadAll(lzwReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress LZW content: %w", err)
	}

	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [LZW] 解压完成，端点: %s, 解压后长度: %d字节", endpointName, len(decompressedBytes)))
	return decompressedBytes, nil
}

// CopyResponseHeaders 复制响应头到客户端
func (p *Processor) CopyResponseHeaders(resp *http.Response, w http.ResponseWriter) {
	for key, values := range resp.Header {
		// 跳过一些不应该复制的头部
		switch key {
		case "Content-Length", "Transfer-Encoding", "Connection", "Content-Encoding":
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
}

// ProcessResponseBody 处理响应体（包括解压缩）
func (p *Processor) ProcessResponseBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	
	// 检查内容编码并解压缩
	encoding := resp.Header.Get("Content-Encoding")
	switch encoding {
	case "gzip":
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
		
	case "deflate":
		reader = flate.NewReader(resp.Body)
		
	case "br":
		reader = brotli.NewReader(resp.Body)
		
	case "compress":
		reader = lzw.NewReader(resp.Body, lzw.LSB, 8)
	}
	
	// 读取响应体
	responseBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	return responseBytes, nil
}