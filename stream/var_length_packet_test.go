package stream

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// 实现最小化 ReadWriteCloser
type memStream struct {
	*bytes.Buffer
}

func (m *memStream) Close() error { return nil }

func newTestStream() io.ReadWriteCloser {
	return &memStream{bytes.NewBuffer(nil)}
}

func TestReadWritePacket(t *testing.T) {
	t.Run("normal_packet", func(t *testing.T) {
		stream := NewVarLenPacketStream(newTestStream(), 1024)
		testData := []byte("测试数据")

		// 写入测试
		if _, err := stream.Write(testData); err != nil {
			t.Fatalf("WritePacket failed: %v", err)
		}

		// 从底层缓冲区读取验证
		rawStream := stream.conn.(*memStream)
		if rawStream.Len() != len(testData)+1 { // 1字节长度头
			t.Errorf("Expected %d bytes written, got %d", len(testData)+2, rawStream.Len())
		}
	})

	t.Run("read_write_loopback", func(t *testing.T) {
		s := NewVarLenPacketStream(newTestStream(), 1024)
		origin := []byte("loopback测试")

		if _, err := s.Write(origin); err != nil {
			t.Fatal(err)
		}

		received := make([]byte, 1024)
		n, err := s.Read(received)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(origin, received[:n]) {
			t.Errorf("Expected %q, got %q", origin, received)
		}
	})
}

func TestEdgeCases(t *testing.T) {
	// 空包测试
	t.Run("empty_packet", func(t *testing.T) {
		s := NewVarLenPacketStream(newTestStream(), 1024)
		if _, err := s.Write(nil); err != nil {
			t.Error(err)
		}

		data := make([]byte, 1024)
		n, err := s.Read(data)
		if err != nil {
			t.Error(err)
		}
		if n != 0 {
			t.Errorf("Expected empty packet, got %v", data)
		}
	})

	// 错误头测试
	t.Run("broken_header", func(t *testing.T) {
		brokenStream := &memStream{bytes.NewBuffer([]byte{0x00})} // 不完整头
		s := NewVarLenPacketStream(brokenStream, 1024)
		buf := make([]byte, 1024)
		_, err := s.Read(buf)
		if !errors.Is(err, ErrInvalidHeader) {
			t.Errorf("Expected ErrInvalidHeader, got %v", err)
		}
	})
}
