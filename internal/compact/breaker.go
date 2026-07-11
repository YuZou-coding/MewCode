package compact

type Breaker struct {
	failures int
}

func (b *Breaker) RecordSuccess() {
	b.failures = 0
}

func (b *Breaker) RecordFailure() {
	b.failures++
}

func (b *Breaker) Failures() int {
	return b.failures
}

func (b *Breaker) AutomaticDisabled() bool {
	return b.failures >= SummaryFailureLimit
}
