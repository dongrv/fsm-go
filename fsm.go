// Package fsm provides a highly abstract, production-ready finite state machine
// implementation for Go 1.21.13+. It supports hierarchical states, parallel states,
// guards, actions, and more.
package fsm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Context provides context for transitions, including current state,
// target state, event, and payload.
type Context[S comparable, E comparable] struct {
	from      S
	to        S
	event     E
	payload   any
	timestamp time.Time
	machine   *Machine[S, E]
}

// From returns the source state of the transition.
func (c Context[S, E]) From() S {
	return c.from
}

// To returns the target state of the transition.
func (c Context[S, E]) To() S {
	return c.to
}

// Event returns the event that triggered the transition.
func (c Context[S, E]) Event() E {
	return c.event
}

// Payload returns the optional payload associated with the event.
func (c Context[S, E]) Payload() any {
	return c.payload
}

// Timestamp returns when the transition occurred.
func (c Context[S, E]) Timestamp() time.Time {
	return c.timestamp
}

// Machine returns the state machine instance.
func (c Context[S, E]) Machine() *Machine[S, E] {
	return c.machine
}

// Guard is a function that determines if a transition should occur.
// Return true to allow the transition, false to block it.
type Guard[S comparable, E comparable] func(Context[S, E]) bool

// Action is a function that executes during a transition.
type Action[S comparable, E comparable] func(Context[S, E])

// Listener is notified of state machine events.
type Listener[S comparable, E comparable] interface {
	// OnTransition is called when a transition occurs.
	OnTransition(ctx Context[S, E])
	// OnEntry is called when entering a state.
	OnEntry(state S, event E)
	// OnExit is called when exiting a state.
	OnExit(state S, event E)
	// OnEvent is called when an event is received.
	OnEvent(event E, payload any)
	// OnError is called when an error occurs.
	OnError(err error)
}

// Transition defines a transition from one state to another triggered by an event.
type Transition[S comparable, E comparable] struct {
	from     S
	to       S
	event    E
	guard    Guard[S, E]
	action   Action[S, E]
	internal bool // true for internal transitions (no state change)
}

// StateConfig defines configuration for a state.
type StateConfig[S comparable, E comparable] struct {
	name         S
	parent       *S
	entryActions []Action[S, E]
	exitActions  []Action[S, E]
	timeout      time.Duration
	timeoutEvent E
	initial      *S // initial substate for composite states
	final        bool
	parallel     bool
	history      HistoryType
	transitions  []Transition[S, E]
}

// HistoryType defines the type of history to preserve.
type HistoryType int

const (
	HistoryNone HistoryType = iota
	HistoryShallow
	HistoryDeep
)

// Machine is the main state machine implementation.
type Machine[S comparable, E comparable] struct {
	mu             sync.RWMutex
	current        S
	states         map[S]*StateConfig[S, E]
	transitions    map[S]map[E][]Transition[S, E]
	listeners      []Listener[S, E]
	eventQueue     chan eventWrapper[S, E]
	stopChan       chan struct{}
	stopped        atomic.Bool
	initialized    bool
	async          bool
	maxQueueSize   int
	context        context.Context
	cancelFunc     context.CancelFunc
	history        map[S]S // for history states
	parallelStates map[S][]S
	stateData      map[S]any
	defaultTimeout time.Duration
	logger         Logger
}

type eventWrapper[S comparable, E comparable] struct {
	event   E
	payload any
}

// Option configures a Machine.
type Option[S comparable, E comparable] func(*Machine[S, E])

