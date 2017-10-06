package main

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"time"
)

const (
	DIM    Format = 2
	RED    Format = 31
	GREEN  Format = 32
	YELLOW Format = 33
)

type MessageWriter struct {
	conn net.Conn
}

type Logger struct {
	writer               io.Writer
	prefix               func() string
	suffix               func() string
	written              bool
	lastEndedWithNewline bool
	lock                 sync.Mutex
}

type NilLogger struct{}

type Format byte

func (*NilLogger) Write(bs []byte) (int, error) {
	return len(bs), nil
}

func (w *MessageWriter) Write(p []byte) (n int, err error) {
	err = Send(w.conn, Message{Log, string(p)})
	n = len(p)
	return
}

func NewMessageLogger(conn net.Conn) io.Writer {
	return &MessageWriter{conn}
}

func NewLogger(writer io.Writer, prefix string) io.Writer {
	return &Logger{
		writer: writer,
		prefix: func() string {
			return prefix
		},
		suffix: func() string {
			return ""
		},
	}
}

func NewFormatter(writer io.Writer, format Format) io.Writer {
	return &Logger{
		writer: writer,
		prefix: func() string {
			return fmt.Sprint("\033[", format, "m")
		},
		suffix: func() string {
			return fmt.Sprint("\033[0m")
		},
	}
}

func NewTimeLogger(writer io.Writer) io.Writer {
	start := time.Now()
	return &Logger{
		writer: writer,
		prefix: func() string {
			now := time.Now()
			seconds := int(math.Ceil(now.Sub(start).Seconds()))
			minutes := seconds / 60
			seconds -= minutes * 60
			return fmt.Sprintf("%d:%02d ", minutes, seconds)
		},
		suffix: func() string {
			return ""
		},
	}
}

func (l *Logger) Write(bs []byte) (int, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	var (
		prefix = l.prefix()
		suffix = l.suffix()
		final  = bs
	)

	if !l.written || l.lastEndedWithNewline {
		l.written = true
		final = append([]byte(prefix), final...)
	}
	if bytes.HasSuffix(final, []byte{byte('\n')}) {
		final = final[:len(final)-1]
		l.lastEndedWithNewline = true
	} else {
		l.lastEndedWithNewline = false
	}
	final = bytes.Replace(final, []byte("\r\n"), []byte("\n"), -1)
	final = bytes.Replace(final, []byte("\n"), []byte(suffix+"\n"+prefix), -1)
	if l.lastEndedWithNewline {
		final = append(final, []byte(suffix)...)
		final = append(final, '\n')
	}
	n, err := l.writer.Write(final)
	if n > len(bs) {
		n = len(bs)
	}
	return n, err
}
