//go:build linux

package cli

import (
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestQuitKeyTerminalModeReadsWithoutEnterAndPreservesSignals(t *testing.T) {
	original := &unix.Termios{
		Lflag: unix.ICANON | unix.ECHO | unix.ECHONL | unix.ISIG | unix.IEXTEN,
	}
	mode := quitKeyTerminalMode(original)

	if mode.Lflag&(unix.ICANON|unix.ECHO|unix.ECHONL) != 0 {
		t.Fatalf("terminal local flags = %#x, canonical input or echo still enabled", mode.Lflag)
	}
	if mode.Lflag&unix.ISIG == 0 {
		t.Fatal("terminal signal generation was disabled")
	}
	if mode.Lflag&unix.IEXTEN == 0 {
		t.Fatal("unrelated terminal flag was changed")
	}
	if mode.Cc[unix.VMIN] != 1 || mode.Cc[unix.VTIME] != 0 {
		t.Fatalf("VMIN=%d VTIME=%d, want VMIN=1 VTIME=0", mode.Cc[unix.VMIN], mode.Cc[unix.VTIME])
	}
	if original.Lflag&unix.ICANON == 0 {
		t.Fatal("original terminal settings were modified")
	}
}

func TestTerminalQuitListenerRecognizesQWithoutNewline(t *testing.T) {
	terminalPipe := makeUnixPipe(t)
	wakePipe := makeUnixPipe(t)
	listener := &linuxTerminalQuitListener{
		terminalFD: terminalPipe[0],
		wakeRead:   wakePipe[0],
		wakeWrite:  wakePipe[1],
		quit:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	go listener.run()

	if _, err := unix.Write(terminalPipe[1], []byte{'q'}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-listener.Quit():
	case <-time.After(time.Second):
		t.Fatal("quit was not detected without a newline")
	}
	<-listener.done
}

func makeUnixPipe(t *testing.T) [2]int {
	t.Helper()
	fds := make([]int, 2)
	if err := unix.Pipe2(fds, unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = unix.Close(fds[0])
		_ = unix.Close(fds[1])
	})
	return [2]int{fds[0], fds[1]}
}