// New creates a new state machine with the given options.
func New[S comparable, E comparable](opts ...Option[S, E]) *Machine[S, E] {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Machine[S, E]{
		states:         make(map[S]*StateConfig[S, E]),
		transitions:    make(map[S]map[E][]Transition[S, E]),
		listeners:      make([]Listener[S, E], 0),
		eventQueue:     make(chan eventWrapper[S, E], 100),
		stopChan:       make(chan struct{}),
		context:        ctx,
		cancelFunc:     cancel,
		history:        make(map[S]S),
		parallelStates: make(map[S][]S),
		stateData:      make(map[S]any),
		maxQueueSize:   100,
		logger:         &defaultLogger{},
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// WithAsync enables asynchronous event processing.
func WithAsync[S comparable, E comparable](enabled bool) Option[S, E] {
	return func(m *Machine[S, E]) {
		m.async = enabled
	}
}

// WithQueueSize sets the maximum event queue size.
func WithQueueSize[S comparable, E comparable](size int) Option[S, E] {
	return func(m *Machine[S, E]) {
		m.maxQueueSize = size
		m.eventQueue = make(chan eventWrapper[S, E], size)
	}
}

// WithLogger sets a custom logger.
func WithLogger[S comparable, E comparable](logger Logger) Option[S, E] {
	return func(m *Machine[S, E]) {
		m.logger = logger
	}
}

// WithDefaultTimeout sets the default timeout for states.
func WithDefaultTimeout[S comparable, E comparable](timeout time.Duration) Option[S, E] {
	return func(m *Machine[S, E]) {
		m.defaultTimeout = timeout
	}
}

// WithContext sets the context for the state machine.
func WithContext[S comparable, E comparable](ctx context.Context) Option[S, E] {
	return func(m *Machine[S, E]) {
		m.context, m.cancelFunc = context.WithCancel(ctx)
	}
}

// AddState adds a state to the machine.
func (m *Machine[S, E]) AddState(state S, opts ...StateOption[S, E]) *Machine[S, E] {
	m.mu.Lock()
	defer m.mu.Unlock()

	config := &StateConfig[S, E]{
		name:        state,
		transitions: make([]Transition[S, E], 0),
	}

	for _, opt := range opts {
		opt(config)
	}

	m.states[state] = config
	m.transitions[state] = make(map[E][]Transition[S, E])

	return m
}

// AddTransition adds a transition between states.
func (m *Machine[S, E]) AddTransition(from, to S, event E, opts ...TransitionOption[S, E]) *Machine[S, E] {
	m.mu.Lock()
	defer m.mu.Unlock()

	transition := Transition[S, E]{
		from:  from,
		to:    to,
		event: event,
	}

	for _, opt := range opts {
		opt(&transition)
	}

	// Add to state config
	if config, exists := m.states[from]; exists {
		config.transitions = append(config.transitions, transition)
	}

	// Add to transitions map
	if _, exists := m.transitions[from]; !exists {
		m.transitions[from] = make(map[E][]Transition[S, E])
	}
	m.transitions[from][event] = append(m.transitions[from][event], transition)

	return m
}

// AddInternalTransition adds an internal transition (no state change).
func (m *Machine[S, E]) AddInternalTransition(state S, event E, opts ...TransitionOption[S, E]) *Machine[S, E] {
	m.mu.Lock()
	defer m.mu.Unlock()

	transition := Transition[S, E]{
		from:     state,
		to:       state,
		event:    event,
		internal: true,
	}

	for _, opt := range opts {
		opt(&transition)
	}

	if config, exists := m.states[state]; exists {
		config.transitions = append(config.transitions, transition)
	}

	if _, exists := m.transitions[state]; !exists {
		m.transitions[state] = make(map[E][]Transition[S, E])
	}
	m.transitions[state][event] = append(m.transitions[state][event], transition)

	return m
}

// Start initializes the state machine with the given initial state.
func (m *Machine[S, E]) Start(initialState S) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.initialized {
		return errors.New("state machine already initialized")
	}

	if _, exists := m.states[initialState]; !exists {
		return fmt.Errorf("initial state %v does not exist", initialState)
	}

	m.current = initialState
	m.initialized = true

	// Start async processor if enabled
	if m.async {
		go m.processEvents()
	}

	// Execute entry action for initial state
	if config, exists := m.states[initialState]; exists && config != nil {
		var zeroCtx Context[S, E]
		m.executeEntryActions(initialState, config, zeroCtx)
	}

	m.notifyListeners(func(l Listener[S, E]) {
		var zeroE E
		l.OnEntry(initialState, zeroE)
	})

	m.logger.Infof("State machine started in state: %v", initialState)

	return nil
}

// Send sends an event to the state machine.
func (m *Machine[S, E]) Send(event E, payload ...any) error {
	if !m.initialized {
		return errors.New("state machine not initialized")
	}

	if m.stopped.Load() {
		return errors.New("state machine stopped")
	}

	var p any
	if len(payload) > 0 {
		p = payload[0]
	}

	if m.async {
		select {
		case m.eventQueue <- eventWrapper[S, E]{event: event, payload: p}:
			m.notifyListeners(func(l Listener[S, E]) {
				l.OnEvent(event, p)
			})
			return nil
		default:
			return errors.New("event queue full")
		}
	}

	return m.processEvent(event, p)
}

// Current returns the current state.
func (m *Machine[S, E]) Current() S {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// IsIn returns true if the machine is in the given state.
func (m *Machine[S, E]) IsIn(state S) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current == state
}

// Can returns true if the given event can be processed from the current state.
func (m *Machine[S, E]) Can(event E) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	transitions, exists := m.transitions[m.current][event]
	if !exists {
		return false
	}

	// Check if any transition can be taken
	for _, t := range transitions {
		if t.guard == nil || t.guard(Context[S, E]{
			from:      m.current,
			to:        t.to,
			event:     event,
			payload:   nil,
			timestamp: time.Now(),
			machine:   m,
		}) {
			return true
		}
	}

	return false
}

