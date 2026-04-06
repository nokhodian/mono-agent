package action

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/nokhodian/mono-agent/internal/browser"
	"github.com/rs/zerolog"
)

// ---------------------------------------------------------------------------
// Step & loop definitions (deserialized from action JSON)
// ---------------------------------------------------------------------------

// StepDef represents a single step in an action definition.
type StepDef struct {
	ID            string            `json:"id"`
	Type          string            `json:"type"`
	URL           string            `json:"url,omitempty"`
	Selector      string            `json:"selector,omitempty"`
	XPath         string            `json:"xpath,omitempty"`
	ConfigKey     string            `json:"configKey,omitempty"`
	Alternatives  []string          `json:"alternatives,omitempty"`
	ElementRef    string            `json:"elementRef,omitempty"`
	Value         interface{}       `json:"value,omitempty"`
	Text          string            `json:"text,omitempty"`
	Attribute     string            `json:"attribute,omitempty"`
	Direction     string            `json:"direction,omitempty"`
	Duration      interface{}       `json:"duration,omitempty"`
	Timeout       float64           `json:"timeout,omitempty"`
	HumanLike     bool              `json:"humanLike,omitempty"`
	MethodName    string            `json:"methodName,omitempty"`
	Method        string            `json:"method,omitempty"`
	Args          []interface{}     `json:"args,omitempty"`
	Variable      string            `json:"variable,omitempty"`
	VariableName  string            `json:"variable_name,omitempty"`
	WaitFor       string            `json:"waitFor,omitempty"`
	WaitAfter     string            `json:"waitAfter,omitempty"`
	RaceSelectors map[string]string `json:"raceSelectors,omitempty"`
	Condition     interface{}       `json:"condition,omitempty"`
	Then          []string          `json:"then,omitempty"`
	Else          []string          `json:"else,omitempty"`
	OnError       *ErrorHandlerDef  `json:"onError,omitempty"`
	OnSuccess     *SuccessAction    `json:"onSuccess,omitempty"`
	Set           map[string]interface{} `json:"set,omitempty"`
	Description   string            `json:"description,omitempty"`
	DataSource    string            `json:"dataSource,omitempty"`
	BatchSize     int               `json:"batchSize,omitempty"`
	Increment     string            `json:"increment,omitempty"`
}

// LoopDef defines an iteration over a collection of items, executing a subset
// of the action's steps for each item.
type LoopDef struct {
	ID         string    `json:"id"`
	Iterator   string    `json:"iterator"`
	IndexVar   string    `json:"indexVar"`
	Steps      []string  `json:"steps"`
	OnComplete interface{} `json:"onComplete,omitempty"`
}

// ConditionDef describes a conditional expression for condition steps.
type ConditionDef struct {
	Variable string      `json:"variable"`
	Operator string      `json:"operator"` // "exists", "not_exists", "equals", "not_equals", "greater_than", "contains"
	Value    interface{} `json:"value,omitempty"`
}

// SuccessAction defines what happens when a step succeeds.
type SuccessAction struct {
	Action    string      `json:"action"`   // "set_variable", "increment", "save_data", "update_progress"
	Variable  string      `json:"variable,omitempty"`
	Value     interface{} `json:"value,omitempty"`
	Increment string      `json:"increment,omitempty"`
}

// ---------------------------------------------------------------------------
// Event types
// ---------------------------------------------------------------------------

// ExecutionEvent is emitted during execution to allow external monitoring.
type ExecutionEvent struct {
	Type     string `json:"type"`
	ActionID string `json:"actionId"`
	StepID   string `json:"stepId,omitempty"`
	Index    int    `json:"index,omitempty"`
	Total    int    `json:"total,omitempty"`
	Message  string `json:"message,omitempty"`
}

// ---------------------------------------------------------------------------
// Execution context
// ---------------------------------------------------------------------------

