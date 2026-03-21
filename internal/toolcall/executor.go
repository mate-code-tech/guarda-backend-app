package toolcall

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/guarda/backend/internal/repository"
	"github.com/guarda/backend/internal/service"
)

type Result struct {
	Name string      `json:"name"`
	Data interface{} `json:"data"`
}

type Executor struct {
	normalizer *service.NormalizerService
	checker    *service.InteractionChecker
	guestRepo  *repository.GuestRepo
}

func NewExecutor(normalizer *service.NormalizerService, checker *service.InteractionChecker, guestRepo *repository.GuestRepo) *Executor {
	return &Executor{normalizer: normalizer, checker: checker, guestRepo: guestRepo}
}

func (e *Executor) Execute(ctx context.Context, convID uuid.UUID, guestID uuid.UUID, calls []service.FunctionCall) ([]Result, error) {
	var results []Result
	seen := make(map[string]bool)

	for _, call := range calls {
		key := fmt.Sprintf("%s:%v", call.Name, call.Args)
		if seen[key] {
			continue
		}
		seen[key] = true

		result, err := e.dispatch(ctx, convID, guestID, call)
		if err != nil {
			return nil, fmt.Errorf("executing %s: %w", call.Name, err)
		}
		results = append(results, *result)
	}

	return results, nil
}

func (e *Executor) dispatch(ctx context.Context, convID uuid.UUID, guestID uuid.UUID, call service.FunctionCall) (*Result, error) {
	switch call.Name {
	case "normalize_medications":
		return e.handleNormalize(ctx, call.Args)
	case "check_interactions":
		return e.handleCheckInteractions(ctx, call.Args)
	case "save_guest_profile":
		return e.handleSaveGuestProfile(ctx, guestID, call.Args)
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

func (e *Executor) handleSaveGuestProfile(ctx context.Context, guestID uuid.UUID, args map[string]interface{}) (*Result, error) {
	var name *string
	var age *int
	var conditions []string
	var allergies []string
	var consultationReason *string
	var isForSelf *bool

	if v, ok := args["name"].(string); ok && v != "" {
		name = &v
	}
	if v, ok := args["age"].(float64); ok {
		a := int(v)
		age = &a
	}
	if v, ok := args["conditions"].([]interface{}); ok {
		for _, c := range v {
			if s, ok := c.(string); ok {
				conditions = append(conditions, s)
			}
		}
	}
	if v, ok := args["allergies"].([]interface{}); ok {
		for _, a := range v {
			if s, ok := a.(string); ok {
				allergies = append(allergies, s)
			}
		}
	}
	if v, ok := args["consultation_reason"].(string); ok && v != "" {
		consultationReason = &v
	}
	if v, ok := args["is_for_self"].(bool); ok {
		isForSelf = &v
	}

	err := e.guestRepo.UpdateProfile(ctx, guestID, name, age, conditions, allergies, consultationReason, isForSelf)
	if err != nil {
		return nil, fmt.Errorf("saving guest profile: %w", err)
	}

	saved := map[string]interface{}{"status": "saved"}
	if name != nil {
		saved["name"] = *name
	}
	if age != nil {
		saved["age"] = *age
	}
	if len(conditions) > 0 {
		saved["conditions"] = conditions
	}
	if len(allergies) > 0 {
		saved["allergies"] = allergies
	}
	if consultationReason != nil {
		saved["consultation_reason"] = *consultationReason
	}
	if isForSelf != nil {
		saved["is_for_self"] = *isForSelf
	}

	return &Result{
		Name: "save_guest_profile",
		Data: saved,
	}, nil
}
