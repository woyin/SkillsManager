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

	var result string
	var pickErr error
	err := withRawTerminal(func() error {
		v := newPickerView(title, items, false)
		v.draw()
		for {
			kt, ch := readKey()
			done, err := v.handleKey(kt, ch, func(val string) { result = val })
			if done {
				pickErr = err
				return nil
			}
		}
	})
	if err != nil {
		return "", err
	}
	return result, pickErr
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

	var result []string
	var pickErr error
	err := withRawTerminal(func() error {
		v := newPickerView(title, items, true)
		v.draw()
		for {
			kt, ch := readKey()
			done, err := v.handleKeyMulti(kt, ch, func(vals []string) { result = vals })
			if done {
				pickErr = err
				return nil
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return result, pickErr
}

// pickerView 持有交互循环的全部可变状态，单选与多选共用同一绘制/过滤引擎。
type pickerView struct {
	title        string
	items        []Item // 原始全量列表
	multi        bool   // 多选模式：显示复选框、支持空格/a 键
	filtered     []Item // 当前过滤后的可见列表
	filterIdx    []int  // filtered[i] 在 items 中的原始下标（多选用）
	cursor       int    // 光标在 filtered 中的位置
	offset       int    // 可见窗口起始行
	query        string // 过滤关键词
	visibleLines int    // 窗口内可渲染的行数
	selected     map[int]bool
}

// newPickerView 初始化视图：全量可见、光标置顶，并探测终端高度。
func newPickerView(title string, items []Item, multi bool) *pickerView {
	v := &pickerView{
		title:    title,
		items:    items,
		multi:    multi,
		filtered: append([]Item(nil), items...),
	}
	if multi {
		v.selected = make(map[int]bool)
		v.filterIdx = make([]int, len(items))
		for i := range items {
			v.filterIdx[i] = i
		}
	}
	// 终端高度决定可见行数；获取失败时回退到 24。
	_, height, _ := term.GetSize(int(os.Stdout.Fd()))
	if height <= 0 {
		height = 24
	}
	v.visibleLines = height - 4 // 为标题、查询行、底栏预留
	return v
}

// handleKey 处理单选模式的一次按键。返回 done=true 表示交互结束，
// err 非 nil 表示失败（取消/无选中），否则选中值已交给 emit。
func (v *pickerView) handleKey(kt keyType, ch rune, emit func(string)) (bool, error) {
	switch kt {
	case keyEscape, keyCtrlC:
		clearPicker(v.visibleLines + 4)
		return true, fmt.Errorf("cancelled")
	case keyEnter:
		clearPicker(v.visibleLines + 4)
		if v.cursor >= len(v.filtered) {
			return true, fmt.Errorf("no selection")
		}
		emit(v.filtered[v.cursor].Value)
		return true, nil
	case keyUp:
		if v.cursor > 0 {
			v.cursor--
		}
	case keyDown:
		if v.cursor < len(v.filtered)-1 {
			v.cursor++
		}
	case keyBackspace:
		if len(v.query) > 0 {
			v.query = v.query[:len(v.query)-1]
			// 单选：过滤变化后光标回到顶部。
			v.applyQuery(true)
		}
	case keyChar:
		v.query += string(ch)
		v.applyQuery(true)
	}
	v.draw()
	return false, nil
}

// handleKeyMulti 处理多选模式的一次按键。返回 done=true 表示交互结束，
// err 非 nil 表示失败（取消/空选），否则全部选中值已交给 emit。
func (v *pickerView) handleKeyMulti(kt keyType, ch rune, emit func([]string)) (bool, error) {
	switch kt {
	case keyEscape, keyCtrlC:
		clearPicker(v.visibleLines + 4)
		return true, fmt.Errorf("cancelled")
	case keyEnter:
		clearPicker(v.visibleLines + 4)
		vals := v.selectedValues()
		if len(vals) == 0 {
			return true, fmt.Errorf("no items selected")
		}
		emit(vals)
		return true, nil
	case keyUp:
		if v.cursor > 0 {
			v.cursor--
		}
	case keyDown:
		if v.cursor < len(v.filtered)-1 {
			v.cursor++
		}
	case keyBackspace:
		if len(v.query) > 0 {
			v.query = v.query[:len(v.query)-1]
			// 多选：过滤变化后光标保持在合法位置。
			v.applyQuery(false)
		}
	case keyChar:
		switch ch {
		case ' ':
			v.toggleCurrent()
		case 'a':
			v.toggleAllFiltered()
		default:
			v.query += string(ch)
			v.applyQuery(false)
		}
	}
	v.draw()
	return false, nil
}

// applyQuery 按当前 query 重建过滤集。resetCursor 为 true 时光标回到顶部
// （单选语义），否则钳制在合法范围内（多选语义）。
func (v *pickerView) applyQuery(resetCursor bool) {
	if v.multi {
		v.filtered, v.filterIdx = filterItemsWithIdx(v.items, v.query)
	} else {
		v.filtered = filterItems(v.items, v.query)
	}
	switch {
	case resetCursor:
		v.cursor = 0
	case v.cursor > len(v.filtered)-1:
		v.cursor = len(v.filtered) - 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
	v.offset = 0
}

// toggleCurrent 切换光标项的选中状态（多选）。
func (v *pickerView) toggleCurrent() {
	if len(v.filtered) > 0 {
		idx := v.filterIdx[v.cursor]
		v.selected[idx] = !v.selected[idx]
	}
}

// toggleAllFiltered 全选/全不选当前过滤集（多选的 a 键）。
func (v *pickerView) toggleAllFiltered() {
	flip := !v.allFilteredSelected()
	for _, idx := range v.filterIdx {
		v.selected[idx] = flip
	}
}

// allFilteredSelected 判断当前过滤集是否非空且全部已选。
func (v *pickerView) allFilteredSelected() bool {
	for _, idx := range v.filterIdx {
		if !v.selected[idx] {
			return false
		}
	}
	return len(v.filterIdx) > 0
}

// selectedValues 按 items 原始顺序返回全部选中项的 Value。
func (v *pickerView) selectedValues() []string {
	var vals []string
	for i, it := range v.items {
		if v.selected[i] {
			vals = append(vals, it.Value)
		}
	}
	return vals
}

// maxShow 返回窗口内可显示的行数（至少 1）。
func (v *pickerView) maxShow() int {
	if v.visibleLines < 1 {
		return 1
	}
	return v.visibleLines
}

// adjustScroll 滚动 offset，使光标始终落在可见窗口内。
func (v *pickerView) adjustScroll() {
	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+v.maxShow() {
		v.offset = v.cursor - v.maxShow() + 1
	}
}

// draw 全量重绘选择器界面。
func (v *pickerView) draw() {
	// 光标回到左上角。
	fmt.Print("\033[H")
	// 标题、查询行、分隔行（含计数）。
	fmt.Printf("\033[2K\033[1m%s\033[0m\r\n", v.title)
	fmt.Printf("\033[2K> %s\033[K\r\n", v.query)
	fmt.Printf("\033[2K\033[90m%s %s\033[0m\r\n", strings.Repeat("─", 40), v.counterLabel())

	v.adjustScroll()
	for i := 0; i < v.maxShow(); i++ {
		v.drawRow(v.offset + i)
	}

	// 底栏：操作提示。
	fmt.Printf("\033[2K\033[90m%s\033[0m\r\n", v.footer())
}

// counterLabel 返回分隔行右侧的计数标签：单选 (可见数)，多选 (已选/可见)。
func (v *pickerView) counterLabel() string {
	if !v.multi {
		return fmt.Sprintf("(%d)", len(v.filtered))
	}
	picked := 0
	for _, idx := range v.filterIdx {
		if v.selected[idx] {
			picked++
		}
	}
	return fmt.Sprintf("(%d/%d)", picked, len(v.filtered))
}

// footer 返回底栏操作提示，随模式变化。
func (v *pickerView) footer() string {
	if v.multi {
		return "↑↓ navigate  space toggle  a all  enter confirm  esc quit"
	}
	return "↑↓ navigate  enter select  esc quit  type to filter"
}

// drawRow 渲染 filtered[row] 一行；row 越界时渲染空行补齐窗口。
func (v *pickerView) drawRow(row int) {
	if row >= len(v.filtered) {
		fmt.Print("\033[2K\r\n")
		return
	}
	item := v.filtered[row]
	// 前缀：光标行反显高亮，其余行缩进两格。
	prefix := "  "
	if row == v.cursor {
		prefix = "\033[7m> "
	}
	fmt.Print("\033[2K", prefix)
	if v.multi {
		mark := "[ ]"
		if v.selected[v.filterIdx[row]] {
			mark = "[x]"
		}
		fmt.Printf("%s %s", mark, item.Label)
	} else {
		fmt.Print(item.Label)
	}
	if prefix != "  " {
		fmt.Print("\033[0m")
	}
	if item.Detail != "" {
		fmt.Printf(" \033[2m%s\033[0m", truncateDetail(item.Detail))
	}
	fmt.Print("\r\n")
}

// withRawTerminal 把终端切到 raw 模式执行 run，结束后恢复状态与光标。
func withRawTerminal(run func() error) error {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("making terminal raw: %w", err)
	}
	defer term.Restore(fd, oldState)

	// 隐藏光标，退出时恢复。
	fmt.Print("\033[?25l")
	defer fmt.Print("\033[?25h")

	return run()
}

// readKey 从标准输入读取一次按键并归类。
func readKey() (keyType, rune) {
	buf := make([]byte, 3)
	n, _ := os.Stdin.Read(buf)
	if n == 0 {
		return keyOther, 0
	}
	switch {
	case buf[0] == 27: // ESC 序列
		return decodeEscape(buf, n)
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

// decodeEscape 解析 ESC 起始的转义序列（方向键等）。
func decodeEscape(buf []byte, n int) (keyType, rune) {
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
	filtered, _ := filterItemsWithIdx(items, query)
	return filtered
}

// filterItemsWithIdx 过滤并同步返回结果项在 items 中的原始下标，
// 供多选模式把过滤位置映射回全量列表的选中键。
func filterItemsWithIdx(items []Item, query string) ([]Item, []int) {
	if query == "" {
		idx := make([]int, len(items))
		for i := range items {
			idx[i] = i
		}
		return append([]Item(nil), items...), idx
	}

	query = strings.ToLower(query)
	var result []Item
	var idx []int
	for i, item := range items {
		if itemMatches(item, query) {
			result = append(result, item)
			idx = append(idx, i)
		}
	}
	return result, idx
}

// itemMatches 判断 item 是否命中全部空格分隔的关键词（AND 语义）。
func itemMatches(item Item, query string) bool {
	label := strings.ToLower(item.Label)
	detail := strings.ToLower(item.Detail)
	for _, term := range strings.Fields(query) {
		if !strings.Contains(label, term) && !strings.Contains(detail, term) {
			return false
		}
	}
	return true
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
