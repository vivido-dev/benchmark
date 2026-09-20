//go:build !windows

package main

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

type unixTerm struct {
	file        *os.File
	origTermios unix.Termios
	restore     bool
}

func openControllingTerm() (terminal, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		f = os.Stdin
	}

	fd := int(f.Fd())
	t := &unixTerm{file: f}

	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err == nil {
		t.origTermios = *termios
		t.restore = true

		raw := *termios
		// cfmakeraw equivalent
		raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
		raw.Oflag &^= unix.OPOST
		raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
		raw.Cflag &^= unix.CSIZE | unix.PARENB
		raw.Cflag |= unix.CS8
		raw.Cc[unix.VMIN] = 1
		raw.Cc[unix.VTIME] = 0

		_ = unix.IoctlSetTermios(fd, unix.TCSETS, &raw)
	}

	return t, nil
}

func (t *unixTerm) RestoreAndClose() {
	if t.restore {
		_ = unix.IoctlSetTermios(int(t.file.Fd()), unix.TCSETS, &t.origTermios)
		t.restore = false
	}
	if t.file != os.Stdin && t.file != nil {
		_ = t.file.Close()
	}
}

func (t *unixTerm) WriteAllString(data string) error {
	b := []byte(data)
	for len(b) > 0 {
		n, err := t.file.Write(b)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return err
		}
		b = b[n:]
	}
	return nil
}

func (t *unixTerm) ReadWithTimeout(buf []byte, timeout time.Duration) (int, error) {
	fd := int(t.file.Fd())
	pfd := []unix.PollFd{
		{Fd: int32(fd), Events: unix.POLLIN},
	}
	ms := int(timeout.Milliseconds())
	if ms == 0 && timeout > 0 {
		ms = 1
	}
	for {
		n, err := unix.Poll(pfd, ms)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return 0, err
		}
		if n == 0 {
			return 0, os.ErrDeadlineExceeded
		}
		break
	}
	for {
		n, err := t.file.Read(buf)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return 0, err
		}
		return n, nil
	}
}
