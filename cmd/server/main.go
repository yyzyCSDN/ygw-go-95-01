package main

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"ygw-go-95-01/internal/alarm"
	"ygw-go-95-01/internal/berth"
	"ygw-go-95-01/internal/capacity"
	"ygw-go-95-01/internal/config"
	"ygw-go-95-01/internal/connect"
	"ygw-go-95-01/internal/event"
	"ygw-go-95-01/internal/grid"
	"ygw-go-95-01/internal/meter"
	"ygw-go-95-01/internal/record"
)

type app struct {
	cfg        config.Config
	bus        *event.Bus
	store      *berth.Store
	alloc      *capacity.Allocator
	ctrl       *grid.Controller
	conn       *connect.Connector
	meter      *meter.Meter
	billing    *meter.Billing
	alarms     *alarm.Manager
	actions    *alarm.Actions
	dispatch   *alarm.DispatchLoop
	reconciler *capacity.Reconciler
	rec        *record.Recorder
	hub        *wsHub
}

type snapshot struct {
	Time     string                 `json:"time"`
	Grid     gridStateView          `json:"grid"`
	Berths   []berth.Berth          `json:"berths"`
	Plan     capacity.Plan          `json:"plan"`
	Sessions []connect.Session      `json:"sessions"`
	Alarms   []alarm.Alarm          `json:"alarms"`
	Meters   []meter.Run            `json:"meters"`
	Override connect.Override       `json:"override"`
	Rate     map[meter.Tier]float64 `json:"rate"`
}

type gridStateView struct {
	State         grid.GridState `json:"state"`
	BreakerClosed bool           `json:"breaker_closed"`
	PhaseChecked  bool           `json:"phase_checked"`
	Mode          string         `json:"mode"`
	Executions    int            `json:"executions"`
	QueueLen      int            `json:"queue_len"`
}

func main() {
	configPath := flag.String("config", "", "path to server config json")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}
	recStore := record.NewStore()
	rec, err := record.NewRecorder(filepath.Join(cfg.DataDir, "records"), recStore)
	if err != nil {
		log.Fatalf("init recorder: %v", err)
	}
	bus := event.NewBus()
	store := berth.NewStore(seedBerths())
	alloc := capacity.NewAllocator(store, cfg.ShoreCapacity)
	syncer := grid.NewSyncer(func(p grid.Phase) bool {
		return p.VoltageKV == cfg.VoltageKV && p.FreqHz == cfg.FreqHz && p.Degree == 0
	})
	breaker := grid.NewSimpleBreaker()
	ctrl := grid.NewController(breaker, syncer, store, rec, bus)
	actions := alarm.NewActions(ctrl, rec)
	alarms := alarm.NewManager(ctrl, store, actions)
	dispatch := alarm.NewDispatchLoop(bus, alarms, actions)
	meterStore := meter.NewStore()
	meterSvc := meter.NewMeter(rec, bus, meterStore)
	rate := meter.NewRate(
		cfg.PeakStartHour,
		cfg.PeakEndHour,
		cfg.ValleyStartHour,
		cfg.ValleyEndHour,
		cfg.EnergyUnitPrice,
		cfg.ServiceUnitPrice,
	)
	billing := meter.NewBilling(meterSvc, rate, rec)
	connector := connect.NewConnector(ctrl, alloc, store, meterSvc, bus, cfg.VerifyTimeout, cfg.SyncTimeout)
	reconciler := capacity.NewReconciler(alloc, store, bus, cfg.ReconcileEvery)
	hub := newWSHub()
	a := &app{
		cfg:        cfg,
		bus:        bus,
		store:      store,
		alloc:      alloc,
		ctrl:       ctrl,
		conn:       connector,
		meter:      meterSvc,
		billing:    billing,
		alarms:     alarms,
		actions:    actions,
		dispatch:   dispatch,
		reconciler: reconciler,
		rec:        rec,
		hub:        hub,
	}
	_ = a.rebuildLiveState()
	mux := http.NewServeMux()
	a.routes(mux)
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go reconciler.Run(ctx)
	go dispatch.Run(ctx)
	go a.sampleLoop(ctx)
	go hub.broadcastLoop(ctx, a)
	go func() {
		log.Printf("shore power control listening on %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	cancel()
	reconciler.Stop()
	dispatch.Stop()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	_ = rec.Flush()
	_ = rec.Close()
	_ = bus.Close()
}

func (a *app) routes(mux *http.ServeMux) {
	mux.HandleFunc("/", a.handleConsole)
	mux.HandleFunc("/api/berths", a.handleBerths)
	mux.HandleFunc("/api/berths/occupy", a.handleOccupy)
	mux.HandleFunc("/api/berths/depart", a.handleDepart)
	mux.HandleFunc("/api/capacity/plan", a.handlePlan)
	mux.HandleFunc("/api/connect/apply", a.handleApply)
	mux.HandleFunc("/api/connect/confirm", a.handleConfirm)
	mux.HandleFunc("/api/connect/wait", a.handleWait)
	mux.HandleFunc("/api/connect/cancel", a.handleCancel)
	mux.HandleFunc("/api/connect/sessions", a.handleSessions)
	mux.HandleFunc("/api/grid/state", a.handleGridState)
	mux.HandleFunc("/api/grid/separate", a.handleSeparate)
	mux.HandleFunc("/api/grid/switch", a.handleSwitch)
	mux.HandleFunc("/api/meter/samples", a.handleSamples)
	mux.HandleFunc("/api/meter/settle", a.handleSettle)
	mux.HandleFunc("/api/meter/suspend", a.handleMeterSuspend)
	mux.HandleFunc("/api/meter/resume", a.handleMeterResume)
	mux.HandleFunc("/api/alarms", a.handleAlarms)
	mux.HandleFunc("/api/alarms/ack", a.handleAlarmAck)
	mux.HandleFunc("/api/control/override", a.handleOverride)
	mux.HandleFunc("/api/control/dispatch", a.handleDispatch)
	mux.HandleFunc("/api/recovery/rebuild", a.handleRecoveryRebuild)
	mux.HandleFunc("/api/recovery/snapshot", a.handleRecoverySnapshot)
	mux.HandleFunc("/api/snapshot", a.handleSnapshot)
	mux.HandleFunc("/ws/console", a.hub.handle)
}

func (a *app) handleConsole(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	raw, err := a.consoleHTML()
	if err != nil {
		http.Error(w, "console unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(raw)
}

func (a *app) consoleHTML() ([]byte, error) {
	candidates := []string{filepath.Join("web", "console.html")}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "web", "console.html"))
	}
	for _, candidate := range candidates {
		if raw, err := os.ReadFile(candidate); err == nil {
			return raw, nil
		}
	}
	return nil, os.ErrNotExist
}

