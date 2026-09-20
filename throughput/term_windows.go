//go:build windows

package main

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

type winTerm struct {
	inHandle    windows.Handle
	outHandle   windows.Handle
	origInMode  uint32
	origOutMode uint32
	restoreIn   bool
	restoreOut  bool
}

func openControllingTerm() (terminal, error) {
	inName, _ := windows.UTF16PtrFromString("CONIN$")
	hIn, errIn := windows.CreateFile(inName, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if errIn != nil {
		hIn = windows.Handle(os.Stdin.Fd())
	}

	outName, _ := windows.UTF16PtrFromString("CONOUT$")
	hOut, errOut := windows.CreateFile(outName, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if errOut != nil {
		hOut = windows.Handle(os.Stdout.Fd())
	}

	t := &winTerm{inHandle: hIn, outHandle: hOut}

	var inMode, outMode uint32
	if err := windows.GetConsoleMode(hIn, &inMode); err == nil {
		t.origInMode = inMode
		t.restoreIn = true
		rawIn := (inMode &^ (windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_WINDOW_INPUT | windows.ENABLE_MOUSE_INPUT)) | windows.ENABLE_VIRTUAL_TERMINAL_INPUT
		_ = windows.SetConsoleMode(hIn, rawIn)
	}

	if err := windows.GetConsoleMode(hOut, &outMode); err == nil {
		t.origOutMode = outMode
		t.restoreOut = true
		rawOut := outMode | windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
		_ = windows.SetConsoleMode(hOut, rawOut)
	}

	return t, nil
}

func (t *winTerm) RestoreAndClose() {
	if t.restoreIn {
		_ = windows.SetConsoleMode(t.inHandle, t.origInMode)
		t.restoreIn = false
	}
	if t.restoreOut {
		_ = windows.SetConsoleMode(t.outHandle, t.origOutMode)
		t.restoreOut = false
	}
	if t.inHandle != windows.Handle(os.Stdin.Fd()) && t.inHandle != 0 && t.inHandle != windows.InvalidHandle {
		_ = windows.CloseHandle(t.inHandle)
		t.inHandle = 0
	}
	if t.outHandle != windows.Handle(os.Stdout.Fd()) && t.outHandle != 0 && t.outHandle != windows.InvalidHandle {
		_ = windows.CloseHandle(t.outHandle)
		t.outHandle = 0
	}
}

func (t *winTerm) WriteAllString(data string) error {
	b := []byte(data)
	for len(b) > 0 {
		chunkLen := len(b)
		if chunkLen > 64*1024 {
			chunkLen = 64 * 1024
		}
		var written uint32
		err := windows.WriteFile(t.outHandle, b[:chunkLen], &written, nil)
		if err != nil {
			return err
		}
		if written == 0 {
			break
		}
		b = b[written:]
	}
	return nil
}

func (t *winTerm) ReadWithTimeout(buf []byte, timeout time.Duration) (int, error) {
	ms := uint32(timeout.Milliseconds())
	if ms == 0 && timeout > 0 {
		ms = 1
	}
	event, err := windows.WaitForSingleObject(t.inHandle, ms)
	if err != nil {
		return 0, err
	}
	if event == uint32(windows.WAIT_TIMEOUT) {
		return 0, os.ErrDeadlineExceeded
	}
	if event != uint32(windows.WAIT_OBJECT_0) {
		return 0, fmt.Errorf("wait on console input failed: event %d", event)
	}
	var read uint32
	err = windows.ReadFile(t.inHandle, buf, &read, nil)
	if err != nil {
		return 0, err
	}
	return int(read), nil
}
