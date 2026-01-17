package audacity

import (
	"fmt"
	"strings"
)

func (p *PipeClient) doCommand(command string) (string, error) {
	if err := p.sendCommand(command); err != nil {
		return "", err
	}
	response, err := p.getResponse()
	if err != nil {
		return "", err
	}
	if strings.Contains(response, "BatchCommand finished: Failed!") {
		return response, fmt.Errorf("audacity command failed: %s", command)
	}
	return response, nil
}

const (
	cmdSelectAll  = "SelectAll:"
	cmdTrackClose = "TrackClose:"

	cmdPrerecordCompressor = "Compressor: Threshold=-12 NoiseFloor=-40 Ratio=2 AttackTime=3 ReleaseTime=10 Normalize=1 UsePeak=1"
	cmdPrerecordLimiter    = "Limiter: type=SoftLimit gain-L=0 gain-R=0 thresh=-4 hold=6.2 makeup=No"
	cmdLiveNormalize       = "Normalize: PeakLevel=-0.3 ApplyGain=1 RemoveDcOffset=1 StereoIndepend=0"
)

func (p *PipeClient) Process(importPath, exportPath string, live bool) error {
	if err := p.cleanupTracks(); err != nil {
		return err
	}
	if _, err := p.doCommand(fmt.Sprintf("Import2: Filename=%s", importPath)); err != nil {
		return err
	}
	if _, err := p.doCommand(cmdSelectAll); err != nil {
		return err
	}
	if live {
		if err := p.processLive(); err != nil {
			return err
		}
	} else {
		if err := p.processPrerecord(); err != nil {
			return err
		}
	}
	if _, err := p.doCommand(fmt.Sprintf("Export2: Filename=%s NumChannels=2", exportPath)); err != nil {
		return err
	}
	return p.cleanupTracks()
}

func (p *PipeClient) processPrerecord() error {
	if _, err := p.doCommand(cmdPrerecordCompressor); err != nil {
		return err
	}
	if _, err := p.doCommand(cmdPrerecordLimiter); err != nil {
		return err
	}
	return nil
}

func (p *PipeClient) processLive() error {
	_, err := p.doCommand(cmdLiveNormalize)
	return err
}

func (p *PipeClient) cleanupTracks() error {
	if _, err := p.doCommand(cmdSelectAll); err != nil {
		return err
	}
	if _, err := p.doCommand(cmdTrackClose); err != nil {
		return err
	}
	return nil
}
