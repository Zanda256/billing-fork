package app

import (
	"context"
	"fmt"
	riverjobs2 "github.com/doujins-org/doujins-billing/internal/manager/data/river"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type riverClientJobInserter struct {
	runtime *Runtime
}

func (i riverClientJobInserter) Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	if i.runtime == nil || i.runtime.RiverClient == nil {
		return nil, fmt.Errorf("river client is not initialised")
	}
	return i.runtime.RiverClient.Insert(ctx, args, opts)
}

// buildRiverWorkers constructs the worker registry for River.
func (r *Runtime) buildRiverWorkers(ctx context.Context) (*river.Workers, error) {
	workers := river.NewWorkers()
	if err := river.AddWorkerSafely(workers, &riverjobs2.DunningAttemptWorker{DB: r.DB, NMIClients: r.NMIClients}); err != nil {
		return nil, fmt.Errorf("add dunning attempt worker: %w", err)
	}
	sweepWorker := &riverjobs2.DunningSweepWorker{DB: r.DB, Inserter: riverClientJobInserter{runtime: r}}
	if err := river.AddWorkerSafely(workers, sweepWorker); err != nil {
		return nil, fmt.Errorf("add dunning sweep worker: %w", err)
	}
	if err := river.AddWorkerSafely(workers, &riverjobs2.IdempotencyCleanupWorker{DB: r.DB}); err != nil {
		return nil, fmt.Errorf("add idempotency cleanup worker: %w", err)
	}
	if err := river.AddWorkerSafely(workers, &riverjobs2.CCBillReconcileWorker{DB: r.DB, DataLink: r.CCBillDataLink}); err != nil {
		return nil, fmt.Errorf("add ccbill reconcile worker: %w", err)
	}
	if err := river.AddWorkerSafely(workers, &riverjobs2.WebhookProcessWorker{Processor: r.WebhookProcessor}); err != nil {
		return nil, fmt.Errorf("add webhook process worker: %w", err)
	}
	if err := river.AddWorkerSafely(workers, &riverjobs2.WebhookRetryWorker{Events: r.WebhookEventService, Processor: r.WebhookProcessor}); err != nil {
		return nil, fmt.Errorf("add webhook retry worker: %w", err)
	}
	return workers, nil
}

// buildRiverPeriodicJobs defines recurring schedules for workers using River periodic jobs.
func (r *Runtime) buildRiverPeriodicJobs(ctx context.Context) ([]*river.PeriodicJob, error) {
	var jobs []*river.PeriodicJob

	// Every minute: run DunningSweep
	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(time.Minute),
		func() (river.JobArgs, *river.InsertOpts) {
			return riverjobs2.DunningSweepArgs{}, &river.InsertOpts{
				Queue: riverjobs2.QueueBilling,
			}
		},
		&river.PeriodicJobOpts{RunOnStart: false},
	))

	// Daily: Idempotency cleanup
	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return riverjobs2.IdempotencyCleanupArgs{}, &river.InsertOpts{
				Queue: riverjobs2.QueueBilling,
			}
		},
		&river.PeriodicJobOpts{RunOnStart: true},
	))

	// Every 6 hours: CCBill reconcile
	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(6*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return riverjobs2.CCBillReconcileArgs{}, &river.InsertOpts{
				Queue: riverjobs2.QueueBilling,
			}
		},
		&river.PeriodicJobOpts{RunOnStart: false},
	))

	// Every minute: check for webhook retries
	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(time.Minute),
		func() (river.JobArgs, *river.InsertOpts) {
			return riverjobs2.WebhookRetryArgs{}, &river.InsertOpts{
				Queue: riverjobs2.QueueBilling,
			}
		},
		&river.PeriodicJobOpts{RunOnStart: true},
	))

	return jobs, nil
}