// ExecutionContext holds all mutable state for a single action execution run.
type ExecutionContext struct {
	mu              sync.Mutex
	Variables       map[string]interface{}
	StepResults     map[string]*StepResult
	Elements        map[string]browser.ElementHandle
	Data            map[string]interface{}
	ExtractedItems  []map[string]interface{}
	FailedItems     []FailedItem
	RecursionCounts map[string]int
	CurrentURL      string
}

// NewExecutionContext returns an initialised execution context.
func NewExecutionContext() *ExecutionContext {
	return &ExecutionContext{
		Variables:       make(map[string]interface{}),
		StepResults:     make(map[string]*StepResult),
		Elements:        make(map[string]browser.ElementHandle),
		Data:            make(map[string]interface{}),
		ExtractedItems:  make([]map[string]interface{}, 0),
		FailedItems:     make([]FailedItem, 0),
		RecursionCounts: make(map[string]int),
	}
}

// SetVariable stores a variable in the context.
func (ec *ExecutionContext) SetVariable(name string, value interface{}) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.Variables[name] = value
}

// GetVariable retrieves a variable by name.
func (ec *ExecutionContext) GetVariable(name string) (interface{}, bool) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	v, ok := ec.Variables[name]
	return v, ok
}

// SetStepResult records the result of a step execution.
func (ec *ExecutionContext) SetStepResult(stepID string, result *StepResult) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.StepResults[stepID] = result
}

// GetStepResult retrieves the result of a previously executed step.
func (ec *ExecutionContext) GetStepResult(stepID string) *StepResult {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	return ec.StepResults[stepID]
}

// SetElement stores an ElementHandle reference by name.
func (ec *ExecutionContext) SetElement(name string, elem browser.ElementHandle) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.Elements[name] = elem
}

// GetElement retrieves a previously stored ElementHandle reference.
func (ec *ExecutionContext) GetElement(name string) browser.ElementHandle {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	return ec.Elements[name]
}

// SetData stores an arbitrary value in the data map.
func (ec *ExecutionContext) SetData(key string, value interface{}) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.Data[key] = value
}

// GetData retrieves a value from the data map.
func (ec *ExecutionContext) GetData(key string) (interface{}, bool) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	v, ok := ec.Data[key]
	return v, ok
}

// AddFailedItem appends a failure record.
func (ec *ExecutionContext) AddFailedItem(item FailedItem) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.FailedItems = append(ec.FailedItems, item)
}

// AddExtractedItem appends an extracted data record.
func (ec *ExecutionContext) AddExtractedItem(item map[string]interface{}) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.ExtractedItems = append(ec.ExtractedItems, item)
}

// IncrementRecursion increments and returns the recursion counter for a key.
func (ec *ExecutionContext) IncrementRecursion(key string) int {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.RecursionCounts[key]++
	return ec.RecursionCounts[key]
}

// ---------------------------------------------------------------------------
// Result types
// ---------------------------------------------------------------------------

// FailedItem records a step failure for later reporting.
type FailedItem struct {
	StepID    string
	Error     error
	Timestamp time.Time
	Index     int
}

// StepResult captures the outcome of a single step execution.
type StepResult struct {
	Success bool
	Element browser.ElementHandle
	Data    interface{}
	Error   error
	Abort   bool
	Skip    bool
	Retry   bool
	StepID  string
}

// ExecutionResult is the aggregate outcome of an entire action execution.
type ExecutionResult struct {
	ExtractedItems []map[string]interface{}
	FailedItems    []FailedItem
	TotalProcessed int
	Duration       time.Duration
}

// ---------------------------------------------------------------------------
// Interfaces (to avoid circular imports with storage/config packages)
// ---------------------------------------------------------------------------

// StorageInterface abstracts the database operations needed by the executor.
type StorageInterface interface {
	UpdateActionState(id, state string) error
	UpdateActionReachedIndex(id string, index int) error
	SaveExtractedData(actionID string, items []map[string]interface{}) error
}

// ConfigInterface abstracts the configuration manager.
type ConfigInterface interface {
	GetConfig(social, action, configContext, htmlContent, purpose string, schema map[string]interface{}) (interface{}, error)
}

