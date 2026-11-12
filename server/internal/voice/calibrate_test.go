package voice

import (
	"sort"
	"testing"
)

// I2: a single short reply used to drag every metric's lo to 0 and set hi
// from one long email (gate-p12-p5-review.md). The fix: only messages of
// 40 or more words count, and percentile() is the 10th/90th-percentile
// building block Calibrate uses instead of min/max.
func TestPercentile_TenAndNinety(t *testing.T) {
	// Linear interpolation over 5 sorted values 10,20,30,40,50: p10 is at
	// rank 0.4 (10% of 4 steps), p90 is at rank 3.6.
	vals := []float64{10, 20, 30, 40, 50}
	if got := percentile(vals, 10); got != 14 {
		t.Fatalf("p10 = %v, want 14", got)
	}
	if got := percentile(vals, 90); got != 46 {
		t.Fatalf("p90 = %v, want 46", got)
	}
	if got := percentile([]float64{7}, 10); got != 7 {
		t.Fatalf("single value = %v, want 7", got)
	}
	if got := percentile([]float64{7, 9}, 50); got != 8 {
		t.Fatalf("midpoint = %v, want 8", got)
	}
}

// I2: calibrate over messages of 40 or more words only. These three
// messages (30, 33 and 37 words) are each under that floor, so none
// qualify and Calibrate must say so with nil, the same as too few
// messages altogether.
func TestCalibrate_SkipsMessagesUnder40Words(t *testing.T) {
	corpus := "I'm sending the draft today, so you can review it before Wed. I think the numbers hold up, and I checked them twice. Let me know if anything looks off.\n---\n" +
		"We moved the review to Thursday. I don't have the final cost yet, but you'll have it by noon. My guess is it lands close to the estimate, so we should be fine.\n---\n" +
		"Thanks for the notes. I agree with most of them, and I'll fold them in tonight. Your point about the timeline is fair, so I pushed the launch a week. We can talk Monday if you want.\n"
	if Calibrate(corpus) != nil {
		t.Fatal("three sub-40-word messages must give nil, not a baseline")
	}
}

// I2: each range is the 10th to 90th percentile across qualifying
// messages, lo x 0.9 and hi x 1.1 -- not the old min/max. Built from the
// same measure() and percentile() building blocks those functions already
// have their own tests for, so this checks Calibrate wires them together
// correctly, not that the arithmetic itself is right.
func TestCalibrate_PercentileRangeFromQualifyingMessages(t *testing.T) {
	msgA := "I'm sending the complete draft to the whole team today, so everyone can review it closely before Wednesday afternoon. I think the numbers hold up well under scrutiny, and I checked each line twice against the source data we collected. Let me know if anything here looks off to you, because I'd rather fix it now than later."
	msgB := "We moved the full review to Thursday morning instead, since the room was already booked for something else. I don't have the final cost yet, but you'll have the complete number by noon tomorrow at the latest. My guess is it lands close to the estimate, so we should plan around that for now."
	msgC := "Thanks for all the detailed notes on the proposal, they were really thorough. I agree with most of them, and I'll fold the easy ones in tonight before the next draft goes out. Your point about the timeline is fair, so I pushed the launch back a full week to be safe."
	corpus := msgA + "\n---\n" + msgB + "\n---\n" + msgC + "\n"

	got := Calibrate(corpus)
	if got == nil {
		t.Fatal("three 40+ word messages must give a baseline, not nil")
	}
	baseline := got["baseline"].(map[string]any)

	for _, sm := range ScoredMetrics {
		var vals []float64
		for _, msg := range []string{msgA, msgB, msgC} {
			vals = append(vals, metricValue(measure(nil, msg), sm.Key))
		}
		sort.Float64s(vals)
		wantLo := round2(percentile(vals, 10) * 0.9)
		wantHi := round2(percentile(vals, 90) * 1.1)
		r := baseline[sm.Key].(map[string]any)
		if r["lo"] != wantLo || r["hi"] != wantHi {
			t.Errorf("%s: got lo=%v hi=%v, want lo=%v hi=%v", sm.Key, r["lo"], r["hi"], wantLo, wantHi)
		}
	}
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
