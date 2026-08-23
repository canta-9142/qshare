//go:build linux

package cli

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

type linuxTerminalQuitListener struct {
	terminalFD int
	original   *unix.Termios
	wakeRead   int
	wakeWrite  int
	quit       chan struct{}
	done       chan struct{}
	closeOnce  sync.Once
	closeErr   error
}

func startTerminalQuitListener(terminal *os.File) (terminalQuitListener, error) {
	terminalFD := int(terminal.Fd())
	original, err := unix.IoctlGetTermios(terminalFD, unix.TCGETS)
	if err != nil {
		return nil, fmt.Errorf("read terminal settings: %w", err)
	}

	wake := make([]int, 2)
	if err := unix.Pipe2(wake, unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		return nil, fmt.Errorf("create terminal listener wake pipe: %w", err)
	}

	mode := quitKeyTerminalMode(original)
	if err := unix.IoctlSetTermios(terminalFD, unix.TCSETS, mode); err != nil {
		_ = unix.Close(wake[0])
		_ = unix.Close(wake[1])
		return nil, fmt.Errorf("configure terminal settings: %w", err)
	}

	listener := &linuxTerminalQuitListener{
		terminalFD: terminalFD,
		original:   original,
		wakeRead:   wake[0],
		wakeWrite:  wake[1],
		quit:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	go listener.run()
	return listener, nil
}

func quitKeyTerminalMode(original *unix.Termios) *unix.Termios {
	mode := *original
	mode.Lflag &^= unix.ICANON | unix.ECHO | unix.ECHONL
	mode.Cc[unix.VMIN] = 1
	mode.Cc[unix.VTIME] = 0
	return &mode
}

func (l *linuxTerminalQuitListener) Quit() <-chan struct{} {
	return l.quit
}

func (l *linuxTerminalQuitListener) Close() error {
	l.closeOnce.Do(func() {
		_, wakeErr := unix.Write(l.wakeWrite, []byte{1})
		<-l.done
		restoreErr := unix.IoctlSetTermios(l.terminalFD, unix.TCSETS, l.original)
		readCloseErr := unix.Close(l.wakeRead)
		writeCloseErr := unix.Close(l.wakeWrite)
		l.closeErr = errors.Join(
			wrapTerminalCloseError("wake terminal listener", wakeErr),
			wrapTerminalCloseError("restore terminal settings", restoreErr),
			wrapTerminalCloseError("close terminal listener read pipe", readCloseErr),
			wrapTerminalCloseError("close terminal listener write pipe", writeCloseErr),
		)
	})
	return l.closeErr
}

func (l *linuxTerminalQuitListener) run() {
	defer close(l.done)

	pollFDs := []unix.PollFd{
		{Fd: int32(l.terminalFD), Events: unix.POLLIN},
		{Fd: int32(l.wakeRead), Events: unix.POLLIN},
	}
	var input [1]byte
	for {
		_, err := unix.Poll(pollFDs, -1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil || pollFDs[1].Revents != 0 {
			return
		}
		if pollFDs[0].Revents == 0 {
			continue
		}

		read, err := unix.Read(l.terminalFD, input[:])
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
			continue
		}
		if err != nil || read == 0 {
			return
		}
		if input[0] == 'q' {
			close(l.quit)
			return
		}
	}
}

func wrapTerminalCloseError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}
