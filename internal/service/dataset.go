package service

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
)

type DrugDictionary struct {
	entries map[string]string // lowercase brand/spanish → INN
}

func NewDrugDictionary(csvPath string) (*DrugDictionary, error) {
	d := &DrugDictionary{entries: make(map[string]string)}

	f, err := os.Open(csvPath)
	if err != nil {
		// If file doesn't exist, use built-in dictionary
		d.loadDefaults()
		return d, nil
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading drug dictionary CSV: %w", err)
	}

	for i, record := range records {
		if i == 0 {
			continue // skip header
		}
		if len(record) >= 2 {
			key := strings.ToLower(strings.TrimSpace(record[0]))
			val := strings.ToLower(strings.TrimSpace(record[1]))
			d.entries[key] = val
		}
	}
	return d, nil
}

func (d *DrugDictionary) Lookup(name string) (string, bool) {
	key := strings.ToLower(removeAccents(strings.TrimSpace(name)))
	// Exact match
	if v, ok := d.entries[key]; ok {
		return v, true
	}
	// Fuzzy match: find best match with edit distance <= 2
	bestMatch := ""
	bestDist := 3 // threshold
	for k, v := range d.entries {
		dist := levenshtein(key, k)
		if dist < bestDist {
			bestDist = dist
			bestMatch = v
		}
	}
	if bestMatch != "" {
		return bestMatch, true
	}
	// Prefix match: "ibu" → "ibuprofeno", "busca" → "buscapina"
	for k, v := range d.entries {
		if len(key) >= 3 && strings.HasPrefix(k, key) {
			return v, true
		}
	}
	return "", false
}

func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (d *DrugDictionary) loadDefaults() {
	defaults := map[string]string{
		"tafirol":        "acetaminophen",
		"paracetamol":    "acetaminophen",
		"bayaspirina":    "aspirin",
		"aspirina":       "aspirin",
		"ibuevanol":      "ibuprofen",
		"ibuprofeno":     "ibuprofen",
		"amoxidal":       "amoxicillin",
		"amoxicilina":    "amoxicillin",
		"diclofenac":     "diclofenac",
		"voltaren":       "diclofenac",
		"omeprazol":      "omeprazole",
		"losec":          "omeprazole",
		"metformina":     "metformin",
		"glucophage":     "metformin",
		"enalapril":      "enalapril",
		"lotrial":        "enalapril",
		"atenolol":       "atenolol",
		"losartan":       "losartan",
		"amlodipina":     "amlodipine",
		"norvasc":        "amlodipine",
		"simvastatina":   "simvastatin",
		"atorvastatina":  "atorvastatin",
		"lipitor":        "atorvastatin",
		"clonazepam":     "clonazepam",
		"rivotril":       "clonazepam",
		"alprazolam":     "alprazolam",
		"alplax":         "alprazolam",
		"lorazepam":      "lorazepam",
		"diazepam":       "diazepam",
		"valium":         "diazepam",
		"ranitidina":     "ranitidine",
		"cefalexina":     "cephalexin",
		"azitromicina":   "azithromycin",
		"ciprofloxacina": "ciprofloxacin",
		"metoclopramida": "metoclopramide",
		"reliveran":      "metoclopramide",
		"dexametasona":   "dexamethasone",
		"prednisona":     "prednisone",
		"levotiroxina":   "levothyroxine",
		"t4":             "levothyroxine",
		"warfarina":      "warfarin",
		"sildenafil":     "sildenafil",
		"tramadol":       "tramadol",
		"codeina":        "codeine",
		"morfina":        "morphine",
		"dipirona":       "metamizole",
		"novalgina":      "metamizole",
		"sertal":         "propinox",
		"buscapina":      "hyoscine",
	}
	for k, v := range defaults {
		d.entries[k] = v
	}
}

func removeAccents(s string) string {
	replacer := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
		"ü", "u", "ñ", "n",
		"Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u",
	)
	return replacer.Replace(s)
}

type InteractionDataset struct {
	entries map[string]string // "drug_a|drug_b" → description
}

func NewInteractionDataset(csvPath string) (*InteractionDataset, error) {
	d := &InteractionDataset{entries: make(map[string]string)}

	f, err := os.Open(csvPath)
	if err != nil {
		return d, nil // empty dataset if file not found
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading interaction dataset CSV: %w", err)
	}

	for i, record := range records {
		if i == 0 {
			continue
		}
		if len(record) >= 3 {
			a := strings.ToLower(strings.TrimSpace(record[0]))
			b := strings.ToLower(strings.TrimSpace(record[1]))
			desc := strings.TrimSpace(record[2])
			// Store both directions
			d.entries[a+"|"+b] = desc
			d.entries[b+"|"+a] = desc
		}
	}
	return d, nil
}

func (d *InteractionDataset) Lookup(drugA, drugB string) (string, bool) {
	key := strings.ToLower(drugA) + "|" + strings.ToLower(drugB)
	if desc, ok := d.entries[key]; ok {
		return desc, true
	}
	return "", false
}
