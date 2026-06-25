package dudect

import (
	"crypto/rand"
	"fmt"
	"math"
	"slices"
)

type State int

const (
	LeakageFound State = iota
	NoLeakageEvidenceYet
)

const (
	numberPercentiles  = 100
	numberTests        = numberPercentiles + 2
	enoughMeasurements = 10000
	thresholdBananas   = 500 // tTest failed, with overwhelming probability
	thresholdModerate  = 10  // tTest failed. Pankaj likes 4.5 but let's be more lenient
)

// DudectContext holds the accumulated timing statistics for a constant-time test.
type DudectContext struct {
	numberMeasurements int
	percentiles        [numberPercentiles]int
	tests              [numberTests]tTest
	classes            []byte
	execTimes          []int
}

// NewDudectContext creates a context doing numberMeasurements timed calls per Round.
func NewDudectContext(numberMeasurements int) *DudectContext {
	return &DudectContext{
		numberMeasurements: numberMeasurements,
		classes:            make([]byte, numberMeasurements),
		execTimes:          make([]int, numberMeasurements),
	}
}

// Round runs one batch of measurements and folds it into the statistics.
// random is the random-input function and fixed is the fixed-input function.
// Call it repeatedly: it returns LeakageFound once a t-test exceeds the threshold, else NoLeakageEvidenceYet.
func (ctx *DudectContext) Round(random func(int), fixed func(int)) State {
	ctx.prepare()
	ctx.measure(random, fixed)
	firstTime := ctx.percentiles[numberPercentiles-1] == 0
	ret := NoLeakageEvidenceYet

	if firstTime {
		// throw away the first batch of measurements.
		// this helps warming things up.
		ctx.preparePercentiles()
	} else {
		ctx.updateStatistics()
		ret = ctx.report()
	}

	return ret
}

func (ctx *DudectContext) prepare() {
	if _, err := rand.Read(ctx.classes); err != nil {
		panic(err)
	}
	for i := 0; i < len(ctx.classes); i++ {
		ctx.classes[i] &= 1
	}
}

func (ctx *DudectContext) measure(random func(int), fixed func(int)) {
	functions := [2]func(index int){fixed, random}

	for i := 0; i < ctx.numberMeasurements; i++ {
		c := ctx.classes[i]
		f := functions[c]

		before := cpuTicks()
		f(i)
		after := cpuTicks()

		ctx.execTimes[i] = int(after - before)
	}
}

func percentile(aSorted []int, which float64) int {
	size := len(aSorted)
	arrayPosition := int(float64(size) * float64(which))
	if arrayPosition >= size {
		panic("arrayPosition out of range")
	}
	return aSorted[arrayPosition]
}

func (ctx *DudectContext) preparePercentiles() {
	slices.Sort(ctx.execTimes)
	for i := 0; i < numberPercentiles; i++ {
		ctx.percentiles[i] = percentile(ctx.execTimes, 1-(math.Pow(0.5, 10*float64(i+1)/float64(numberPercentiles))))
	}
}

type tTest struct {
	mean [2]float64
	m2   [2]float64
	n    [2]float64
}

func (t *tTest) push(x float64, clazz byte) {
	t.n[clazz]++

	// estimate variance on the fly as per the Welford method.
	// this gives good numerical stability, see Knuth's TAOCP vol 2

	delta := x - t.mean[clazz]
	t.mean[clazz] = t.mean[clazz] + delta/t.n[clazz]
	t.m2[clazz] = t.m2[clazz] + delta*(x-t.mean[clazz])
}

func (t *tTest) compute() float64 {
	if t.n[0] < 2 || t.n[1] < 2 {
		return 0
	}
	v0 := t.m2[0] / (t.n[0] - 1)
	v1 := t.m2[1] / (t.n[1] - 1)
	num := t.mean[0] - t.mean[1]
	den := math.Sqrt(v0/t.n[0] + v1/t.n[1])
	return num / den
}

func (ctx *DudectContext) updateStatistics() {
	for i := 10; /* discard the first few measurements */ i < ctx.numberMeasurements; i++ {
		difference := ctx.execTimes[i]

		if difference < 0 {
			continue // the cpu cycle counter overflowed, just throw away the measurement
		}

		// t-test on the execution time
		ctx.tests[0].push(float64(difference), ctx.classes[i])

		// t-test on cropped execution times, for several cropping thresholds.
		for cropIndex := 0; cropIndex < numberPercentiles; cropIndex++ {
			if difference < ctx.percentiles[cropIndex] {
				ctx.tests[cropIndex+1].push(float64(difference), ctx.classes[i])
			}
		}

		// second-order test (only if we have more than 10000 measurements).
		// Centered product pre-processing.
		if ctx.tests[0].n[0] > enoughMeasurements {
			centered := float64(difference) - ctx.tests[0].mean[ctx.classes[i]]
			ctx.tests[1+numberPercentiles].push(centered*centered, ctx.classes[i])
		}
	}
}

func (ctx *DudectContext) maxTest() *tTest {
	var best *tTest
	m := 0.0
	for i := 0; i < numberTests; i++ {
		if ctx.tests[i].n[0] > enoughMeasurements {
			x := math.Abs(ctx.tests[i].compute())
			if best == nil || m < x {
				m = x
				best = &ctx.tests[i]
			}
		}
	}
	return best
}

func (ctx *DudectContext) report() State {
	t := ctx.maxTest()
	if t == nil {
		stillToGo := enoughMeasurements - ctx.tests[0].n[0]
		fmt.Printf("not enough measurements (%.0f still to go).\n", stillToGo)
		return NoLeakageEvidenceYet
	}

	maxT := math.Abs(t.compute())
	numberTracesMaxT := t.n[0] + t.n[1]
	maxTau := maxT / math.Sqrt(numberTracesMaxT)

	// print the number of measurements of the test that yielded max t.
	// sometimes you can see this number go down - this can be confusing
	// but can happen (different test)
	fmt.Printf("meas: %7.2f M, ", (numberTracesMaxT / 1e6))

	/*
	 * We report the following statistics:
	 *
	 * maxT: the t value
	 * maxTau: a t value normalized by sqrt(number of measurements).
	 *          this way we can compare maxTau taken with different
	 *          number of measurements. This is sort of "distance
	 *          between distributions", independent of number of
	 *          measurements.
	 * (5/tau)^2: how many measurements we would need to barely
	 *            detect the leak, if present. "barely detect the
	 *            leak" here means have a t value greater than 5.
	 *
	 * The first metric is standard; the other two aren't (but
	 * pretty sensible imho)
	 */

	fmt.Printf("max t: %+7.2f, max tau: %.2e, (5/tau)^2: %.2e.", maxT, maxTau, (5.0*5.0)/(maxTau*maxTau))

	if maxT > thresholdBananas {
		fmt.Print(" Definitely not constant time.\n")
		return LeakageFound
	}
	if maxT > thresholdModerate {
		fmt.Print(" Probably not constant time.\n")
		return LeakageFound
	}
	if maxT < thresholdModerate {
		fmt.Print(" For the moment, maybe constant time.\n")
	}
	return NoLeakageEvidenceYet
}