func (a *app) handleBerths(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.store.List())
}

func (a *app) handleOccupy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Code    string  `json:"code"`
		BerthID string  `json:"berth_id"`
		Vessel  string  `json:"vessel"`
		NeedKVA float64 `json:"need_kva"`
	}
	if err := decodeBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.BerthID == "" && req.Code != "" {
		if b, ok := a.store.ByCode(req.Code); ok {
			req.BerthID = b.ID
		}
	}
	if req.BerthID == "" {
		http.Error(w, "berth_id or code is required", http.StatusBadRequest)
		return
	}
	if err := a.store.CheckReady(req.BerthID, req.NeedKVA); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if _, err := a.alloc.Allocate(req.BerthID, req.Vessel, req.NeedKVA); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := a.store.MarkOccupied(req.BerthID, req.Vessel); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"berth": req.BerthID, "vessel": req.Vessel, "state": string(berth.StateSettled)})
}

func (a *app) handleDepart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		BerthID string `json:"berth_id"`
	}
	if err := decodeBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	vessel, ok := a.store.OccupiedBy(req.BerthID)
	if !ok {
		http.Error(w, "berth is not occupied", http.StatusConflict)
		return
	}
	if a.meter.StateOf(vessel) == meter.Metering {
		if _, err := a.billing.Settle(vessel); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	}
	if err := a.store.MarkReleasing(req.BerthID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := a.alloc.Release(req.BerthID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := a.store.MarkIdle(req.BerthID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"berth": req.BerthID, "state": string(berth.StateIdle)})
}

func (a *app) handlePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, capacity.BuildPlan(a.store, a.alloc))
}

func (a *app) handleApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		RequestID string  `json:"request_id"`
		Vessel    string  `json:"vessel"`
		BerthID   string  `json:"berth_id"`
		NeedKVA   float64 `json:"need_kva"`
		VoltageKV float64 `json:"voltage_kv"`
		FreqHz    float64 `json:"freq_hz"`
		Degree    float64 `json:"degree"`
	}
	if err := decodeBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sess, err := a.conn.Apply(connect.ApplyRequest{
		RequestID: req.RequestID,
		Vessel:    req.Vessel,
		BerthID:   req.BerthID,
		NeedKVA:   req.NeedKVA,
		Phase: grid.Phase{
			VoltageKV: req.VoltageKV,
			FreqHz:    req.FreqHz,
			Degree:    req.Degree,
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (a *app) handleConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := decodeBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.conn.ConfirmVerification(req.SessionID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"session": req.SessionID, "state": string(connect.StateConnected)})
}

func (a *app) handleWait(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := decodeBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.conn.WaitVerification(req.SessionID, a.conn.VerifyTimeout()); err != nil {
		http.Error(w, err.Error(), http.StatusGatewayTimeout)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"session": req.SessionID, "state": "verified"})
}