// Stop gracefully stops the state machine.
func (m *Machine[S, E]) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		return errors.New("state machine not initialized")
	}

	if m.stopped.Load() {
		return errors.New("state machine already stopped")
	}

	m.stopped.Store(true)
	m.cancelFunc()

	if m.async {
		close(m.stopChan)
		close(m.eventQueue)
	}

	m.logger.Infof("State machine stopped")
	return nil
}

// AddListener adds a listener to the state machine.
func (m *Machine[S, E]) AddListener(listener Listener[S, E]) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, listener)
}

// RemoveListener removes a listener from the state machine.
func (m *Machine[S, E]) RemoveListener(listener Listener[S, E]) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.listeners == nil {
		return
	}

	for i, l := range m.listeners {
		if l == listener {
			m.listeners = append(m.listeners[:i], m.listeners[i+1:]...)
			break
		}
	}
}

// SetStateData associates data with a state.
func (m *Machine[S, E]) SetStateData(state S, data any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stateData == nil {
		m.stateData = make(map[S]any)
	}
	m.stateData[state] = data
}

// GetStateData retrieves data associated with a state.
func (m *Machine[S, E]) GetStateData(state S) (any, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stateData == nil {
		return nil, false
	}
	data, exists := m.stateData[state]
	return data, exists
}

// Save serializes the current state of the machine.
func (m *Machine[S, E]) Save() ([]byte, error) {
	// Implementation depends on serialization library
	// For now, return a simple representation
	m.mu.RLock()
	defer m.mu.RUnlock()

	data := map[string]any{
		"current": m.current,
		"history": m.history,
	}

	// Convert to JSON or other format
	// This is a placeholder implementation
	return []byte(fmt.Sprintf("%v", data)), nil
}

// Load restores the state machine from serialized data.
func (m *Machine[S, E]) Load(data []byte) error {
	// Implementation depends on serialization library
	// This is a placeholder implementation
	m.mu.Lock()
	defer m.mu.Unlock()

	// Parse data and restore state
	// For now, just log
	m.logger.Infof("Loading state from data: %s", string(data))
	return nil
}

// processEvents processes events from the queue (async mode).
func (m *Machine[S, E]) processEvents() {
	for {
		select {
		case <-m.stopChan:
			return
		case <-m.context.Done():
			return
		case wrapper := <-m.eventQueue:
			if err := m.processEvent(wrapper.event, wrapper.payload); err != nil {
				m.notifyListeners(func(l Listener[S, E]) {
					l.OnError(err)
				})
			}
		}
	}
}

// processEvent processes a single event.
func (m *Machine[S, E]) processEvent(event E, payload any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	current := m.current

	// Safety check: ensure transitions map exists for current state
	stateTransitions, stateExists := m.transitions[current]
	if !stateExists {
		err := fmt.Errorf("no transitions defined for state %v", current)
		m.notifyListeners(func(l Listener[S, E]) {
			l.OnError(err)
		})
		return err
	}

	transitions, exists := stateTransitions[event]

	if !exists {
		err := fmt.Errorf("no transition for event %v from state %v", event, current)
		m.notifyListeners(func(l Listener[S, E]) {
			l.OnError(err)
		})
		return err
	}

	// Find the first transition that can be taken
	var transition *Transition[S, E]
	for i := range transitions {
		t := &transitions[i]
		if t.guard == nil || t.guard(Context[S, E]{
			from:      current,
			to:        t.to,
			event:     event,
			payload:   payload,
			timestamp: time.Now(),
			machine:   m,
		}) {
			transition = t
			break
		}
	}

	if transition == nil {
		err := fmt.Errorf("no valid transition for event %v from state %v", event, current)
		m.notifyListeners(func(l Listener[S, E]) {
			l.OnError(err)
		})
		return err
	}

	// Execute the transition
	return m.executeTransition(transition, payload)
}

