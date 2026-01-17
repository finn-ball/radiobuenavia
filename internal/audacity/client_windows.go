//go:build windows

package audacity

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

type PipeClient struct {
	writeHandle windows.Handle
	readHandle  windows.Handle
}

func NewPipeClient() (*PipeClient, error) {
	writeHandle, err := openPipe(`\\.\pipe\ToSrvPipe`)
	if err != nil {
		return nil, fmt.Errorf("audacity pipe not found (\\\\.\\pipe\\ToSrvPipe); open Audacity and enable \"mod-script-pipe\" in Preferences > Modules")
	}
	readHandle, err := openPipe(`\\.\pipe\FromSrvPipe`)
	if err != nil {
		windows.CloseHandle(writeHandle)
		return nil, fmt.Errorf("audacity pipe not found (\\\\.\\pipe\\FromSrvPipe); open Audacity and enable \"mod-script-pipe\" in Preferences > Modules")
	}
	if err := setPipeMessageMode(readHandle); err != nil {
		windows.CloseHandle(writeHandle)
		windows.CloseHandle(readHandle)
		return nil, err
	}
	return &PipeClient{writeHandle: writeHandle, readHandle: readHandle}, nil
}

func openPipe(path string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	handle, err := windows.CreateFile(
		p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return 0, err
	}
	return handle, nil
}

func setPipeMessageMode(handle windows.Handle) error {
	mode := uint32(windows.PIPE_READMODE_MESSAGE)
	return windows.SetNamedPipeHandleState(handle, &mode, nil, nil)
}

func (p *PipeClient) Close() error {
	if p.writeHandle != 0 {
		windows.CloseHandle(p.writeHandle)
	}
	if p.readHandle != 0 {
		windows.CloseHandle(p.readHandle)
	}
	return nil
}

func (p *PipeClient) sendCommand(command string) error {
	if p.writeHandle == 0 {
		return errors.New("audacity write pipe is not open")
	}
	payload := []byte(command + "\n")
	var written uint32
	return windows.WriteFile(p.writeHandle, payload, &written, nil)
}

func (p *PipeClient) getResponse() (string, error) {
	if p.readHandle == 0 {
		return "", errors.New("audacity read pipe is not open")
	}
	var builder strings.Builder
	buf := make([]byte, 64*1024)
	for {
		var read uint32
		err := windows.ReadFile(p.readHandle, buf, &read, nil)
		if err != nil {
			return builder.String(), err
		}
		if read == 0 {
			break
		}
		builder.Write(buf[:read])
		if strings.Contains(builder.String(), "BatchCommand finished") {
			break
		}
	}
	return builder.String(), nil
}
