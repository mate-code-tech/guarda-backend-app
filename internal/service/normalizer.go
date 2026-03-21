package service

import (
	"context"
	"fmt"
)

type NormalizerService struct {
	dictionary *DrugDictionary
	rxnorm     *RxNormClient
	ai         *AIService
}

func NewNormalizerService(dict *DrugDictionary, rxnorm *RxNormClient, ai *AIService) *NormalizerService {
	return &NormalizerService{dictionary: dict, rxnorm: rxnorm, ai: ai}
}

func (s *NormalizerService) Normalize(ctx context.Context, name string) (string, error) {
	// Level 1: Local dictionary
	if generic, ok := s.dictionary.Lookup(name); ok {
		return generic, nil
	}

	// Level 2: RxNorm API
	if generic, err := s.rxnorm.FindByName(ctx, name); err == nil {
		return generic, nil
	}

	// Level 3: AI fallback
	if s.ai != nil {
		generic, err := s.ai.NormalizeMedication(ctx, name)
		if err == nil {
			return generic, nil
		}
	}

	return "", fmt.Errorf("could not normalize: %s", name)
}
