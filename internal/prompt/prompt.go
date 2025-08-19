package prompt

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// Prompter handles interactive CLI prompts
type Prompter struct {
	reader *bufio.Reader
}

// New creates a new Prompter instance
func New() *Prompter {
	return &Prompter{
		reader: bufio.NewReader(os.Stdin),
	}
}

// Input prompts for text input with optional validation
func (p *Prompter) Input(question string, validator func(string) error) string {
	for {
		fmt.Printf("%s ", question)
		input, err := p.reader.ReadString('\n')
		if err != nil {
			fmt.Printf("❌ Error reading input: %v\n", err)
			continue
		}
		
		input = strings.TrimSpace(input)
		
		if validator != nil {
			if err := validator(input); err != nil {
				fmt.Printf("❌ %v Please try again.\n", err)
				continue
			}
		}
		
		return input
	}
}

// InputWithDefault prompts for text input with a default value
func (p *Prompter) InputWithDefault(question, defaultValue string, validator func(string) error) string {
	if defaultValue != "" {
		question = fmt.Sprintf("%s (default: %s)", question, defaultValue)
	}
	
	for {
		fmt.Printf("%s: ", question)
		input, err := p.reader.ReadString('\n')
		if err != nil {
			fmt.Printf("❌ Error reading input: %v\n", err)
			continue
		}
		
		input = strings.TrimSpace(input)
		
		// Use default if empty
		if input == "" && defaultValue != "" {
			input = defaultValue
		}
		
		if validator != nil {
			if err := validator(input); err != nil {
				fmt.Printf("❌ %v Please try again.\n", err)
				continue
			}
		}
		
		return input
	}
}

// Confirm prompts for yes/no confirmation
func (p *Prompter) Confirm(question string, defaultYes bool) bool {
	defaultStr := "y/N"
	if defaultYes {
		defaultStr = "Y/n"
	}
	
	for {
		fmt.Printf("%s (%s): ", question, defaultStr)
		input, err := p.reader.ReadString('\n')
		if err != nil {
			fmt.Printf("❌ Error reading input: %v\n", err)
			continue
		}
		
		input = strings.TrimSpace(strings.ToLower(input))
		
		if input == "" {
			return defaultYes
		}
		
		switch input {
		case "y", "yes", "true", "1":
			return true
		case "n", "no", "false", "0":
			return false
		default:
			fmt.Printf("❌ Please enter 'y' for yes or 'n' for no.\n")
		}
	}
}

// Select prompts for single choice from options
func (p *Prompter) Select(question string, options []string) int {
	fmt.Println(question)
	for i, option := range options {
		fmt.Printf("  %d) %s\n", i+1, option)
	}
	
	for {
		fmt.Printf("Enter choice (1-%d): ", len(options))
		input, err := p.reader.ReadString('\n')
		if err != nil {
			fmt.Printf("❌ Error reading input: %v\n", err)
			continue
		}
		
		choice, err := strconv.Atoi(strings.TrimSpace(input))
		if err != nil || choice < 1 || choice > len(options) {
			fmt.Printf("❌ Please enter a number between 1 and %d\n", len(options))
			continue
		}
		
		return choice - 1
	}
}

// SelectWithDefault prompts for single choice with a default option
func (p *Prompter) SelectWithDefault(question string, options []string, defaultIndex int) int {
	fmt.Println(question)
	for i, option := range options {
		prefix := "  "
		if i == defaultIndex {
			prefix = "* "
		}
		fmt.Printf("%s%d) %s\n", prefix, i+1, option)
	}
	
	for {
		fmt.Printf("Enter choice (1-%d, default: %d): ", len(options), defaultIndex+1)
		input, err := p.reader.ReadString('\n')
		if err != nil {
			fmt.Printf("❌ Error reading input: %v\n", err)
			continue
		}
		
		input = strings.TrimSpace(input)
		
		// Use default if empty
		if input == "" {
			return defaultIndex
		}
		
		choice, err := strconv.Atoi(input)
		if err != nil || choice < 1 || choice > len(options) {
			fmt.Printf("❌ Please enter a number between 1 and %d\n", len(options))
			continue
		}
		
		return choice - 1
	}
}

// MultiSelect prompts for multiple choices (space-separated indices)
func (p *Prompter) MultiSelect(question string, options []string) []int {
	fmt.Println(question)
	for i, option := range options {
		fmt.Printf("  %d) %s\n", i+1, option)
	}
	
	for {
		fmt.Printf("Enter choices (space-separated, e.g., '1 3 5'): ")
		input, err := p.reader.ReadString('\n')
		if err != nil {
			fmt.Printf("❌ Error reading input: %v\n", err)
			continue
		}
		
		input = strings.TrimSpace(input)
		if input == "" {
			return []int{}
		}
		
		parts := strings.Fields(input)
		choices := make([]int, 0, len(parts))
		valid := true
		
		for _, part := range parts {
			choice, err := strconv.Atoi(part)
			if err != nil || choice < 1 || choice > len(options) {
				fmt.Printf("❌ Invalid choice '%s'. Please enter numbers between 1 and %d\n", part, len(options))
				valid = false
				break
			}
			choices = append(choices, choice-1)
		}
		
		if valid {
			return choices
		}
	}
}

// Password prompts for password input (hidden)
func (p *Prompter) Password(question string) string {
	fmt.Printf("%s: ", question)
	password, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println() // Add newline after password input
	
	if err != nil {
		fmt.Printf("❌ Error reading password: %v\n", err)
		return ""
	}
	
	return string(password)
}

// ShowSummary displays a formatted summary
func (p *Prompter) ShowSummary(title string, items map[string]string) {
	fmt.Printf("\n📋 %s\n", title)
	fmt.Println(strings.Repeat("=", len(title)+3))
	
	maxKeyLen := 0
	for key := range items {
		if len(key) > maxKeyLen {
			maxKeyLen = len(key)
		}
	}
	
	for key, value := range items {
		fmt.Printf("  %-*s: %s\n", maxKeyLen, key, value)
	}
	fmt.Println()
}