func (a *app) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := decodeBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.conn.Cancel(req.SessionID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"session": req.SessionID, "state": string(connect.StateCancelled)})
}

func (a *app) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.conn.Sessions())
}

func (a *app) handleGridState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.gridView())
}

func (a *app) handleSeparate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.ctrl.Separate(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, a.gridView())
}

func (a *app) handleSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SeqID    string  `json:"seq_id"`
		BerthID  string  `json:"berth_id"`
		Vessel   string  `json:"vessel"`
		OnGrid   bool    `json:"on_grid"`
		CloseKVA float64 `json:"close_kva"`
	}
	if err := decodeBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	seq := grid.Sequence{ID: req.SeqID}
	if req.OnGrid {
		seq.Steps = append(seq.Steps, grid.SequenceStep{Kind: grid.StepSetGridState, GridState: grid.StateSyncing})
		seq.Steps = append(seq.Steps, grid.SequenceStep{Kind: grid.StepSetBerthState, BerthID: req.BerthID, BerthState: berth.StateSettled, Vessel: req.Vessel})
		seq.Steps = append(seq.Steps, grid.SequenceStep{Kind: grid.StepCloseBreaker})
	} else {
		seq.Steps = append(seq.Steps, grid.SequenceStep{Kind: grid.StepOpenBreaker})
		seq.Steps = append(seq.Steps, grid.SequenceStep{Kind: grid.StepSetBerthState, BerthID: req.BerthID, BerthState: berth.StateIdle})
		seq.Steps = append(seq.Steps, grid.SequenceStep{Kind: grid.StepSetGridState, GridState: grid.StateOff})
	}
	if err := a.ctrl.ApplySequence(seq); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, a.gridView())
}

func (a *app) handleSamples(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.meter.Runs())
}

func (a *app) handleSettle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Vessel string `json:"vessel"`
	}
	if err := decodeBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	bill, err := a.billing.Settle(req.Vessel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, bill)
}

func (a *app) handleMeterSuspend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Vessel string `json:"vessel"`
	}
	if err := decodeBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.meter.Suspend(req.Vessel); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"vessel": req.Vessel, "state": string(meter.Suspended)})
}

func (a *app) handleMeterResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Vessel string `json:"vessel"`
	}
	if err := decodeBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.meter.Resume(req.Vessel); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"vessel": req.Vessel, "state": string(meter.Metering)})
}

func (a *app) handleAlarms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.alarms.Alarms())
}

func (a *app) handleAlarmAck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		AlarmID string `json:"alarm_id"`
	}
	if err := decodeBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.alarms.Ack(req.AlarmID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"alarm": req.AlarmID, "acked": "true"})
}

func (a *app) handleOverride(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Active bool   `json:"active"`
		Vessel string `json:"vessel"`
	}
	if err := decodeBody(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.conn.SetLocalOverride(connect.Override{Active: req.Active, Vessel: req.Vessel, Since: time.Now().UTC()}); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, a.conn.Override())
}

func (a *app) handleDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	count, err := a.conn.AutoDispatch()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"dispatched": count})
}

func (a *app) handleRecoveryRebuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.rebuildLiveState(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, a.store.List())
}

func (a *app) handleRecoverySnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snap := a.store.TakeSnapshot()
	if err := a.store.PersistSnapshot(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, snap.Berths)
}

func (a *app) rebuildLiveState() error {
	live := make(map[string]berth.Berth)
	allocated := make(map[string]bool)
	for _, alloc := range a.alloc.Allocations() {
		allocated[alloc.BerthID] = true
	}
	for _, b := range a.store.List() {
		if allocated[b.ID] {
			b.State = berth.StateSettled
			if vessel, ok := a.store.OccupiedBy(b.ID); ok {
				b.Vessel = vessel
			}
		} else {
			b.State = berth.StateIdle
			b.Vessel = ""
			b.OccupiedAt = time.Time{}
		}
		live[b.ID] = b
	}
	if err := a.store.RebuildFromLive(live); err != nil {
		return err
	}
	_, err := a.alloc.SyncFromStore()
	return err
}