// BotAdapter is the interface for platform-specific bot methods that can be
// called via the "call_bot_method" step type.
type BotAdapter interface {
	// GetMethodByName returns a callable function for the given method name.
	// The returned function accepts a context and variadic args, returning
	// (result interface{}, err error).
	GetMethodByName(name string) (func(ctx context.Context, args ...interface{}) (interface{}, error), bool)
}

// StorageAction is a flat representation of an action from the storage layer,
// used to avoid a direct dependency on the storage package.
type StorageAction struct {
	ID                string
	CreatedAt         int64
	Title             string
	Type              string
	State             string
	TargetPlatform    string
	ContentSubject    string
	ContentMessage    string
	ContentBlobURLs   string
	Keywords          string
	ReachedIndex      int
	ScheduledDate     string
	StartDate         string
	EndDate           string
	ExecutionInterval int
	CampaignID        string
	Params        map[string]interface{}
}

// ---------------------------------------------------------------------------
// Step handler signature
// ---------------------------------------------------------------------------

// StepHandler is the function signature for individual step implementations.
type StepHandler func(ctx context.Context, step StepDef) (*StepResult, error)

// ---------------------------------------------------------------------------
// ActionExecutor
// ---------------------------------------------------------------------------

// ActionExecutor orchestrates the execution of a loaded action definition
// against a browser page.
type ActionExecutor struct {
	ctx          context.Context
	page         browser.PageInterface
	db           StorageInterface
	configMgr    ConfigInterface
	events       chan<- ExecutionEvent
	botAdapter   BotAdapter
	logger       zerolog.Logger
	execCtx      *ExecutionContext
	resolver     *VariableResolver
	errorHandler *ErrorHandler
	handlers     map[string]StepHandler
	action       *StorageAction
	actionDef    *ActionDef
	startTime    time.Time
}

// NewActionExecutor creates a fully initialised executor. The page must already
// be navigated to the platform's domain (or will be by the first navigate
// step). events may be nil if no external monitoring is needed.
func NewActionExecutor(
	ctx context.Context,
	page browser.PageInterface,
	db StorageInterface,
	configMgr ConfigInterface,
	events chan<- ExecutionEvent,
	botAdapter BotAdapter,
	logger zerolog.Logger,
) *ActionExecutor {
	execCtx := NewExecutionContext()
	ae := &ActionExecutor{
		ctx:          ctx,
		page:         page,
		db:           db,
		configMgr:    configMgr,
		events:       events,
		botAdapter:   botAdapter,
		logger:       logger.With().Str("component", "executor").Logger(),
		execCtx:      execCtx,
		resolver:     NewVariableResolver(execCtx),
		errorHandler: NewErrorHandler(),
		handlers:     make(map[string]StepHandler),
	}
	ae.initHandlers()
	return ae
}

// SetVariable pre-seeds a variable in the execution context before Execute() is
// called. Use this to pass user-supplied data (e.g., selectedListItems) into the
// action pipeline.
func (ae *ActionExecutor) SetVariable(key string, value interface{}) {
	ae.execCtx.SetVariable(key, value)
}

// initHandlers registers all step type handlers in the dispatch map.
func (ae *ActionExecutor) initHandlers() {
	ae.handlers["navigate"] = ae.stepNavigate
	ae.handlers["wait"] = ae.stepWait
	ae.handlers["refresh"] = ae.stepRefresh
	ae.handlers["find_element"] = ae.stepFindElement
	ae.handlers["click"] = ae.stepClick
	ae.handlers["type"] = ae.stepType
	ae.handlers["upload"] = ae.stepUpload
	ae.handlers["scroll"] = ae.stepScroll
	ae.handlers["hover"] = ae.stepHover
	ae.handlers["extract_text"] = ae.stepExtractText
	ae.handlers["extract_attribute"] = ae.stepExtractAttribute
	ae.handlers["extract_multiple"] = ae.stepExtractMultiple
	ae.handlers["condition"] = ae.stepCondition
	ae.handlers["update_progress"] = ae.stepUpdateProgress
	ae.handlers["save_data"] = ae.stepSaveData
	ae.handlers["mark_failed"] = ae.stepMarkFailed
	ae.handlers["log"] = ae.stepLog
	ae.handlers["call_bot_method"] = ae.stepCallBotMethod
	ae.handlers["set_variable"] = ae.stepSetVariable
}

