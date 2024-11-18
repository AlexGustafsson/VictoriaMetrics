package streamaggr

import (
	"strconv"
	"sync"

	"github.com/VictoriaMetrics/VictoriaMetrics/lib/bytesutil"
	"github.com/valyala/histogram"
)

// quantilesAggrState calculates output=quantiles, e.g. the given quantiles over the input samples.
type quantilesAggrState struct {
	m sync.Map

	phis []float64
}

type quantilesStateValue struct {
	mu             sync.Mutex
	h              *histogram.Fast
	deleted        bool
	deleteDeadline int64
}

func newQuantilesAggrState(phis []float64) *quantilesAggrState {
	return &quantilesAggrState{
		phis: phis,
	}
}

func (as *quantilesAggrState) pushSamples(samples []pushSample, deleteDeadline int64, includeInputKey bool) {
	for i := range samples {
		s := &samples[i]
		outputKey := getOutputKey(s.key, includeInputKey)

	again:
		v, ok := as.m.Load(outputKey)
		if !ok {
			// The entry is missing in the map. Try creating it.
			h := histogram.GetFast()
			v = &quantilesStateValue{
				h: h,
			}
			outputKey = bytesutil.InternString(outputKey)
			vNew, loaded := as.m.LoadOrStore(outputKey, v)
			if loaded {
				// Use the entry created by a concurrent goroutine.
				histogram.PutFast(h)
				v = vNew
			}
		}
		sv := v.(*quantilesStateValue)
		sv.mu.Lock()
		deleted := sv.deleted
		if !deleted {
			sv.h.Update(s.value)
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

func (as *quantilesAggrState) flushState(ctx *flushCtx) {
	m := &as.m
	phis := as.phis
	var quantiles []float64
	var b []byte
	m.Range(func(k, v any) bool {
		// Atomically delete the entry from the map, so new entry is created for the next flush.
		m.Delete(k)

		sv := v.(*quantilesStateValue)
		sv.mu.Lock()
		if ctx.flushTimestamp > sv.deleteDeadline {
			sv.deleted = true
			sv.mu.Unlock()
			key := k.(string)
			ctx.a.lc.Delete(bytesutil.ToUnsafeBytes(key), ctx.flushTimestamp)
			m.Delete(k)
			return true
		}
		quantiles = sv.h.Quantiles(quantiles[:0], phis)
		sv.h.Reset()
		sv.mu.Unlock()

		key := k.(string)
		for i, quantile := range quantiles {
			b = strconv.AppendFloat(b[:0], phis[i], 'g', -1, 64)
			phiStr := bytesutil.InternBytes(b)
			ctx.appendSeriesWithExtraLabel(key, "quantiles", quantile, "quantile", phiStr)
		}
		return true
	})
}
