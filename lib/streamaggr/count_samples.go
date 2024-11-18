package streamaggr

import (
	"sync"

	"github.com/VictoriaMetrics/VictoriaMetrics/lib/bytesutil"
)

// countSamplesAggrState calculates output=count_samples, e.g. the count of input samples.
type countSamplesAggrState struct {
	m sync.Map
}

type countSamplesStateValue struct {
	mu             sync.Mutex
	n              uint64
	deleted        bool
	deleteDeadline int64
}

func newCountSamplesAggrState() *countSamplesAggrState {
	return &countSamplesAggrState{}
}

func (as *countSamplesAggrState) pushSamples(samples []pushSample, deleteDeadline int64, includeInputKey bool) {
	for i := range samples {
		s := &samples[i]
		outputKey := getOutputKey(s.key, includeInputKey)

	again:
		v, ok := as.m.Load(outputKey)
		if !ok {
			// The entry is missing in the map. Try creating it.
			v = &countSamplesStateValue{}
			outputKey = bytesutil.InternString(outputKey)
			vNew, loaded := as.m.LoadOrStore(outputKey, v)
			if loaded {
				// Use the entry created by a concurrent goroutine.
				v = vNew
			}
		}
		sv := v.(*countSamplesStateValue)
		sv.mu.Lock()
		deleted := sv.deleted
		if !deleted {
			sv.n++
			sv.deleteDeadline = deleteDeadline
		}
		sv.mu.Unlock()
		if deleted {
			// The entry has been deleted by the concurrent call to flushState
			// Try obtaining and updating the entry again.
			goto again
		}
	}
}

func (as *countSamplesAggrState) flushState(ctx *flushCtx) {
	m := &as.m
	m.Range(func(k, v any) bool {
		// Atomically delete the entry from the map, so new entry is created for the next flush.
		m.Delete(k)

		sv := v.(*countSamplesStateValue)
		sv.mu.Lock()
		if ctx.flushTimestamp > sv.deleteDeadline {
			sv.deleted = true
			sv.mu.Unlock()
			key := k.(string)
			ctx.a.lc.Delete(bytesutil.ToUnsafeBytes(key), ctx.flushTimestamp)
			m.Delete(k)
			return true
		}
		n := sv.n
		if n == 0 {
			sv.mu.Unlock()
			return true
		}

		sv.n = 0
		sv.mu.Unlock()

		key := k.(string)
		ctx.appendSeries(key, "count_samples", float64(n))
		return true
	})
}
