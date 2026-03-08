package script

import (
	"context"
	"fmt"
	"log"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/loganjanssen/ccm/internal/config"
	"github.com/loganjanssen/ccm/internal/cronexpr"
	"github.com/loganjanssen/ccm/internal/sshx"
)

type Service struct {
	cfg         *config.Config
	ssh         *sshx.Manager
	assignments []assignment

	mu    sync.Mutex
	state map[string]string

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type assignment struct {
	key        string
	stackID    string
	targetID   string
	deployPath string
	name       string
	cron       string
	spec       cronexpr.Spec
	timezone   string
	location   *time.Location
	file       string
}

func NewService(cfg *config.Config, ssh *sshx.Manager) (*Service, error) {
	assignments, err := buildAssignments(cfg)
	if err != nil {
		return nil, err
	}
	return &Service{
		cfg:         cfg,
		ssh:         ssh,
		assignments: assignments,
		state:       map[string]string{},
	}, nil
}

func (s *Service) Start(ctx context.Context) {
	if len(s.assignments) == 0 {
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.wg.Add(1)
	go s.loop(runCtx)
}

func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

func (s *Service) loop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	s.evaluate(ctx, time.Now())

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.evaluate(ctx, now)
		}
	}
}

func (s *Service) evaluate(ctx context.Context, now time.Time) {
	for _, a := range s.assignments {
		localNow := now.In(a.location)
		minuteKey := localNow.Format("2006-01-02T15:04")
		if !a.spec.Match(localNow) {
			continue
		}
		if !s.markIfNewMinute(a.key, minuteKey) {
			continue
		}
		s.runAssignment(ctx, a)
	}
}

func (s *Service) markIfNewMinute(key, minuteKey string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state[key] == minuteKey {
		return false
	}
	s.state[key] = minuteKey
	return true
}

func (s *Service) runAssignment(ctx context.Context, a assignment) {
	cmd := fmt.Sprintf("cd %q && /bin/sh %q", a.deployPath, path.Join("ccm_scripts", a.file))
	res, err := s.ssh.RunCommand(ctx, a.targetID, cmd, 30*time.Minute)
	if err != nil {
		log.Printf("script scheduler: %s failed on %s: %v", a.key, a.targetID, err)
		return
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = fmt.Sprintf("exit %d", res.ExitCode)
		}
		log.Printf("script scheduler: %s failed on %s: %s", a.key, a.targetID, msg)
		return
	}
	log.Printf("script scheduler: %s executed on %s", a.key, a.targetID)
}

func buildAssignments(cfg *config.Config) ([]assignment, error) {
	stackIDs := make([]string, 0, len(cfg.Stacks))
	for id := range cfg.Stacks {
		stackIDs = append(stackIDs, id)
	}
	sort.Strings(stackIDs)

	assignments := make([]assignment, 0)
	for _, stackID := range stackIDs {
		stack := cfg.Stacks[stackID]
		if stack == nil || len(stack.Scripts) == 0 {
			continue
		}
		deployPath := path.Join(stack.Target.DeployRoot, stack.DeploySubdir)
		for _, script := range stack.Scripts {
			spec, err := cronexpr.Parse(script.Cron)
			if err != nil {
				return nil, fmt.Errorf("parse stack %q script %q cron: %w", stackID, script.Name, err)
			}
			loc := time.Local
			tz := strings.TrimSpace(script.Timezone)
			if tz == "" {
				tz = "Local"
			} else {
				loaded, err := time.LoadLocation(tz)
				if err != nil {
					return nil, fmt.Errorf("load stack %q script %q timezone: %w", stackID, script.Name, err)
				}
				loc = loaded
			}
			assignments = append(assignments, assignment{
				key:        fmt.Sprintf("%s:%s", stackID, script.Name),
				stackID:    stackID,
				targetID:   stack.TargetID,
				deployPath: deployPath,
				name:       script.Name,
				cron:       script.Cron,
				spec:       spec,
				timezone:   tz,
				location:   loc,
				file:       script.File,
			})
		}
	}
	return assignments, nil
}
