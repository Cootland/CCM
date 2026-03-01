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
	return s.ssh.RunCommand(ctx, targetID, cmd, 30*time.Second)
}

func parseContainerRef(id string) (string, string, error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid container id; expected target:container")
	}
	return parts[0], parts[1], nil
}
