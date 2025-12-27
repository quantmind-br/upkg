package ui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/manifoldco/promptui"
)

// ConfirmPrompt asks a yes/no confirmation question
func ConfirmPrompt(label string) (bool, error) {
	prompt := promptui.Prompt{
		Label:     label,
		IsConfirm: true,
	}

	result, err := prompt.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrAbort) {
			return false, fmt.Errorf("operation cancelled by user")
		}
		return false, err
	}

	// promptui returns "y" for yes
	return result == "y", nil
}

// SelectPrompt presents a list of options for selection
func SelectPrompt(label string, items []string) (int, string, error) {
	prompt := promptui.Select{
		Label: label,
		Items: items,
	}

	index, result, err := prompt.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrAbort) {
			return -1, "", fmt.Errorf("selection cancelled by user")
		}
		return -1, "", err
	}

	return index, result, nil
}

// InputPrompt asks for text input with optional validation
func InputPrompt(label string, defaultValue string, validate func(string) error) (string, error) {
	prompt := promptui.Prompt{
		Label:    label,
		Default:  defaultValue,
		Validate: validate,
	}

	result, err := prompt.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrAbort) {
			return "", fmt.Errorf("input cancelled by user")
		}
		return "", err
	}

	return result, nil
}

// SelectPromptWithDetails presents a list with additional details
type SelectOption struct {
	Label  string
	Detail string
	Value  string
}

// SelectPromptDetailed presents options with details
func SelectPromptDetailed(label string, options []SelectOption) (int, SelectOption, error) {
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}?",
		Active:   "▸ {{ .Label | cyan }} ({{ .Detail | faint }})",
		Inactive: "  {{ .Label | faint }} ({{ .Detail | faint }})",
		Selected: "▸ {{ .Label | green }}",
	}

	prompt := promptui.Select{
		Label:     label,
		Items:     options,
		Templates: templates,
	}

	index, _, err := prompt.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrAbort) {
			return -1, SelectOption{}, fmt.Errorf("selection cancelled by user")
		}
		return -1, SelectOption{}, err
	}

	return index, options[index], nil
}

// MultiSelectPrompt presents a checkbox-style multi-select list using survey
// Users can toggle items with Space and confirm with Enter
func MultiSelectPrompt(label string, items []string) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}

	var selected []string

	prompt := &survey.MultiSelect{
		Message:  label,
		Options:  items,
		PageSize: minInt(15, len(items)),
		Filter: func(filterValue string, optValue string, _ int) bool {
			if filterValue == "" {
				return true
			}
			return fuzzy.MatchNormalizedFold(strings.TrimSpace(filterValue), optValue)
		},
	}

	err := survey.AskOne(prompt, &selected, survey.WithKeepFilter(true))
	if err != nil {
		return nil, fmt.Errorf("selection cancelled by user")
	}

	return selected, nil
}

// MultiSelectPromptLegacy presents a multi-select list using the old sequential selection method
// Kept for backwards compatibility if needed
func MultiSelectPromptLegacy(label string, items []string) ([]string, error) {
	selected := make([]string, 0)
	availableItems := make([]string, len(items)+1)
	copy(availableItems, items)
	availableItems[len(items)] = "[Done - Finish selection]"

	for {
		currentItems := make([]string, len(availableItems))
		copy(currentItems, availableItems)

		prompt := promptui.Select{
			Label: label + " (select multiple, choose 'Done' when finished)",
			Items: currentItems,
			Size:  minInt(10, len(currentItems)),
			Searcher: func(input string, index int) bool {
				if index < 0 || index >= len(currentItems) {
					return false
				}
				item := currentItems[index]
				if input == "" {
					return true
				}
				return fuzzy.MatchNormalizedFold(strings.TrimSpace(input), item)
			},
		}

		index, result, err := prompt.Run()
		if err != nil {
			if errors.Is(err, promptui.ErrAbort) {
				return nil, fmt.Errorf("selection cancelled by user")
			}
			return nil, err
		}

		// Check if user is done
		if index == len(availableItems)-1 {
			break
		}

		// Add to selected if not already chosen
		alreadySelected := false
		for _, existing := range selected {
			if existing == result {
				alreadySelected = true
				break
			}
		}
		if !alreadySelected {
			selected = append(selected, result)
		}

		// Remove from available
		availableItems = append(availableItems[:index], availableItems[index+1:]...)

		if len(availableItems) == 1 { // Only "Done" left
			break
		}
	}

	return selected, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// PasswordPrompt asks for password input (masked)
func PasswordPrompt(label string) (string, error) {
	prompt := promptui.Prompt{
		Label: label,
		Mask:  '*',
	}

	result, err := prompt.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrAbort) {
			return "", fmt.Errorf("input cancelled by user")
		}
		return "", err
	}

	return result, nil
}

// ValidateNonEmpty validates that input is not empty
func ValidateNonEmpty(input string) error {
	if len(input) == 0 {
		return errors.New("input cannot be empty")
	}
	return nil
}

// ValidateMinLength validates minimum length
func ValidateMinLength(minLen int) func(string) error {
	return func(input string) error {
		if len(input) < minLen {
			return fmt.Errorf("input must be at least %d characters", minLen)
		}
		return nil
	}
}

// ValidateMaxLength validates maximum length
func ValidateMaxLength(maxLen int) func(string) error {
	return func(input string) error {
		if len(input) > maxLen {
			return fmt.Errorf("input must be at most %d characters", maxLen)
		}
		return nil
	}
}

// ConfirmDangerousAction asks for confirmation with a warning
func ConfirmDangerousAction(action string, target string) (bool, error) {
	PrintWarning("You are about to %s: %s", action, target)
	PrintWarning("This action cannot be undone!")
	fmt.Println()

	return ConfirmPrompt(fmt.Sprintf("Are you sure you want to %s", action))
}

// ConfirmWithDefault asks for confirmation with a default value
func ConfirmWithDefault(label string, defaultYes bool) (bool, error) {
	var defaultStr string
	if defaultYes {
		defaultStr = "Y/n"
	} else {
		defaultStr = "y/N"
	}

	prompt := promptui.Prompt{
		Label:     fmt.Sprintf("%s [%s]", label, defaultStr),
		IsConfirm: true,
		Default:   "",
	}

	result, err := prompt.Run()
	if err != nil {
		// User pressed enter with no input - use default
		if errors.Is(err, promptui.ErrAbort) {
			return false, fmt.Errorf("operation cancelled")
		}
		// Empty input means use default
		if result == "" {
			return defaultYes, nil
		}
		return defaultYes, nil
	}

	return result == "y" || result == "Y", nil
}
