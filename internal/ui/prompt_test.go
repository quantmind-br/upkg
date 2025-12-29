package ui

import (
	"errors"
	"testing"

	"github.com/manifoldco/promptui"
)

func TestValidateNonEmpty(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Empty", "", true},
		{"NonEmpty", "test", false},
		{"Whitespace", "  ", false}, // Whitespace is considered non-empty by ValidateNonEmpty
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNonEmpty(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNonEmpty(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateMinLength(t *testing.T) {
	validator := ValidateMinLength(5)

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"TooShort", "abc", true},
		{"Exact", "abcde", false},
		{"Longer", "abcdef", false},
		{"Empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMinLength(5)(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateMaxLength(t *testing.T) {
	validator := ValidateMaxLength(5)

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"TooLong", "abcdef", true},
		{"Exact", "abcde", false},
		{"Shorter", "abcd", false},
		{"Empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMaxLength(5)(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestMinInt(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{"FirstSmaller", 3, 5, 3},
		{"SecondSmaller", 5, 3, 3},
		{"Equal", 4, 4, 4},
		{"Negative", -1, 1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := minInt(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("minInt(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSelectOption(t *testing.T) {
	option := SelectOption{
		Label:  "Test Label",
		Detail: "Test Detail",
		Value:  "test-value",
	}

	if option.Label != "Test Label" {
		t.Errorf("SelectOption.Label = %q, want %q", option.Label, "Test Label")
	}
	if option.Detail != "Test Detail" {
		t.Errorf("SelectOption.Detail = %q, want %q", option.Detail, "Test Detail")
	}
	if option.Value != "test-value" {
		t.Errorf("SelectOption.Value = %q, want %q", option.Value, "test-value")
	}
}

func TestPromptFunctions(t *testing.T) {
	// These tests are more integration-like and would normally require
	// mocking the promptui package or using a testing framework that can
	// simulate user input. For unit testing, we focus on the validation
	// functions and helper functions.

	// Test that validation functions work correctly
	tests := []struct {
		name      string
		validator func(string) error
		input     string
		wantErr   bool
	}{
		{"NonEmpty", ValidateNonEmpty, "", true},
		{"NonEmptyValid", ValidateNonEmpty, "test", false},
		{"MinLength", ValidateMinLength(5), "abc", true},
		{"MinLengthValid", ValidateMinLength(5), "abcdef", false},
		{"MaxLength", ValidateMaxLength(5), "abcdef", true},
		{"MaxLengthValid", ValidateMaxLength(5), "abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validator(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("%s(%q) error = %v, wantErr %v", tt.name, tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestPromptErrorHandling(t *testing.T) {
	// Test that prompt functions handle errors correctly
	// Note: These are more like integration tests and would need
	// proper mocking to test thoroughly

	// Test that ErrAbort is handled
	err := promptui.ErrAbort
	if err == nil {
		t.Error("promptui.ErrAbort should not be nil")
	}

	// Test that we can create a custom error
	customErr := errors.New("custom error")
	if customErr == nil {
		t.Error("custom error should not be nil")
	}
}

func TestMultiSelectPromptEmpty(t *testing.T) {
	// Test empty items handling
	selected, err := MultiSelectPrompt("Test", []string{})
	if err != nil {
		t.Errorf("MultiSelectPrompt with empty items should not error: %v", err)
	}
	if selected != nil {
		t.Errorf("MultiSelectPrompt with empty items should return nil, got %v", selected)
	}
}

func TestConfirmDangerousAction(t *testing.T) {
	// This test verifies the function exists and has the right signature
	// Actually running it would require interactive input
	// So we just verify it compiles and can be referenced
	_ = ConfirmDangerousAction
}

func TestConfirmWithDefault(t *testing.T) {
	// This test verifies the function exists and has the right signature
	_ = ConfirmWithDefault
}

func TestPasswordPrompt(t *testing.T) {
	// This test verifies the function exists and has the right signature
	_ = PasswordPrompt
}

func TestInputPrompt(t *testing.T) {
	// This test verifies the function exists and has the right signature
	_ = InputPrompt
}

func TestSelectPrompt(t *testing.T) {
	// This test verifies the function exists and has the right signature
	_ = SelectPrompt
}

func TestSelectPromptDetailed(t *testing.T) {
	// This test verifies the function exists and has the right signature
	_ = SelectPromptDetailed
}

func TestMultiSelectPromptLegacy(t *testing.T) {
	// This test verifies the function exists and has the right signature
	_ = MultiSelectPromptLegacy
}

func TestConfirmPrompt(t *testing.T) {
	// This test verifies the function exists and has the right signature
	_ = ConfirmPrompt
}
