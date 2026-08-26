package connect

type Step int

const (
	StepVerify Step = iota
	StepPhaseCheck
	StepLink
	StepClose
)

func (s Step) String() string {
	switch s {
	case StepVerify:
		return "verify"
	case StepPhaseCheck:
		return "phase-check"
	case StepLink:
		return "link"
	case StepClose:
		return "close"
	default:
		return "unknown"
	}
}

func (s Step) Valid() bool {
	return s >= StepVerify && s <= StepClose
}