func (a *app) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.buildSnapshot())
}

func (a *app) gridView() gridStateView {
	mode := "auto"
	if a.ctrl.ControlMode() == grid.ControlManual {
		mode = "manual"
	}
	return gridStateView{
		State:         a.ctrl.State(),
		BreakerClosed: a.ctrl.BreakerClosed(),
		PhaseChecked:  a.ctrl.PhaseChecked(),
		Mode:          mode,
		Executions:    a.ctrl.ExecutionCount(),
		QueueLen:      a.ctrl.QueueLen(),
	}
}

func (a *app) buildSnapshot() snapshot {
	rate := meter.NewRate(
		a.cfg.PeakStartHour,
		a.cfg.PeakEndHour,
		a.cfg.ValleyStartHour,
		a.cfg.ValleyEndHour,
		a.cfg.EnergyUnitPrice,
		a.cfg.ServiceUnitPrice,
	)
	return snapshot{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Grid:     a.gridView(),
		Berths:   a.store.List(),
		Plan:     capacity.BuildPlan(a.store, a.alloc),
		Sessions: a.conn.Sessions(),
		Alarms:   a.alarms.Alarms(),
		Meters:   a.meter.Runs(),
		Override: a.conn.Override(),
		Rate:     rate.Prices(),
	}
}

func (a *app) sampleLoop(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.SampleEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, run := range a.meter.Runs() {
				_, _ = a.meter.Sample(run.Vessel)
			}
		}
	}
}

func decodeBody(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func seedBerths() []berth.Berth {
	return []berth.Berth{
		{ID: "B01", Code: "B01", VoltageKV: 10, CapacityKVA: 3000, State: berth.StateIdle},
		{ID: "B02", Code: "B02", VoltageKV: 10, CapacityKVA: 2500, State: berth.StateIdle},
		{ID: "B03", Code: "B03", VoltageKV: 10, CapacityKVA: 2000, State: berth.StateIdle},
		{ID: "B04", Code: "B04", VoltageKV: 6, CapacityKVA: 1500, State: berth.StateIdle},
		{ID: "B05", Code: "B05", VoltageKV: 6, CapacityKVA: 1200, State: berth.StateIdle},
		{ID: "B06", Code: "B06", VoltageKV: 6, CapacityKVA: 1000, State: berth.StateIdle},
		{ID: "B07", Code: "B07", VoltageKV: 6, CapacityKVA: 800, State: berth.StateIdle},
		{ID: "B08", Code: "B08", VoltageKV: 10, CapacityKVA: 2200, State: berth.StateIdle},
	}
}

type wsClient struct {
	conn net.Conn
	send chan []byte
}

type wsHub struct {
	mu      sync.Mutex
	clients map[*wsClient]bool
}

func newWSHub() *wsHub {
	return &wsHub{clients: make(map[*wsClient]bool)}
}

func (h *wsHub) handle(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "websocket handshake required", http.StatusBadRequest)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, "hijack failed", http.StatusInternalServerError)
		return
	}
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	_, _ = conn.Write([]byte(
		"HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + accept + "\r\n\r\n",
	))
	client := &wsClient{conn: conn, send: make(chan []byte, 8)}
	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()
	go client.writeLoop()
	go client.readLoop(h)
}

func (c *wsClient) writeLoop() {
	for payload := range c.send {
		_, _ = c.conn.Write(wsFrame(payload))
	}
	_ = c.conn.Close()
}

func (c *wsClient) readLoop(h *wsHub) {
	buf := make([]byte, 128)
	for {
		if _, err := c.conn.Read(buf); err != nil {
			break
		}
	}
	h.mu.Lock()
	if h.clients[c] {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

func (h *wsHub) broadcastLoop(ctx context.Context, a *app) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			payload, err := json.Marshal(a.buildSnapshot())
			if err != nil {
				continue
			}
			h.broadcast(payload)
		}
	}
}

func (h *wsHub) broadcast(payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
		select {
		case client.send <- payload:
		default:
		}
	}
}

func wsFrame(payload []byte) []byte {
	frame := []byte{0x81}
	size := len(payload)
	switch {
	case size < 126:
		frame = append(frame, byte(size))
	case size <= 65535:
		frame = append(frame, 126, byte(size>>8), byte(size))
	default:
		frame = append(frame, 127,
			byte(size>>56), byte(size>>48), byte(size>>40), byte(size>>32),
			byte(size>>24), byte(size>>16), byte(size>>8), byte(size))
	}
	return append(frame, payload...)
}