// Execute runs the complete action. It follows four phases:
//  1. Identify which steps belong to loops
//  2. Execute initial (non-loop) steps
//  3. Execute each loop
//  4. Aggregate and return results
func (ae *ActionExecutor) Execute(action *StorageAction) (*ExecutionResult, error) {
	ae.startTime = time.Now()
	ae.action = action

	// Load the action definition from the embedded JSON.
	loader := GetLoader()
	actionDef, err := loader.Load(action.TargetPlatform, action.Type)
	if err != nil {
		return nil, fmt.Errorf("loading action definition: %w", err)
	}
	ae.actionDef = actionDef

	// Seed the execution context with action fields.
	ae.seedVariables(action)

	ae.emitEvent(ExecutionEvent{
		Type:     "action_start",
		ActionID: action.ID,
		Message:  fmt.Sprintf("Starting %s/%s", action.TargetPlatform, action.Type),
	})

	// Update state to RUNNING.
	if ae.db != nil {
		if err := ae.db.UpdateActionState(action.ID, "RUNNING"); err != nil {
			ae.logger.Warn().Err(err).Msg("failed to update action state to RUNNING")
		}
	}

	// Phase 1: Identify all step IDs that are referenced by loops.
	loopStepIDs := make(map[string]bool)
	for _, loop := range actionDef.Loops {
		for _, id := range ae.getAllReferencedIDs(loop, actionDef.Steps) {
			loopStepIDs[id] = true
		}
	}

	// Identify all step IDs referenced only in condition then/else branches.
	// These should not be included in initialSteps — the condition step executes
	// them directly via executeSteps when the branch is taken.
	conditionBranchIDs := make(map[string]bool)
	var collectBranches func(steps []StepDef)
	collectBranches = func(steps []StepDef) {
		for _, step := range steps {
			for _, id := range step.Then {
				conditionBranchIDs[id] = true
			}
			for _, id := range step.Else {
				conditionBranchIDs[id] = true
			}
		}
	}
	collectBranches(actionDef.Steps)

	// Phase 2: Execute initial (non-loop, non-condition-branch) steps in order.
	var initialSteps []StepDef
	for _, step := range actionDef.Steps {
		if !loopStepIDs[step.ID] && !conditionBranchIDs[step.ID] {
			initialSteps = append(initialSteps, step)
		}
	}

	if err := ae.executeSteps(ae.ctx, initialSteps); err != nil {
		if err == ErrAbort {
			ae.logger.Error().Msg("action aborted during initial steps")
			if ae.db != nil {
				_ = ae.db.UpdateActionState(action.ID, "FAILED")
			}
			return ae.buildResult(), err
		}
		ae.logger.Warn().Err(err).Msg("error during initial steps (continuing to loops)")
	}

	// Phase 3: Execute loops.
	for _, loop := range actionDef.Loops {
		if err := ae.executeLoop(ae.ctx, loop, actionDef.Steps); err != nil {
			if err == ErrAbort {
				ae.logger.Error().Str("loopID", loop.ID).Msg("action aborted during loop")
				if ae.db != nil {
					_ = ae.db.UpdateActionState(action.ID, "FAILED")
				}
				return ae.buildResult(), err
			}
			ae.logger.Warn().Err(err).Str("loopID", loop.ID).Msg("loop completed with errors")
		}
	}

	// Phase 4: Mark completed.
	if ae.db != nil {
		_ = ae.db.UpdateActionState(action.ID, "COMPLETED")
	}

	ae.emitEvent(ExecutionEvent{
		Type:     "action_complete",
		ActionID: action.ID,
		Message:  fmt.Sprintf("Completed %s/%s", action.TargetPlatform, action.Type),
	})

	return ae.buildResult(), nil
}

