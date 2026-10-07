package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/robfig/cron/v3"
)

const iqTestDefaultMaxWorkers = 5

// IQTestRunnerService periodically scans due IQ test plans and executes them.
type IQTestRunnerService struct {
	planRepo IQTestPlanRepository
	iqSvc    *IQTestService
	cfg      *config.Config

	cron      *cron.Cron
	startOnce sync.Once
	stopOnce  sync.Once
}

// NewIQTestRunnerService creates a new runner.
func NewIQTestRunnerService(
	planRepo IQTestPlanRepository,
	iqSvc *IQTestService,
	cfg *config.Config,
) *IQTestRunnerService {
	return &IQTestRunnerService{
		planRepo: planRepo,
		iqSvc:    iqSvc,
		cfg:      cfg,
	}
}

// Start begins the cron ticker (every minute).
func (s *IQTestRunnerService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		loc := time.Local
		if s.cfg != nil {
			if parsed, err := time.LoadLocation(s.cfg.Timezone); err == nil && parsed != nil {
				loc = parsed
			}
		}

		c := cron.New(cron.WithParser(scheduledTestCronParser), cron.WithLocation(loc))
		if _, err := c.AddFunc("* * * * *", func() { s.runScheduled() }); err != nil {
			logger.LegacyPrintf("service.iq_test_runner", "[IQTestRunner] not started (invalid schedule): %v", err)
			return
		}
		s.cron = c
		s.cron.Start()
		logger.LegacyPrintf("service.iq_test_runner", "[IQTestRunner] started (tick=every minute)")
	})
}

// Stop gracefully shuts down the cron scheduler.
func (s *IQTestRunnerService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.cron != nil {
			ctx := s.cron.Stop()
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
				logger.LegacyPrintf("service.iq_test_runner", "[IQTestRunner] cron stop timed out")
			}
		}
	})
}

func (s *IQTestRunnerService) runScheduled() {
	defer func() {
		if r := recover(); r != nil {
			logger.LegacyPrintf("service.iq_test_runner", "[IQTestRunner] recovered from panic: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	plans, err := s.planRepo.ListDue(ctx, time.Now())
	if err != nil {
		logger.LegacyPrintf("service.iq_test_runner", "[IQTestRunner] ListDue error: %v", err)
		return
	}
	if len(plans) == 0 {
		return
	}

	sem := make(chan struct{}, iqTestDefaultMaxWorkers)
	var wg sync.WaitGroup
	for _, plan := range plans {
		if !plan.Enabled {
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(p *IQTestPlan) {
			defer wg.Done()
			defer func() { <-sem }()
			s.runOnePlan(ctx, p)
		}(plan)
	}
	wg.Wait()
}

func (s *IQTestRunnerService) runOnePlan(ctx context.Context, plan *IQTestPlan) {
	if _, err := s.iqSvc.RunQuestion(ctx, plan.BankID, plan.QuestionID, plan.AccountID, plan.ModelID, IQTestTriggerScheduled, plan.ID, plan.ReasoningEffort); err != nil {
		logger.LegacyPrintf("service.iq_test_runner", "[IQTestRunner] plan=%d RunQuestion error: %v", plan.ID, err)
	}

	nextRun, err := computeNextRun(plan.CronExpression, time.Now())
	if err != nil {
		logger.LegacyPrintf("service.iq_test_runner", "[IQTestRunner] plan=%d computeNextRun error: %v", plan.ID, err)
		return
	}
	if err := s.planRepo.UpdateAfterRun(ctx, plan.ID, time.Now(), nextRun); err != nil {
		logger.LegacyPrintf("service.iq_test_runner", "[IQTestRunner] plan=%d UpdateAfterRun error: %v", plan.ID, err)
	}
}
