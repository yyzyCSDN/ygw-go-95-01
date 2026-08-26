package grid

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestController(match func(Phase) bool) *Controller {
	breaker := NewSimpleBreaker()
	syncer := NewSyncer(match)
	return NewController(breaker, syncer, nil, nil, nil)
}

func TestSyncAndClose_RejectsWhenPhaseNotChecked(t *testing.T) {
	ctrl := newTestController(func(p Phase) bool { return true })
	phase := Phase{VoltageKV: 10, FreqHz: 50, Degree: 0}

	// 没有调用 PhaseCheck，直接合闸必须被门禁拒绝，断路器不得闭合。
	err := ctrl.SyncAndClose(context.Background(), phase, 100*time.Millisecond)
	if !errors.Is(err, ErrPhaseNotChecked) {
		t.Fatalf("expected ErrPhaseNotChecked, got %v", err)
	}
	if ctrl.BreakerClosed() {
		t.Fatal("breaker must remain open when phase not checked")
	}
	if ctrl.State() == StateOnGrid {
		t.Fatal("grid must not enter on-grid when phase not checked")
	}
}

func TestSyncAndClose_RejectsWhenPhaseDiffersFromChecked(t *testing.T) {
	ctrl := newTestController(func(p Phase) bool { return true })
	checked := Phase{VoltageKV: 10, FreqHz: 50, Degree: 0}
	requested := Phase{VoltageKV: 6, FreqHz: 50, Degree: 0}

	if err := ctrl.PhaseCheck(checked); err != nil {
		t.Fatalf("PhaseCheck: %v", err)
	}
	// 核对相位与请求合闸相位不一致时，必须拒绝合闸。
	err := ctrl.SyncAndClose(context.Background(), requested, 100*time.Millisecond)
	if !errors.Is(err, ErrPhaseNotChecked) {
		t.Fatalf("expected ErrPhaseNotChecked, got %v", err)
	}
	if ctrl.BreakerClosed() {
		t.Fatal("breaker must remain open when checked phase differs")
	}
}

func TestSyncAndClose_AllowsAfterPhaseCheck(t *testing.T) {
	ctrl := newTestController(func(p Phase) bool { return true })
	phase := Phase{VoltageKV: 10, FreqHz: 50, Degree: 0}

	if err := ctrl.PhaseCheck(phase); err != nil {
		t.Fatalf("PhaseCheck: %v", err)
	}
	if err := ctrl.SyncAndClose(context.Background(), phase, 100*time.Millisecond); err != nil {
		t.Fatalf("SyncAndClose after PhaseCheck: %v", err)
	}
	if !ctrl.BreakerClosed() {
		t.Fatal("breaker should be closed after authorized close")
	}
	if ctrl.State() != StateOnGrid {
		t.Fatal("grid should be on-grid after authorized close")
	}
}

func TestSyncAndClose_InterlockResetsAfterSeparate(t *testing.T) {
	ctrl := newTestController(func(p Phase) bool { return true })
	phase := Phase{VoltageKV: 10, FreqHz: 50, Degree: 0}

	if err := ctrl.PhaseCheck(phase); err != nil {
		t.Fatalf("PhaseCheck: %v", err)
	}
	if err := ctrl.SyncAndClose(context.Background(), phase, 100*time.Millisecond); err != nil {
		t.Fatalf("SyncAndClose: %v", err)
	}
	if err := ctrl.Separate(); err != nil {
		t.Fatalf("Separate: %v", err)
	}
	// 解列后门禁复位：再次合闸前必须重新核对相位。
	err := ctrl.SyncAndClose(context.Background(), phase, 100*time.Millisecond)
	if !errors.Is(err, ErrPhaseNotChecked) {
		t.Fatalf("expected ErrPhaseNotChecked after separate, got %v", err)
	}
}

func TestApplySequence_RejectsCloseBreakerWhenPhaseNotChecked(t *testing.T) {
	ctrl := newTestController(func(p Phase) bool { return true })
	seq := Sequence{
		ID: "switch-on",
		Steps: []SequenceStep{
			{Kind: StepSetGridState, GridState: StateSyncing},
			{Kind: StepCloseBreaker},
		},
	}
	// 序列路径也必须受相位门禁约束，未核对时不得合闸。
	err := ctrl.ApplySequence(seq)
	if !errors.Is(err, ErrPhaseNotChecked) {
		t.Fatalf("expected ErrPhaseNotChecked, got %v", err)
	}
	if ctrl.BreakerClosed() {
		t.Fatal("breaker must remain open when sequence closes without phase check")
	}
}

func TestApplySequence_AllowsCloseBreakerAfterPhaseCheck(t *testing.T) {
	ctrl := newTestController(func(p Phase) bool { return true })
	phase := Phase{VoltageKV: 10, FreqHz: 50, Degree: 0}
	if err := ctrl.PhaseCheck(phase); err != nil {
		t.Fatalf("PhaseCheck: %v", err)
	}
	seq := Sequence{
		ID: "switch-on",
		Steps: []SequenceStep{
			{Kind: StepSetGridState, GridState: StateSyncing},
			{Kind: StepCloseBreaker},
		},
	}
	if err := ctrl.ApplySequence(seq); err != nil {
		t.Fatalf("ApplySequence after PhaseCheck: %v", err)
	}
	if !ctrl.BreakerClosed() {
		t.Fatal("breaker should be closed after authorized sequence close")
	}
}