// seedVariables populates the execution context with initial values from the
// storage action.
func (ae *ActionExecutor) seedVariables(action *StorageAction) {
	ae.execCtx.SetVariable("actionId", action.ID)
	ae.execCtx.SetVariable("actionType", action.Type)
	ae.execCtx.SetVariable("platform", action.TargetPlatform)
	ae.execCtx.SetVariable("reachedIndex", action.ReachedIndex)

	if action.ContentMessage != "" {
		ae.execCtx.SetVariable("messageText", action.ContentMessage)
		ae.execCtx.SetVariable("contentMessage", action.ContentMessage)
		ae.execCtx.SetVariable("commentText", action.ContentMessage)
		ae.execCtx.SetVariable("replyText", action.ContentMessage)
		ae.execCtx.SetVariable("text", action.ContentMessage)
	}
	if action.ContentSubject != "" {
		ae.execCtx.SetVariable("messageSubject", action.ContentSubject)
		ae.execCtx.SetVariable("contentSubject", action.ContentSubject)
	}
	if action.Keywords != "" {
		ae.execCtx.SetVariable("keyword", action.Keywords)
		ae.execCtx.SetVariable("keywords", action.Keywords)
		// URL-safe version for embedding in URL templates.
		ae.execCtx.SetVariable("keywordEncoded", url.QueryEscape(action.Keywords))
	}
	if action.ContentBlobURLs != "" {
		ae.execCtx.SetVariable("contentBlobUrls", action.ContentBlobURLs)
	}
	// Seed all custom params as execution variables.
	for k, v := range action.Params {
		ae.execCtx.SetVariable(k, v)
	}
}

// executeSteps runs a slice of steps sequentially. It handles variable
// resolution, error handling, and retries for each step.
func (ae *ActionExecutor) executeSteps(ctx context.Context, steps []StepDef) error {
	for i := 0; i < len(steps); i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		step := steps[i]

		// Resolve templates in the step definition.
		resolved := ae.resolver.ResolveStepDef(step)

		handler, ok := ae.handlers[resolved.Type]
		if !ok {
			ae.logger.Warn().
				Str("stepID", resolved.ID).
				Str("type", resolved.Type).
				Msg("unknown step type, skipping")
			continue
		}

		ae.emitEvent(ExecutionEvent{
			Type:     "step_start",
			ActionID: ae.action.ID,
			StepID:   resolved.ID,
			Index:    i,
			Total:    len(steps),
			Message:  fmt.Sprintf("Executing step %s (%s)", resolved.ID, resolved.Type),
		})

		result, err := handler(ctx, resolved)
		if err != nil || (result != nil && !result.Success) {
			if result == nil {
				result = &StepResult{
					Success: false,
					StepID:  resolved.ID,
					Error:   err,
				}
			}
			result.StepID = resolved.ID

			// Apply error handler if defined.
			handled := ae.errorHandler.Handle(ctx, resolved.OnError, result, ae.execCtx)

			if handled.Abort {
				return ErrAbort
			}
			if handled.Retry {
				// Re-execute the same step.
				i--
				continue
			}
			if handled.Skip {
				skipLog := ae.logger.Debug().Str("stepID", resolved.ID)
				if result != nil && result.Error != nil {
					skipLog = skipLog.Err(result.Error)
				}
				skipLog.Msg("step skipped after error handling")
				ae.execCtx.SetStepResult(resolved.ID, handled)
				continue
			}

			// For "continue" error action, record and move on.
			ae.execCtx.SetStepResult(resolved.ID, handled)
			continue
		}

		// Step succeeded.
		if result != nil {
			result.StepID = resolved.ID
			ae.execCtx.SetStepResult(resolved.ID, result)

			// Handle onSuccess callback.
			if resolved.OnSuccess != nil {
				ae.handleOnSuccess(resolved.OnSuccess)
			}
		}

		// Reset retry counter on success.
		ae.errorHandler.ResetRetries(resolved.ID)

		ae.emitEvent(ExecutionEvent{
			Type:     "step_complete",
			ActionID: ae.action.ID,
			StepID:   resolved.ID,
			Index:    i,
			Total:    len(steps),
		})
	}
	return nil
}

