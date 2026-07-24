// Package picker 提供轻量、fzf 风格的交互式终端选择器。
//
// 不依赖任何外部库（除 golang.org/x/term 用于原始模式控制），支持：
//   - 实时过滤（输入即过滤）；
//   - 上下方向键导航；
//   - 回车选中、Esc / Ctrl+C 取消。
//
// 当标准输入不是 TTY 时，Pick 返回第一项，PickMultiple 返回全部。
//
// Input: fmt, os, strings, golang.org/x/term
// Output: type Item, func Pick, func PickMultiple
// Pos: UI层-交互选择器
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package picker

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Item 表示选择器中一条可选项。
type Item struct {
	Label  string // 显示标签（第一列）
	Detail string // 附加细节（第二列，灰显）
	Value  string // 选中时返回的值
}

// Pick 弹出交互式选择器，返回选中项的 Value。
// 非交互终端时返回第一项的 Value。
func Pick(title string, items []Item) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("no items to pick")
	}

	// 非 TTY：退化返回第一项。
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return items[0].Value, nil
	}

	return interactivePick(title, items)
}

// PickMultiple 弹出带复选框的多选 UI：空格切换、a 全选/全不选、回车确认。
// 返回所有选中项的 Value；空选返回错误，取消（Esc/Ctrl+C）返回 "cancelled"。
// 非交互终端时返回全部项的 Value。
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

