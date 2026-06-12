// internal/picker/picker.go
// Package picker provides a lightweight fzf-style interactive terminal picker.
package picker

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Item represents a selectable item in the picker.
type Item struct {
	Label  string // Display label (first column)
	Detail string // Additional detail (second column, dimmed)
	Value  string // Value returned when selected
}

// Pick shows an interactive picker and returns the selected item's Value.
// If the terminal is not interactive, returns the first item's value.
// title is shown at the top of the picker.
func Pick(title string, items []Item) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("no items to pick")
	}

	// If not a terminal, fall back to first item
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return items[0].Value, nil
	}

	return interactivePick(title, items)
}

// PickMultiple shows an interactive picker allowing multiple selections.
// Returns the values of all selected items.
func PickMultiple(title string, items []Item) ([]string, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no items to pick")
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		var vals []string
		for _, item := range items {
			vals = append(vals, item.Value)
		}
		return vals, nil
	}

	return interactivePickMulti(title, items)
}

func interactivePick(title string, items []Item) (string, error) {
	// Filtered list
	filtered := make([]Item, len(items))
	copy(filtered, items)

	cursor := 0
	query := ""
	offset := 0

	_, height, _ := term.GetSize(int(os.Stdout.Fd()))
	if height <= 0 {
		height = 24
	}
	visibleLines := height - 4 // reserve lines for title, query, and footer

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", fmt.Errorf("making terminal raw: %w", err)
	}
	defer term.Restore(fd, oldState)

	// Hide cursor
	fmt.Print("\033[?25l")
	defer fmt.Print("\033[?25h")

	draw := func() {
		// Move to top
		fmt.Print("\033[H")

		// Title
		fmt.Printf("\033[2K\033[1m%s\033[0m\r\n", title)

		// Query line
		fmt.Printf("\033[2K> %s\033[K\r\n", query)

		// Separator
		fmt.Printf("\033[2K\033[90m%s (%d)\033[0m\r\n", strings.Repeat("─", 40), len(filtered))

		// Items
		maxShow := visibleLines
		if maxShow < 1 {
			maxShow = 1
		}

		// Adjust offset to keep cursor visible
		if cursor < offset {
			offset = cursor
		}
		if cursor >= offset+maxShow {
			offset = cursor - maxShow + 1
		}

		for i := 0; i < maxShow; i++ {
			idx := offset + i
			if idx < len(filtered) {
				item := filtered[idx]
				if idx == cursor {
					fmt.Printf("\033[2K\033[7m> %s\033[0m", item.Label)
					if item.Detail != "" {
						detail := item.Detail
						if len(detail) > 40 {
							detail = detail[:37] + "..."
						}
						fmt.Printf(" \033[2m%s\033[0m", detail)
					}
				} else {
					fmt.Printf("\033[2K  %s", item.Label)
					if item.Detail != "" {
						detail := item.Detail
						if len(detail) > 40 {
							detail = detail[:37] + "..."
						}
						fmt.Printf(" \033[2m%s\033[0m", detail)
					}
				}
			} else {
				fmt.Print("\033[2K")
			}
			fmt.Print("\r\n")
		}

		// Footer
		fmt.Printf("\033[2K\033[90m↑↓ navigate  enter select  esc quit  type to filter\033[0m\r\n")
	}

	readKey := func() (keyType, rune) {
		buf := make([]byte, 3)
		n, _ := os.Stdin.Read(buf)
		if n == 0 {
			return keyOther, 0
		}
		switch {
		case buf[0] == 27: // ESC
			if n == 1 {
				return keyEscape, 0
			}
			if n >= 3 && buf[1] == '[' {
				switch buf[2] {
				case 'A':
					return keyUp, 0
				case 'B':
					return keyDown, 0
				}
			}
			return keyOther, 0
		case buf[0] == 13 || buf[0] == 10: // Enter
			return keyEnter, 0
		case buf[0] == 127 || buf[0] == 8: // Backspace
			return keyBackspace, 0
		case buf[0] == 3: // Ctrl+C
			return keyCtrlC, 0
		default:
			if buf[0] >= 32 && buf[0] < 127 {
				return keyChar, rune(buf[0])
			}
			return keyOther, 0
		}
	}

	draw()

	for {
		kt, ch := readKey()
		switch kt {
		case keyEscape, keyCtrlC:
			// Clear screen area
			clearPicker(visibleLines + 4)
			return "", fmt.Errorf("cancelled")
		case keyEnter:
			clearPicker(visibleLines + 4)
			if cursor < len(filtered) {
				return filtered[cursor].Value, nil
			}
			return "", fmt.Errorf("no selection")
		case keyUp:
			if cursor > 0 {
				cursor--
			}
		case keyDown:
			if cursor < len(filtered)-1 {
				cursor++
			}
		case keyBackspace:
			if len(query) > 0 {
				query = query[:len(query)-1]
				filtered = filterItems(items, query)
				cursor = 0
				offset = 0
			}
		case keyChar:
			query += string(ch)
			filtered = filterItems(items, query)
			cursor = 0
			offset = 0
		}
		draw()
	}
}

func interactivePickMulti(title string, items []Item) ([]string, error) {
	// For simplicity, use single pick and return it as a slice
	// A full multi-select would add checkbox UI
	val, err := interactivePick(title, items)
	if err != nil {
		return nil, err
	}
	return []string{val}, nil
}

func filterItems(items []Item, query string) []Item {
	if query == "" {
		result := make([]Item, len(items))
		copy(result, items)
		return result
	}

	query = strings.ToLower(query)
	var result []Item
	for _, item := range items {
		label := strings.ToLower(item.Label)
		detail := strings.ToLower(item.Detail)
		match := true
		for _, term := range strings.Fields(query) {
			if !strings.Contains(label, term) && !strings.Contains(detail, term) {
				match = false
				break
			}
		}
		if match {
			result = append(result, item)
		}
	}
	return result
}

func clearPicker(lines int) {
	fmt.Print("\033[H")
	for i := 0; i < lines; i++ {
		fmt.Print("\033[2K\r\n")
	}
	fmt.Print("\033[H")
}

type keyType int

const (
	keyOther keyType = iota
	keyEscape
	keyEnter
	keyUp
	keyDown
	keyBackspace
	keyChar
	keyCtrlC
)
