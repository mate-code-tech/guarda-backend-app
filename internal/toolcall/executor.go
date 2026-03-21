package toolcall

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/guarda/backend/internal/service"
)

type Result struct {
	Name string      `json:"name"`
	Data interface{} `json:"data"`
}

type Executor struct {
	normalizer *service.NormalizerService
	checker    *service.InteractionChecker
}

func NewExecutor(normalizer *service.NormalizerService, checker *service.InteractionChecker) *Executor {
	return &Executor{normalizer: normalizer, checker: checker}
}

func (e *Executor) Execute(ctx context.Context, convID uuid.UUID, calls []service.FunctionCall) ([]Result, error) {
	var results []Result
	seen := make(map[string]bool)

	for _, call := range calls {
		key := fmt.Sprintf("%s:%v", call.Name, call.Args)
		if seen[key] {
			continue
		}
		seen[key] = true

		result, err := e.dispatch(ctx, convID, call)
		if err != nil {
			return nil, fmt.Errorf("executing %s: %w", call.Name, err)
		}
		results = append(results, *result)
	}

	return results, nil
}

func (e *Executor) dispatch(ctx context.Context, convID uuid.UUID, call service.FunctionCall) (*Result, error) {
	switch call.Name {
	case "normalize_medications":
		return e.handleNormalize(ctx, call.Args)
	case "check_interactions":
		return e.handleCheckInteractions(ctx, call.Args)
	default:
		return &Result{Name: call.Name, Data: map[string]string{"error": "unknown function"}}, nil
	}
}

type medResult struct {
	InputName   string `json:"input_name"`
	GenericName string `json:"generic_name"`
}

func (e *Executor) handleNormalize(ctx context.Context, args map[string]interface{}) (*Result, error) {
	medsRaw, ok := args["medications"]
	if !ok {
		return nil, fmt.Errorf("medications arg required")
	}

	medsList, ok := medsRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("medications must be array")
	}

	var medications []medResult
	for _, m := range medsList {
		name, ok := m.(string)
		if !ok {
			continue
		}
		generic, err := e.normalizer.Normalize(ctx, name)
		if err != nil {
			generic = ""
		}
		medications = append(medications, medResult{InputName: name, GenericName: generic})
	}

	return &Result{
		Name: "normalize_medications",
		Data: map[string]interface{}{"medications": medications},
	}, nil
}

func (e *Executor) handleCheckInteractions(ctx context.Context, args map[string]interface{}) (*Result, error) {
	medsRaw, ok := args["medications"]
	if !ok {
		return nil, fmt.Errorf("medications arg required")
	}

	medsList, ok := medsRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("medications must be array")
	}

	var meds []string
	for _, m := range medsList {
		if name, ok := m.(string); ok {
			meds = append(meds, name)
		}
	}

	var results []service.InteractionResult
	for i := 0; i < len(meds); i++ {
		for j := i + 1; j < len(meds); j++ {
			result, err := e.checker.Check(ctx, meds[i], meds[j])
			if err != nil {
				continue
			}
			results = append(results, *result)
		}
	}
	if results == nil {
		results = []service.InteractionResult{}
	}

	return &Result{
		Name: "check_interactions",
		Data: map[string]interface{}{"results": results},
	}, nil
}