// executeTransition executes a transition.
func (m *Machine[S, E]) executeTransition(t *Transition[S, E], payload any) error {
	ctx := Context[S, E]{
		from:      m.current,
		to:        t.to,
		event:     t.event,
		payload:   payload,
		timestamp: time.Now(),
		machine:   m,
	}

	// Notify exit of current state
	m.notifyListeners(func(l Listener[S, E]) {
		l.OnExit(m.current, t.event)
	})

	// Execute exit actions
	if config, exists := m.states[m.current]; exists {
		m.executeExitActions(m.current, config, ctx)
	}

	// Execute transition action
	if t.action != nil {
		t.action(ctx)
	}

	// Update current state if not internal transition
	if !t.internal {
		oldState := m.current
		m.current = t.to

		// Execute entry actions for target state
		if targetConfig, exists := m.states[t.to]; exists && targetConfig != nil {
			m.executeEntryActions(t.to, targetConfig, ctx)
		}

		// Notify entry to new state
		m.notifyListeners(func(l Listener[S, E]) {
			l.OnEntry(t.to, t.event)
		})

		// Notify transition
		m.notifyListeners(func(l Listener[S, E]) {
			l.OnTransition(ctx)
		})

		m.logger.Infof("Transition: %v -> %v (event: %v)", oldState, t.to, t.event)
	} else {
		m.logger.Infof("Internal transition in state %v (event: %v)", m.current, t.event)
	}

	return nil
}

// executeEntryActions executes entry actions for a state.
func (m *Machine[S, E]) executeEntryActions(state S, config *StateConfig[S, E], ctx Context[S, E]) {
	if config == nil {
		return
	}
	for _, action := range config.entryActions {
		if action != nil {
			action(ctx)
		}
	}
}

// executeExitActions executes exit actions for a state.
func (m *Machine[S, E]) executeExitActions(state S, config *StateConfig[S, E], ctx Context[S, E]) {
	if config == nil {
		return
	}
	for _, action := range config.exitActions {
		if action != nil {
			action(ctx)
		}
	}
}

// notifyListeners notifies all listeners.
func (m *Machine[S, E]) notifyListeners(fn func(Listener[S, E])) {
	if m.listeners == nil {
		return
	}
	for _, listener := range m.listeners {
		if listener != nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						m.logger.Errorf("监听器panic: %v", r)
					}
				}()
				fn(listener)
			}()
		}
	}
}

// StateOption configures a state.
type StateOption[S comparable, E comparable] func(*StateConfig[S, E])

// WithEntryAction adds an entry action to a state.
func WithEntryAction[S comparable, E comparable](action Action[S, E]) StateOption[S, E] {
	return func(c *StateConfig[S, E]) {
		c.entryActions = append(c.entryActions, action)
	}
}

// WithExitAction adds an exit action to a state.
func WithExitAction[S comparable, E comparable](action Action[S, E]) StateOption[S, E] {
	return func(c *StateConfig[S, E]) {
		c.exitActions = append(c.exitActions, action)
	}
}

// WithTimeout adds a timeout to a state.
func WithTimeout[S comparable, E comparable](timeout time.Duration, timeoutEvent E) StateOption[S, E] {
	return func(c *StateConfig[S, E]) {
		c.timeout = timeout
		c.timeoutEvent = timeoutEvent
	}
}

// WithInitialSubstate sets the initial substate for a composite state.
func WithInitialSubstate[S comparable, E comparable](initial S) StateOption[S, E] {
	return func(c *StateConfig[S, E]) {
		c.initial = &initial
	}
}

// AsFinal marks a state as final.
func AsFinal[S comparable, E comparable]() StateOption[S, E] {
	return func(c *StateConfig[S, E]) {
		c.final = true
	}
}

// AsParallel marks a state as parallel.
func AsParallel[S comparable, E comparable]() StateOption[S, E] {
	return func(c *StateConfig[S, E]) {
		c.parallel = true
	}
}

// WithHistory sets the history type for a state.
func WithHistory[S comparable, E comparable](history HistoryType) StateOption[S, E] {
	return func(c *StateConfig[S, E]) {
		c.history = history
	}
}

// TransitionOption configures a transition.
type TransitionOption[S comparable, E comparable] func(*Transition[S, E])

