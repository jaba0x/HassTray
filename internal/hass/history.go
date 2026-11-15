package hass

import (
	"encoding/json"
	"math"
	"strconv"
	"time"
)

// Point is one reading on a graph.
type Point struct {
	T time.Time
	V float64
}

// ParseHistory reads the body of /api/history/period for a single entity.
// Readings that aren't numbers (unknown, unavailable) are left out.
func ParseHistory(raw []byte) ([]Point, error) {
	var series [][]struct {
		State       string    `json:"state"`
		LastChanged time.Time `json:"last_changed"`
		LastUpdated time.Time `json:"last_updated"`
	}
	if err := json.Unmarshal(raw, &series); err != nil {
		return nil, err
	}
	if len(series) == 0 {
		return nil, nil
	}
	pts := make([]Point, 0, len(series[0]))
	for _, r := range series[0] {
		v, err := strconv.ParseFloat(r.State, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		t := r.LastChanged
		if t.IsZero() {
			t = r.LastUpdated
		}
		pts = append(pts, Point{T: t, V: v})
	}
	return pts, nil
}

// Downsample squeezes readings into n evenly spaced values between from and
// to. Each value is the average of the readings in its slice of time; a slice
// with no readings repeats the last known value, because that is what a
// sensor that hasn't reported is still showing.
func Downsample(pts []Point, from, to time.Time, n int) []float64 {
	if len(pts) == 0 || n <= 0 || !to.After(from) {
		return nil
	}
	step := to.Sub(from) / time.Duration(n)
	out := make([]float64, n)
	last := pts[0].V
	i := 0
	// readings from before the window set the starting value
	for i < len(pts) && !pts[i].T.After(from) {
		last = pts[i].V
		i++
	}
	for b := 0; b < n; b++ {
		end := from.Add(step * time.Duration(b+1))
		sum, count := 0.0, 0
		for i < len(pts) && !pts[i].T.After(end) {
			sum += pts[i].V
			count++
			i++
		}
		if count > 0 {
			last = sum / float64(count)
		}
		out[b] = last
	}
	return out
}

// Stats is the summary line under a graph.
type Stats struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
	Avg float64 `json:"avg"`
}

// Summarize works out the smallest, largest and average value.
func Summarize(vals []float64) Stats {
	if len(vals) == 0 {
		return Stats{}
	}
	s := Stats{Min: vals[0], Max: vals[0]}
	sum := 0.0
	for _, v := range vals {
		s.Min = math.Min(s.Min, v)
		s.Max = math.Max(s.Max, v)
		sum += v
	}
	s.Avg = sum / float64(len(vals))
	return s
}