// interactivePick 实现单选交互循环。
func interactivePick(title string, items []Item) (string, error) {
	// filtered 是当前过滤后的可见列表（始终从 items 派生）。
	filtered := make([]Item, len(items))
	copy(filtered, items)

	cursor := 0
	query := ""
	offset := 0

	// 终端高度决定可见行数；获取失败时回退到 24。
	_, height, _ := term.GetSize(int(os.Stdout.Fd()))
	if height <= 0 {
		height = 24
	}
	visibleLines := height - 4 // 为标题、查询行、底栏预留

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", fmt.Errorf("making terminal raw: %w", err)
	}
	defer term.Restore(fd, oldState)

	// 隐藏光标，退出时恢复。
	fmt.Print("\033[?25l")
	defer fmt.Print("\033[?25h")

	draw := func() {
		// 光标回到左上角。
		fmt.Print("\033[H")

		// 标题
		fmt.Printf("\033[2K\033[1m%s\033[0m\r\n", title)
		// 查询行
		fmt.Printf("\033[2K> %s\033[K\r\n", query)
		// 分隔行
		fmt.Printf("\033[2K\033[90m%s (%d)\033[0m\r\n", strings.Repeat("─", 40), len(filtered))

		maxShow := visibleLines
		if maxShow < 1 {
			maxShow = 1
		}

		// 维持光标可见：超出窗口则滚动 offset。
		if cursor < offset {
			offset = cursor
		}
		if cursor >= offset+maxShow {
			offset = cursor - maxShow + 1
		}

		// 绘制可见项。
		for i := 0; i < maxShow; i++ {
			idx := offset + i
			if idx < len(filtered) {
				item := filtered[idx]
				detail := truncateDetail(item.Detail)
				if idx == cursor {
					fmt.Printf("\033[2K\033[7m> %s\033[0m", item.Label)
					if item.Detail != "" {
						fmt.Printf(" \033[2m%s\033[0m", detail)
					}
				} else {
					fmt.Printf("\033[2K  %s", item.Label)
					if item.Detail != "" {
						fmt.Printf(" \033[2m%s\033[0m", detail)
					}
				}
			} else {
				fmt.Print("\033[2K")
			}
			fmt.Print("\r\n")
		}

		// 底栏：操作提示。
		fmt.Printf("\033[2K\033[90m↑↓ navigate  enter select  esc quit  type to filter\033[0m\r\n")
	}

	readKey := func() (keyType, rune) {
		buf := make([]byte, 3)
		n, _ := os.Stdin.Read(buf)
		if n == 0 {
			return keyOther, 0
		}
		switch {
		case buf[0] == 27: // ESC 序列
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
			// 清屏后返回"已取消"错误。
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

// interactivePickMulti 弹出带复选框的多选 UI：空格切换、a 全选/全不选、
// 回车确认。selected 以 items 原始下标为键，过滤/导航不影响已选项。
// 返回所有选中项的 Value；空选时返回错误（与"取消"语义区分）。
func interactivePickMulti(title string, items []Item) ([]string, error) {
	filtered := make([]Item, len(items))
	copy(filtered, items)

	cursor := 0
	query := ""
	offset := 0
	selected := make(map[int]bool) // 键为 items 原始下标

	_, height, _ := term.GetSize(int(os.Stdout.Fd()))
	if height <= 0 {
		height = 24
	}
	visibleLines := height - 4

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("making terminal raw: %w", err)
	}
	defer term.Restore(fd, oldState)

	fmt.Print("\033[?25l")
	defer fmt.Print("\033[?25h")

	// filterIdx[i] = filtered[i] 在 items 中的原始下标。
	filterIdx := make([]int, len(items))
	for i := range items {
		filterIdx[i] = i
	}

	draw := func() {
		fmt.Print("\033[H")
		fmt.Printf("\033[2K\033[1m%s\033[0m\r\n", title)
		fmt.Printf("\033[2K> %s\033[K\r\n", query)

		// 选中计数。
		picked := 0
		for _, idx := range filterIdx {
			if selected[idx] {
				picked++
			}
		}
		fmt.Printf("\033[2K\033[90m%s (%d/%d)\033[0m\r\n", strings.Repeat("─", 40), picked, len(filtered))

		maxShow := visibleLines
		if maxShow < 1 {
			maxShow = 1
		}
		if cursor < offset {
			offset = cursor
		}
		if cursor >= offset+maxShow {
			offset = cursor - maxShow + 1
		}

		for i := 0; i < maxShow; i++ {
			idx := offset + i
			if idx < len(filtered) {
				origIdx := filterIdx[idx]
				item := filtered[idx]
				mark := "[ ]"
				if selected[origIdx] {
					mark = "[x]"
				}
				detail := truncateDetail(item.Detail)
				if idx == cursor {
					fmt.Printf("\033[2K\033[7m> %s %s\033[0m", mark, item.Label)
					if item.Detail != "" {
						fmt.Printf(" \033[2m%s\033[0m", detail)
					}
				} else {
					fmt.Printf("\033[2K  %s %s", mark, item.Label)
					if item.Detail != "" {
						fmt.Printf(" \033[2m%s\033[0m", detail)
					}
				}
			} else {
				fmt.Print("\033[2K")
			}
			fmt.Print("\r\n")
		}

		fmt.Printf("\033[2K\033[90m↑↓ navigate  space toggle  a all  enter confirm  esc quit\033[0m\r\n")
	}

	// recomputeFilter 重建 filtered / filterIdx，并保持 cursor 合法。
	recomputeFilter := func() {
		filtered = filterItems(items, query)
		filterIdx = filterIdx[:0]
		for i, it := range items {
			for _, f := range filtered {
				if f == it {
					filterIdx = append(filterIdx, i)
					break
				}
			}
		}
		if cursor > len(filtered)-1 {
			cursor = len(filtered) - 1
		}
		if cursor < 0 {
			cursor = 0
		}
		offset = 0
	}

	// allFilteredSelected 判断当前过滤集是否全部已选。
	allFilteredSelected := func() bool {
		for _, idx := range filterIdx {
			if !selected[idx] {
				return false
			}
		}
		return len(filterIdx) > 0
	}

	readKey := func() (keyType, rune) {
		buf := make([]byte, 3)
		n, _ := os.Stdin.Read(buf)
		if n == 0 {
			return keyOther, 0
		}
		switch {
		case buf[0] == 27:
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
		case buf[0] == 13 || buf[0] == 10:
			return keyEnter, 0
		case buf[0] == 127 || buf[0] == 8:
			return keyBackspace, 0
		case buf[0] == 3:
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
			clearPicker(visibleLines + 4)
			return nil, fmt.Errorf("cancelled")
		case keyEnter:
			clearPicker(visibleLines + 4)
			var vals []string
			for i, it := range items {
				if selected[i] {
					vals = append(vals, it.Value)
				}
			}
			if len(vals) == 0 {
				return nil, fmt.Errorf("no items selected")
			}
			return vals, nil
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
				recomputeFilter()
			}
		case keyChar:
			switch ch {
			case ' ':
				if len(filtered) > 0 {
					origIdx := filterIdx[cursor]
					selected[origIdx] = !selected[origIdx]
				}
			case 'a':
				flip := !allFilteredSelected()
				for _, idx := range filterIdx {
					selected[idx] = flip
				}
			default:
				query += string(ch)
				recomputeFilter()
			}
		}
		draw()
	}
}

// truncateDetail 把 detail 截断到 40 字符（含省略号）。
func truncateDetail(detail string) string {
	if len(detail) > 40 {
		return detail[:37] + "..."
	}
	return detail
}

// filterItems 按 query 过滤 items。query 为空时原样返回（拷贝）。
// 多关键词以空格分隔，需全部命中（AND 语义）。
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

// clearPicker 清空选择器占用的 lines 行区域，光标回到左上角。
func clearPicker(lines int) {
	fmt.Print("\033[H")
	for i := 0; i < lines; i++ {
		fmt.Print("\033[2K\r\n")
	}
	fmt.Print("\033[H")
}

// keyType 是按键的分类枚举。
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