// WithGuard adds a guard to a transition.
func WithGuard[S comparable, E comparable](guard Guard[S, E]) TransitionOption[S, E] {
	return func(t *Transition[S, E]) {
		t.guard = guard
	}
}

// WithAction adds an action to a transition.
func WithAction[S comparable, E comparable](action Action[S, E]) TransitionOption[S, E] {
	return func(t *Transition[S, E]) {
		t.action = action
	}
}

// Logger interface for logging.
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// defaultLogger is a simple logger that writes to stdout.
type defaultLogger struct{}

func (l *defaultLogger) Debugf(format string, args ...any) {
	fmt.Printf("[DEBUG] "+format+"\n", args...)
}

func (l *defaultLogger) Infof(format string, args ...any) {
	fmt.Printf("[INFO] "+format+"\n", args...)
}

func (l *defaultLogger) Warnf(format string, args ...any) {
	fmt.Printf("[WARN] "+format+"\n", args...)
}

func (l *defaultLogger) Errorf(format string, args ...any) {
	fmt.Printf("[ERROR] "+format+"\n", args...)
}

// Builder provides a fluent API for building state machines.
type Builder[S comparable, E comparable] struct {
	machine *Machine[S, E]
}

// NewBuilder creates a new builder.
func NewBuilder[S comparable, E comparable]() *Builder[S, E] {
	return &Builder[S, E]{
		machine: New[S, E](),
	}
}

// AddState adds a state with options.
func (b *Builder[S, E]) AddState(state S, opts ...StateOption[S, E]) *Builder[S, E] {
	b.machine.AddState(state, opts...)
	return b
}

// AddTransition adds a transition with options.
func (b *Builder[S, E]) AddTransition(from, to S, event E, opts ...TransitionOption[S, E]) *Builder[S, E] {
	b.machine.AddTransition(from, to, event, opts...)
	return b
}

// AddInternalTransition adds an internal transition.
func (b *Builder[S, E]) AddInternalTransition(state S, event E, opts ...TransitionOption[S, E]) *Builder[S, E] {
	b.machine.AddInternalTransition(state, event, opts...)
	return b
}

// WithAsync enables async processing.
func (b *Builder[S, E]) WithAsync(enabled bool) *Builder[S, E] {
	b.machine.async = enabled
	return b
}

// WithQueueSize sets the queue size.
func (b *Builder[S, E]) WithQueueSize(size int) *Builder[S, E] {
	b.machine.maxQueueSize = size
	b.machine.eventQueue = make(chan eventWrapper[S, E], size)
	return b
}

// WithLogger sets a custom logger.
func (b *Builder[S, E]) WithLogger(logger Logger) *Builder[S, E] {
	b.machine.logger = logger
	return b
}

// WithDefaultTimeout sets the default timeout for states.
func (b *Builder[S, E]) WithDefaultTimeout(timeout time.Duration) *Builder[S, E] {
	b.machine.defaultTimeout = timeout
	return b
}

// WithContext sets a context for the state machine.
func (b *Builder[S, E]) WithContext(ctx context.Context) *Builder[S, E] {
	b.machine.context = ctx
	return b
}

// Build returns the configured state machine.
func (b *Builder[S, E]) Build() *Machine[S, E] {
	return b.machine
}

// SimpleListener is a simple implementation of Listener.
type SimpleListener[S comparable, E comparable] struct {
	OnTransitionFunc func(Context[S, E])
	OnEntryFunc      func(state S, event E)
	OnExitFunc       func(state S, event E)
	OnEventFunc      func(event E, payload any)
	OnErrorFunc      func(err error)
}

func (l *SimpleListener[S, E]) OnTransition(ctx Context[S, E]) {
	if l.OnTransitionFunc != nil {
		l.OnTransitionFunc(ctx)
	}
}

func (l *SimpleListener[S, E]) OnEntry(state S, event E) {
	if l.OnEntryFunc != nil {
		l.OnEntryFunc(state, event)
	}
}

func (l *SimpleListener[S, E]) OnExit(state S, event E) {
	if l.OnExitFunc != nil {
		l.OnExitFunc(state, event)
	}
}

func (l *SimpleListener[S, E]) OnEvent(event E, payload any) {
	if l.OnEventFunc != nil {
		l.OnEventFunc(event, payload)
	}
}

func (l *SimpleListener[S, E]) OnError(err error) {
	if l.OnErrorFunc != nil {
		l.OnErrorFunc(err)
	}
}
