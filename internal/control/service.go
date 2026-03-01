package control

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/loganjanssen/ccm/internal/config"
	"github.com/loganjanssen/ccm/internal/model"
	"github.com/loganjanssen/ccm/internal/sshx"
)

type Service struct {
	cfg *config.Config
	ssh *sshx.Manager
}

func NewService(cfg *config.Config, ssh *sshx.Manager) *Service {
	return &Service{cfg: cfg, ssh: ssh}
}

func (s *Service) Start(ctx context.Context, id string) (model.CommandResult, error) {
	return s.containerCmd(ctx, id, "start")
}

func (s *Service) Stop(ctx context.Context, id string) (model.CommandResult, error) {
	return s.containerCmd(ctx, id, "stop")
}

func (s *Service) Restart(ctx context.Context, id string) (model.CommandResult, error) {
	return s.containerCmd(ctx, id, "restart")
}

func (s *Service) containerCmd(ctx context.Context, id, op string) (model.CommandResult, error) {
	targetID, containerID, err := parseContainerRef(id)
	if err != nil {
		return model.CommandResult{}, err
	}
	if _, ok := s.cfg.Targets[targetID]; !ok {
		return model.CommandResult{}, fmt.Errorf("unknown target %q", targetID)
	}
	cmd := fmt.Sprintf("docker %s %s", op, containerID)
	res, err := s.ssh.RunCommand(ctx, targetID, cmd, 30*time.Second)
	if err == nil {
		return res, nil
	}

	// Some SSH servers can drop command exit status even when docker action completed.
	// Verify container state before reporting failure to the UI.
	expectedRunning := op != "stop"
	if s.verifyRunningState(ctx, targetID, containerID, expectedRunning) {
		reason := "command exit status unavailable; container state verified"
		if strings.TrimSpace(res.Stderr) == "" {
			res.Stderr = reason
		} else {
			res.Stderr = strings.TrimSpace(res.Stderr) + "; " + reason
		}
		res.ExitCode = 0
		return res, nil
	}
	return res, err
}

func (s *Service) verifyRunningState(ctx context.Context, targetID, containerID string, wantRunning bool) bool {
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		res, err := s.ssh.RunCommand(ctx, targetID, fmt.Sprintf("docker inspect -f '{{.State.Running}}' %s", containerID), 5*time.Second)
		if err == nil && res.ExitCode == 0 {
			running := strings.EqualFold(strings.TrimSpace(res.Stdout), "true")
			if running == wantRunning {
				return true
			}
		}
		time.Sleep(700 * time.Millisecond)
	}
	return false
}

func parseContainerRef(id string) (string, string, error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid container id; expected target:container")
	}
	return parts[0], parts[1], nil
}
