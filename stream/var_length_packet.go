package stream

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// VarLenPacketStream 实现可变长度包头的数据流
type VarLenPacketStream struct {
	conn    io.ReadWriteCloser
	readBuf []byte
	buf     []byte
	maxSize uint64
}

var (
	ErrInvalidHeader = errors.New("invalid header")
)

// NewVarLenPacketStream 创建实例
func NewVarLenPacketStream(conn io.ReadWriteCloser, maxPacketSize uint64) *VarLenPacketStream {
	return &VarLenPacketStream{
		conn:    conn,
		readBuf: make([]byte, 1),
		buf:     make([]byte, maxPacketSize),
		maxSize: maxPacketSize,
	}
}

func (s *VarLenPacketStream) ReadByte() (byte, error) {
	_, err := io.ReadFull(s.conn, s.readBuf)
	return s.readBuf[0], err
}

// Read 实现io.Reader接口
func (s *VarLenPacketStream) Read(p []byte) (int, error) {
	// 读取包头长度
	length, err := binary.ReadUvarint(s)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalidHeader, err)
	}

	// 检查长度
	if length > s.maxSize {
		return 0, io.ErrShortBuffer
	}

	// 确保目标缓冲区足够大
	if uint64(len(p)) < length {
		return 0, io.ErrShortBuffer
	}

	// 读取实际数据
	return io.ReadFull(s.conn, p[:length])
}

// Write 实现io.Writer接口
func (s *VarLenPacketStream) Write(p []byte) (int, error) {
	// 检查数据长度
	l := len(p)
	if uint64(l) > s.maxSize {
		return 0, io.ErrShortBuffer
	}

	// 写入包头长度
	headerOff := binary.PutUvarint(s.buf, uint64(l))
	// 写入实际数据
	copy(s.buf[headerOff:], p)

	return s.conn.Write(s.buf[:l+headerOff])
}

// Close 实现io.Closer接口
func (s *VarLenPacketStream) Close() error {
	return s.conn.Close()
}
