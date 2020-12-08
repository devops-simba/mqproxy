package main

import (
	"errors"
	"io"
	"net"
	"strings"

	"github.com/devops-simba/helpers"
	"github.com/eclipse/paho.mqtt.golang/packets"
)

const (
	Raw         ServiceProxyMode = "raw"
	PacketProxy ServiceProxyMode = "packets"

	FrontendToBackend ServiceProxyDirection = true
	BackendToFrontend ServiceProxyDirection = false
)

func isEOF(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}

	s := err.Error()
	if strings.Contains(s, "use of closed network connection") {
		return true
	}
	if strings.Contains(s, "unexpected EOF") {
		return true
	}
	return false
}

type ServiceProxyDirection bool

func (this ServiceProxyDirection) SourceConnectionName() string {
	return helpers.IIFs(bool(this), "Frontend", "Backend")
}
func (this ServiceProxyDirection) DestinationConnectionName() string {
	return helpers.IIFs(bool(this), "Backend", "Frontend")
}
func (this ServiceProxyDirection) String() string {
	return helpers.IIFs(bool(this), ">", "<")
}

type memoryBuffer struct {
	buffer []byte
	used   int
	length int
}

func newMemoryBuffer(capacity int) *memoryBuffer {
	return &memoryBuffer{buffer: make([]byte, capacity)}
}

func (this *memoryBuffer) remaining() int     { return this.length - this.used }
func (this *memoryBuffer) readBuffer() []byte { return this.buffer[this.length:] }
func (this *memoryBuffer) remove(n int) {
	this.length = copy(this.buffer, this.buffer[n:this.length])
	this.used = 0
}
func (this *memoryBuffer) add(n int) {
	this.length += n
}
func (this *memoryBuffer) Read(buffer []byte) (int, error) {
	numberOfBytesRead := copy(buffer, this.buffer[this.used:this.length])
	this.used += numberOfBytesRead
	if numberOfBytesRead < len(buffer) {
		return numberOfBytesRead, io.EOF
	} else {
		return numberOfBytesRead, nil
	}
}

type ServiceProxyMode string

func rawProxy(logger helpers.Logger, dir ServiceProxyDirection, src, dst net.Conn) error {
	buffer := newMemoryBuffer(65536)
	sourceName := dir.SourceConnectionName()
	destName := dir.DestinationConnectionName()
	for {
		buf := buffer.readBuffer()
		if len(buf) == 0 {
			logger.Errorf("Failed to read data from %s: Message is too big", sourceName)
			return helpers.StringError("Message is too big")
		}

		numberOfBytesRead, err := src.Read(buf)
		if err != nil {
			dst.Close()
			if isEOF(err) {
				logger.Debugf("%s connection closed", sourceName)
				return nil
			} else {
				src.Close()
				logger.Errorf("error in reading data from %s: %v", sourceName, err)
				return err
			}
		}
		buffer.add(numberOfBytesRead)

		used := 0
		for buffer.remaining() != 0 {
			pkt, err := packets.ReadPacket(buffer)
			if err == nil {
				logger.Verbosef(11, "Read a packet from %s: %s", sourceName, pkt.String())
				numberOfBytesWrite, err := dst.Write(buffer.buffer[used:buffer.used])
				if err != nil {
					src.Close()
					if isEOF(err) {
						logger.Verbosef(11, "%s connection closed", destName)
						return nil
					} else {
						dst.Close()
						logger.Errorf("error in writing data to %s: %v", destName, err)
						return err
					}
				}
				logger.Verbosef(11, "Written %d bytes of data to %s", numberOfBytesWrite, destName)
				used = buffer.used
			} else if !isEOF(err) {
				logger.Errorf("Failed to read a packet from received buffer: %#v", err)
			}
		}
		buffer.remove(used)
		if buffer.length != 0 {
			logger.Debugf("%d bytes of data remained in the buffer", buffer.length)
		}
	}
}
func packetsProxy(logger helpers.Logger, dir ServiceProxyDirection, src, dst net.Conn) error {
	for {
		packet, err := packets.ReadPacket(src)
		if err != nil {
			dst.Close()
			if isEOF(err) {
				logger.Verbosef(11, "%s connection closed", dir.SourceConnectionName())
				return nil
			} else {
				src.Close()
				logger.Errorf("error in reading packet from %s: %v", dir.SourceConnectionName(), err)
				return err
			}
		}

		err = packet.Write(dst)
		if err != nil {
			src.Close()
			if isEOF(err) {
				logger.Verbosef(11, "%s connection closed", dir.DestinationConnectionName())
				return nil
			} else {
				dst.Close()
				logger.Errorf("error in writing packet to %s: %v", dir.DestinationConnectionName(), err)
				return err
			}
		}
	}
}
func (this ServiceProxyMode) Proxy(logger helpers.Logger, dir ServiceProxyDirection, src, dst net.Conn) error {
	switch this {
	case Raw:
		return rawProxy(logger, dir, src, dst)
	case PacketProxy:
		return packetsProxy(logger, dir, src, dst)
	default:
		return helpers.StringError("Invalid proxy mode")
	}
}