// handleOnSuccess processes the onSuccess callback of a step.
func (ae *ActionExecutor) handleOnSuccess(sa *SuccessAction) {
	switch sa.Action {
	case "set_variable":
		if sa.Variable != "" {
			ae.execCtx.SetVariable(sa.Variable, sa.Value)
		}
	case "increment":
		varName := sa.Increment
		if varName == "" {
			varName = sa.Variable
		}
		if varName != "" {
			val, _ := ae.execCtx.GetVariable(varName)
			switch v := val.(type) {
			case int:
				ae.execCtx.SetVariable(varName, v+1)
			case float64:
				ae.execCtx.SetVariable(varName, v+1)
			default:
				ae.execCtx.SetVariable(varName, 1)
			}
		}
	case "update_progress":
		if sa.Increment != "" {
			val, _ := ae.execCtx.GetVariable(sa.Increment)
			switch v := val.(type) {
			case int:
				ae.execCtx.SetVariable(sa.Increment, v+1)
			case float64:
				ae.execCtx.SetVariable(sa.Increment, v+1)
			default:
				ae.execCtx.SetVariable(sa.Increment, 1)
			}
		}
	case "save_data":
		// Trigger a flush of extracted items.
		if ae.db != nil && ae.action != nil && len(ae.execCtx.ExtractedItems) > 0 {
			ae.execCtx.mu.Lock()
			items := make([]map[string]interface{}, len(ae.execCtx.ExtractedItems))
			copy(items, ae.execCtx.ExtractedItems)
			ae.execCtx.mu.Unlock()
			if err := ae.db.SaveExtractedData(ae.action.ID, items); err != nil {
				ae.logger.Warn().Err(err).Msg("failed to save extracted data on success callback")
			}
		}
	}
}

// executeLoop runs a single loop definition. It resolves the iterator to a
// slice, then executes the loop's steps for each item starting from
// the action's reached index.
func (ae *ActionExecutor) executeLoop(ctx context.Context, loop LoopDef, allSteps []StepDef) error {
	// Resolve the iterator to get the collection to iterate over.
	items := ae.resolveIterator(loop.Iterator)
	if items == nil {
		ae.logger.Warn().
			Str("loopID", loop.ID).
			Str("iterator", loop.Iterator).
			Msg("loop iterator resolved to nil, skipping loop")
		return nil
	}

	collection := toSlice(items)
	if len(collection) == 0 {
		ae.logger.Info().Str("loopID", loop.ID).Msg("loop collection is empty")
		return nil
	}

	// Determine the starting index (for resume support).
	startIdx := 0
	if ae.action != nil && ae.action.ReachedIndex > 0 {
		startIdx = ae.action.ReachedIndex
	}

	loopSteps := ae.getStepsByIDs(allSteps, loop.Steps)

	ae.logger.Info().
		Str("loopID", loop.ID).
		Int("total", len(collection)).
		Int("startIndex", startIdx).
		Int("stepCount", len(loopSteps)).
		Msg("starting loop execution")

	for idx := startIdx; idx < len(collection); idx++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		item := collection[idx]

		// Set loop variables.
		ae.execCtx.SetVariable("item", item)
		ae.execCtx.SetVariable(loop.IndexVar, idx)
		ae.execCtx.SetVariable("loopIndex", idx)
		ae.execCtx.SetVariable("loopTotal", len(collection))

		ae.emitEvent(ExecutionEvent{
			Type:     "loop_iteration",
			ActionID: ae.action.ID,
			StepID:   loop.ID,
			Index:    idx,
			Total:    len(collection),
			Message:  fmt.Sprintf("Loop %s: item %d/%d", loop.ID, idx+1, len(collection)),
		})

		if err := ae.executeSteps(ctx, loopSteps); err != nil {
			if err == ErrAbort {
				return err
			}
			ae.logger.Warn().
				Err(err).
				Int("index", idx).
				Str("loopID", loop.ID).
				Msg("loop iteration failed")
		}

		// Update reached index in storage for resume support.
		// Batch writes: persist every 50 iterations or on the last item to avoid N+1 DB writes.
		if ae.db != nil && ae.action != nil {
			if (idx+1)%50 == 0 || idx == len(collection)-1 {
				if err := ae.db.UpdateActionReachedIndex(ae.action.ID, idx+1); err != nil {
					ae.logger.Warn().Err(err).Int("index", idx+1).Msg("failed to update reached index")
				}
			}
		}
	}

	return nil
}

