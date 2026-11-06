package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Per-worker hashrate history for the worker graphs: the credited share
// difficulty of each minute for the last week, kept across restarts in
// <data_dir>/workers-<coin>.json, plus the 10-second buckets of the last
// hour (the "raw" view; not persisted).

const (
	histMinutes   = 7 * 24 * 60 // one week of minutes
	maxHistWorker = 200         // workers kept; the longest idle are dropped
)

// minuteHist is a ring of per-minute credited difficulty sums.
type minuteHist struct {
	vals   [histMinutes]float64
	stamps [histMinutes]int64 // minute epoch (unix/60) each slot holds
	last   int64              // latest minute with a share
}

func (h *minuteHist) add(now time.Time, diff float64) {
	m := now.Unix() / 60
	i := m % histMinutes
	if h.stamps[i] != m {
		h.stamps[i], h.vals[i] = m, 0
	}
	h.vals[i] += diff
	if m > h.last {
		h.last = m
	}
}

// series returns the sums of the minutes endMinute-histMinutes+1 .. endMinute,
// oldest first (0 for minutes without shares).
func (h *minuteHist) series(endMinute int64) []float64 {
	out := make([]float64, histMinutes)
	for k := int64(0); k < histMinutes; k++ {
		m := endMinute - histMinutes + 1 + k
		if m < 0 {
			continue
		}
		i := m % histMinutes
		if h.stamps[i] == m {
			out[k] = h.vals[i]
		}
	}
	return out
}

// WorkerSeries is one worker's history for the graphs. Values are credited
// share difficulty per bucket; hashrate = value × Diff1Hashes / step.
type WorkerSeries struct {
	Name        string    `json:"name"`
	Diff1Hashes float64   `json:"diff1_hashes"`
	Now         int64     `json:"now"` // unix seconds
	RawStep     int64     `json:"raw_step_s"`
	RawEnd      int64     `json:"raw_end"` // unix start of the newest raw bucket
	Raw         []float64 `json:"raw"`     // last hour, oldest first
	MinuteEnd   int64     `json:"minute_end"`
	Minutes     []float64 `json:"minutes"` // last week, oldest first
}

// WorkerHistory returns the graph data of worker, or false if it has none.
func (c *Collector) WorkerHistory(name string) (WorkerSeries, bool) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	h := c.whist[name]
	w := c.workers[name]
	if h == nil && w == nil {
		return WorkerSeries{}, false
	}
	s := WorkerSeries{Name: name, Diff1Hashes: c.diff1Hashes, Now: now.Unix(), RawStep: bucketSecs}
	e := now.Unix() / bucketSecs
	s.RawEnd = e * bucketSecs
	s.Raw = make([]float64, numBuckets)
	if w != nil {
		for k := int64(0); k < numBuckets; k++ {
			b := e - numBuckets + 1 + k
			i := b % numBuckets
			if w.rate.stamps[i] == b {
				s.Raw[k] = w.rate.buckets[i]
			}
		}
	}
	m := now.Unix() / 60
	s.MinuteEnd = m * 60
	if h != nil {
		s.Minutes = h.series(m)
	} else {
		s.Minutes = make([]float64, histMinutes)
	}
	return s, true
}

// addHistLocked records an accepted share in the worker's minute history.
// c.mu must be held.
func (c *Collector) addHistLocked(worker string, now time.Time, diff float64) {
	if c.whist == nil {
		c.whist = map[string]*minuteHist{}
	}
	h := c.whist[worker]
	if h == nil {
		if len(c.whist) >= maxHistWorker {
			c.dropIdlestLocked()
		}
		h = &minuteHist{}
		c.whist[worker] = h
	}
	h.add(now, diff)
}

func (c *Collector) dropIdlestLocked() {
	var name string
	oldest := int64(1 << 62)
	for n, h := range c.whist {
		if h.last < oldest {
			name, oldest = n, h.last
		}
	}
	delete(c.whist, name)
}

// persistedHist is the saved form: per worker, the newest minute and the
// sums of the week before it (oldest first, trailing idle minutes trimmed).
type persistedHist struct {
	End  int64     `json:"end_minute"`
	Vals []float64 `json:"vals"`
}

func (c *Collector) histPath() string {
	if c.persistPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(c.persistPath), "workers-"+c.coin+".json")
}

func (c *Collector) loadHist() {
	p := c.histPath()
	if p == "" {
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var saved map[string]persistedHist
	if json.Unmarshal(b, &saved) != nil {
		return
	}
	c.whist = map[string]*minuteHist{}
	for name, ph := range saved {
		if len(ph.Vals) > histMinutes {
			ph.Vals = ph.Vals[len(ph.Vals)-histMinutes:]
		}
		h := &minuteHist{}
		for k, v := range ph.Vals {
			if v == 0 {
				continue
			}
			m := ph.End - int64(len(ph.Vals)-1-k)
			if m < 0 {
				continue
			}
			i := m % histMinutes
			h.stamps[i], h.vals[i] = m, v
			if m > h.last {
				h.last = m
			}
		}
		c.whist[name] = h
	}
}

func (c *Collector) saveHist() error {
	p := c.histPath()
	if p == "" {
		return nil
	}
	m := time.Now().Unix() / 60
	c.mu.Lock()
	out := make(map[string]persistedHist, len(c.whist))
	names := make([]string, 0, len(c.whist))
	for n := range c.whist {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		h := c.whist[n]
		if m-h.last >= histMinutes {
			continue // idle for a week: nothing left to show
		}
		vals := h.series(h.last)
		first := 0
		for first < len(vals) && vals[first] == 0 {
			first++
		}
		out[n] = persistedHist{End: h.last, Vals: vals[first:]}
	}
	c.mu.Unlock()
	b, err := json.Marshal(out)
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
