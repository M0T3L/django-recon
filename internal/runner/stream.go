package runner

import (
	"bufio"
	"io"
)

// LineHandler is a function callback signature that processes an individual output line.
type LineHandler func(line []byte) error

// StreamConfig allows configuring the buffer capacity for streaming lines.
type StreamConfig struct {
	MaxBufferSize int
}

// DefaultMaxBufferSize sets max buffer size to 10MB to accommodate large single-line JSON outputs.
const DefaultMaxBufferSize = 10 * 1024 * 1024

// StreamOutput reads an io.Reader line-by-line using bufio.Scanner without writing to disk
// and passes each line to the provided LineHandler callback.
func StreamOutput(r io.Reader, handler LineHandler) error {
	return StreamOutputWithConfig(r, handler, StreamConfig{MaxBufferSize: DefaultMaxBufferSize})
}

// StreamOutputWithConfig streams lines with a custom StreamConfig.
func StreamOutputWithConfig(r io.Reader, handler LineHandler, cfg StreamConfig) error {
	if handler == nil {
		return nil
	}

	scanner := bufio.NewScanner(r)

	maxBufSize := cfg.MaxBufferSize
	if maxBufSize <= 0 {
		maxBufSize = DefaultMaxBufferSize
	}

	// Allocate buffer and specify max token size
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxBufSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		// Make a copy of line bytes to prevent caller issues with scanner buffer reuse
		lineCopy := make([]byte, len(line))
		copy(lineCopy, line)

		if err := handler(lineCopy); err != nil {
			return err
		}
	}

	return scanner.Err()
}