// resolveIterator resolves the loop iterator path to its underlying value.
func (ae *ActionExecutor) resolveIterator(iteratorPath string) interface{} {
	return ae.resolver.ResolvePath(iteratorPath)
}

// toSlice converts various collection types to []interface{}.
func toSlice(val interface{}) []interface{} {
	switch v := val.(type) {
	case []interface{}:
		return v
	case []string:
		result := make([]interface{}, len(v))
		for i, s := range v {
			result[i] = s
		}
		return result
	case []map[string]interface{}:
		result := make([]interface{}, len(v))
		for i, m := range v {
			result[i] = m
		}
		return result
	default:
		return nil
	}
}

// getAllReferencedIDs collects all step IDs referenced by a loop, including
// those in then/else branches of condition steps.
func (ae *ActionExecutor) getAllReferencedIDs(loop LoopDef, allSteps []StepDef) []string {
	// Build a lookup map once instead of scanning allSteps per ID.
	stepMap := make(map[string]StepDef, len(allSteps))
	for _, step := range allSteps {
		stepMap[step.ID] = step
	}

	seen := make(map[string]bool)
	// Iterative BFS instead of recursion to avoid stack overflow on deeply nested actions.
	queue := make([]string, len(loop.Steps))
	copy(queue, loop.Steps)

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		if step, ok := stepMap[id]; ok {
			if len(step.Then) > 0 {
				queue = append(queue, step.Then...)
			}
			if len(step.Else) > 0 {
				queue = append(queue, step.Else...)
			}
		}
	}

	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	return result
}

// getStepsByIDs returns step definitions from allSteps matching the given IDs,
// preserving the order of the ids slice.
func (ae *ActionExecutor) getStepsByIDs(allSteps []StepDef, ids []string) []StepDef {
	stepMap := make(map[string]StepDef, len(allSteps))
	for _, step := range allSteps {
		stepMap[step.ID] = step
	}

	result := make([]StepDef, 0, len(ids))
	for _, id := range ids {
		if step, ok := stepMap[id]; ok {
			result = append(result, step)
		}
	}
	return result
}

// emitEvent sends an execution event to the events channel. If the channel is
// nil or full, the event is silently dropped.
func (ae *ActionExecutor) emitEvent(event ExecutionEvent) {
	if ae.events == nil {
		return
	}
	select {
	case ae.events <- event:
	default:
		ae.logger.Debug().
			Str("type", event.Type).
			Str("stepID", event.StepID).
			Msg("event channel full, dropping event")
	}
}

// buildResult aggregates the execution context into a final ExecutionResult.
func (ae *ActionExecutor) buildResult() *ExecutionResult {
	ae.execCtx.mu.Lock()
	defer ae.execCtx.mu.Unlock()

	extracted := make([]map[string]interface{}, len(ae.execCtx.ExtractedItems))
	copy(extracted, ae.execCtx.ExtractedItems)

	failed := make([]FailedItem, len(ae.execCtx.FailedItems))
	copy(failed, ae.execCtx.FailedItems)

	return &ExecutionResult{
		ExtractedItems: extracted,
		FailedItems:    failed,
		TotalProcessed: len(extracted) + len(failed),
		Duration:       time.Since(ae.startTime),
	}
}
