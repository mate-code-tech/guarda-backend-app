package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type RxNormClient struct {
	client  *http.Client
	baseURL string
}

func NewRxNormClient() *RxNormClient {
	return &RxNormClient{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: "https://rxnav.nlm.nih.gov/REST",
	}
}

func (r *RxNormClient) FindByName(ctx context.Context, name string) (string, error) {
	// Try exact match first
	generic, err := r.exactSearch(ctx, name)
	if err == nil && generic != "" {
		return cleanDrugName(generic), nil
	}

	// Try approximate match
	generic, err = r.approximateSearch(ctx, name)
	if err == nil && generic != "" {
		return cleanDrugName(generic), nil
	}

	return "", fmt.Errorf("not found in RxNorm: %s", name)
}

// cleanDrugName extracts just the base drug name from RxNorm results
// e.g. "aspirin 500 mg oral tablet [aspirina caplets]" → "aspirin"
func cleanDrugName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	// Remove everything after first digit (dose info)
	for i, c := range name {
		if c >= '0' && c <= '9' {
			name = strings.TrimSpace(name[:i])
			break
		}
	}
	// Remove everything in brackets
	if idx := strings.Index(name, "["); idx > 0 {
		name = strings.TrimSpace(name[:idx])
	}
	if idx := strings.Index(name, "("); idx > 0 {
		name = strings.TrimSpace(name[:idx])
	}
	return name
}

func (r *RxNormClient) exactSearch(ctx context.Context, name string) (string, error) {
	u := fmt.Sprintf("%s/rxcui.json?name=%s&search=2", r.baseURL, url.QueryEscape(name))
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		IDGroup struct {
			RxnormID []string `json:"rxnormId"`
		} `json:"idGroup"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.IDGroup.RxnormID) == 0 {
		return "", fmt.Errorf("no match")
	}

	// Get properties to find generic name
	return r.getGenericName(ctx, result.IDGroup.RxnormID[0])
}

func (r *RxNormClient) approximateSearch(ctx context.Context, name string) (string, error) {
	u := fmt.Sprintf("%s/approximateTerm.json?term=%s&maxEntries=3", r.baseURL, url.QueryEscape(name))
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		ApproximateGroup struct {
			Candidate []struct {
				RxCUI string `json:"rxcui"`
				Name  string `json:"name"`
			} `json:"candidate"`
		} `json:"approximateGroup"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.ApproximateGroup.Candidate) == 0 {
		return "", fmt.Errorf("no approximate match")
	}

	return result.ApproximateGroup.Candidate[0].Name, nil
}

func (r *RxNormClient) getGenericName(ctx context.Context, rxcui string) (string, error) {
	u := fmt.Sprintf("%s/rxcui/%s/properties.json", r.baseURL, rxcui)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Properties struct {
			Name string `json:"name"`
		} `json:"properties"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Properties.Name, nil
}
