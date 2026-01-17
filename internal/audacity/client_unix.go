//go:build !windows

package audacity

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type PipeClient struct {
	writePipe *os.File
	readPath  string
}

func NewPipeClient() (*PipeClient, error) {
	uid := os.Getuid()
	writePath := filepath.Join(os.TempDir(), fmt.Sprintf("audacity_script_pipe.to.%d", uid))
	readPath := filepath.Join(os.TempDir(), fmt.Sprintf("audacity_script_pipe.from.%d", uid))

	if _, err := os.Stat(writePath); err != nil {
		return nil, fmt.Errorf("audacity pipe not found (%s); enable Audacity scripting pipes and start Audacity", writePath)
	}
	fd, err := syscall.Open(writePath, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, syscall.ENXIO) {
			return nil, fmt.Errorf("audacity pipe not ready (%s)\nopen Audacity and enable \"mod-script-pipe\" in Preferences > Modules", writePath)
		}
		return nil, err
	}
	writePipe := os.NewFile(uintptr(fd), writePath)

	return &PipeClient{
		writePipe: writePipe,
		readPath:  readPath,
	}, nil
}

func (p *PipeClient) Close() error {
	if p.writePipe == nil {
		return nil
	}
	return p.writePipe.Close()
}

func (p *PipeClient) sendCommand(command string) error {
	if p.writePipe == nil {
		return errors.New("audacity write pipe is not open")
	}
	_, err := p.writePipe.WriteString(command + "\n")
	if err != nil {
		return err
	}
	return nil
}

func (p *PipeClient) getResponse() (string, error) {
	file, err := os.Open(p.readPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var builder strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, os.ErrClosed) {
			if errors.Is(err, io.EOF) {
				builder.WriteString(line)
				break
			}
			return builder.String(), err
		}
		builder.WriteString(line)
		if strings.Contains(builder.String(), "BatchCommand finished") {
			break
		}
		if line == "\n" || len(line) == 0 {
			break
		}
	}
	return builder.String(), nil
}
