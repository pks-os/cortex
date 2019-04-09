package ruler

import (
	"context"
	"fmt"
	"time"

	"github.com/cortexproject/cortex/pkg/ring"
	"github.com/cortexproject/cortex/pkg/util"
	"github.com/go-kit/kit/log/level"
)

const (
	pendingSearchIterations = 10
)

// TransferOut finds an ingester in Active state and terminates upon finding it
func (r *Ruler) TransferOut(ctx context.Context) error {
	_, err := r.findTargetRuler(ctx)
	if err != nil {
		return fmt.Errorf("cannot find replacement ruler: %v", err)
	}

	return nil
}

// findTargetRuler finds an ingester in PENDING state. Before terminating
// the ruler must be sure there is a replacement ruler being created
func (r *Ruler) findTargetRuler(ctx context.Context) (*ring.IngesterDesc, error) {
	level.Debug(util.Logger).Log("msg", "looking for pending rulers")
	findRuler := func() (*ring.IngesterDesc, error) {
		ringDesc, err := r.lifecycler.KVStore.Get(ctx, ring.ConsulKey)
		if err != nil {
			return nil, err
		}

		rulers := ringDesc.(*ring.Desc).FindIngestersByState(ring.PENDING)
		if len(rulers) <= 0 {
			return nil, fmt.Errorf("no pending rulers")
		}

		return &rulers[0], nil
	}

	deadline := time.Now().Add(r.cfg.SearchPendingFor)
	for {
		ingester, err := findRuler()
		if err != nil {
			level.Debug(util.Logger).Log("msg", "Error looking for active rulers", "err", err)
			if time.Now().Before(deadline) {
				time.Sleep(r.cfg.SearchPendingFor / pendingSearchIterations)
				continue
			} else {
				level.Warn(util.Logger).Log("msg", "Could not find pending ruler before deadline", "err", err)
				return nil, err
			}
		}
		return ingester, nil
	}
}

// StopIncomingRequests is called during the shutdown process.
// Ensure no new rules are scheduled on this Ruler
// Currently the api is decoupled from the scheduler, no action
// is required.
func (r *Ruler) StopIncomingRequests() {}

// Flush triggers a flush of all the work items currently
// scheduled by the ruler, currently every ruler will
// query a backend rule store for it's rules so no
// flush is required.
func (r *Ruler) Flush() {
	flushTicker := time.NewTicker(r.cfg.FlushCheckPeriod)
	defer flushTicker.Stop()

	for {
		select {
		case <-flushTicker.C:
			level.Debug(util.Logger).Log("msg", "ruler flush timed out")
			return
		default:
			item := r.scheduler.nextWorkItem()
			if item == nil {
				level.Debug(util.Logger).Log("msg", "flushing complete")
				return
			}
			level.Debug(util.Logger).Log("msg", "flushing item", "item", item)
		}
	}
}
