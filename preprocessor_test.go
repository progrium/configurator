package main

import "testing"

const (
	json_preprocess = `{
		"tree": 3,
		"other_thing": [ 1, 3, 2 ],
		"preprocess_this": {
			"$value": "/one",
			"default": "nope"
		},
		"and_this": {
			"$value": "/two"
		},
		"but_this_doesn't_exist": {
			"$value": "non_existing_path"
		},
		"also_three": {
			"$value": "/three"
		}
	}`
)

func TestPreprocessor(t *testing.T) {
	p := &Preprocessor{}

	store := NewTestStore()

	config, err := NewConfig(store, "", "", "", "")
	if err != nil {
		t.Fatalf("failed to make config: %v", err)
	}

	loadBuiltinMacros(p, store, config)

	preprocess := &JsonTree{}

	input := []byte(json_preprocess)
	err = preprocess.Load(input)
	if err != nil {
		t.Fatalf("failed to load input to preprocess, %v", err)
	}

	result := p.Process(preprocess)

	if preprocess_this := result.Get("/preprocess_this"); preprocess_this != "1" {
		t.Fatalf("preprocess_this did not preprocess right: %v", preprocess_this)
	}

	if and_this := result.Get("/and_this"); and_this != "2" {
		t.Fatalf("and_this did not preprocess right: %v", and_this)
	}

	if btde := result.Get("/but_this_doesn't_exist"); btde != "" {
		t.Fatalf("but_this_doesn't_exist did not preprocess right: %v", btde)
	}

	if also_three := result.Get("/also_three"); also_three != "3" {
		t.Fatalf("also_three did not preprocess right: %v", also_three)
	}
}
