package main

import (
	"io"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WSConnection struct {
	*websocket.Conn
	reader    io.Reader
	readLock  sync.Mutex
	writeLock sync.Mutex
}

func NewWSConnection(conn *websocket.Conn) *WSConnection {
	return &WSConnection{Conn: conn}
}

func (this *WSConnection) SetDeadline(t time.Time) error {
	if err := this.SetReadDeadline(t); err != nil {
		return err
	}

	return this.SetWriteDeadline(t)
}
func (this *WSConnection) Write(buffer []byte) (int, error) {
	this.writeLock.Lock()
	defer this.writeLock.Unlock()

	if err := this.WriteMessage(websocket.BinaryMessage, buffer); err != nil {
		return 0, err
	}

	return len(buffer), nil
}
func (this *WSConnection) Read(buffer []byte) (int, error) {
	this.readLock.Lock()
	defer this.readLock.Unlock()

	for {
		if this.reader == nil {
			// Advance to next message.
			var err error
			_, this.reader, err = this.NextReader()
			if err != nil {
				return 0, err
			}
		}

		n, err := this.reader.Read(buffer)
		if err == io.EOF {
			// this message is finished
			this.reader = nil
			if n > 0 {
				return n, nil
			}

			// no data read from this reader, advance to next message
			continue
		}
		return n, err
	}
}
func (this *WSConnection) Close() error { return this.Conn.Close() }
