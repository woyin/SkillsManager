// cmd/browse_display.go 实现 `sm browse` 的展示层：把抓取到的技能列表以
// 交互选择器（picker）或表格呈现，并把用户选中项转交给 `sm add` 安装。
//
// Input: fmt, os, strings, text/tabwriter, github.com/woyin/skills-manager/internal/picker, golang.org/x/term
// Output: func browsePicker, func browseTable, func formatInstalls, func runAddFromBrowse
// Pos: 控制层-browse命令展示层（交互选择器/表格渲染/安装调用）
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/woyin/skills-manager/internal/picker"
	"golang.org/x/term"
)

func browsePicker(skills []browseSkill) error {
	items := make([]picker.Item, len(skills))
	for i, s := range skills {
		detail := s.Source
		if s.Description != "" {
			detail = s.Description
		}
		if s.Installs > 0 {
			detail = fmt.Sprintf("%s (%s installs)", detail, formatInstalls(s.Installs))
		}
		items[i] = picker.Item{
			Label:  s.Name,
			Detail: detail,
			Value:  fmt.Sprintf("%d", i),
		}
	}

	title := "Browse skills.sh"
	if browseTrending {
		title = "Trending on skills.sh"
	} else if browseHot {
		title = "Hot on skills.sh"
	}

	selected, err := picker.Pick(title+" (enter to install, esc to quit)", items)
	if err != nil {
		return nil
	}

	var idx int
	fmt.Sscanf(selected, "%d", &idx)
	if idx < 0 || idx >= len(skills) {
		return nil
	}

	skill := skills[idx]
	fmt.Printf("\nSelected: %s (%s)\n", skill.Name, skill.Source)
	if skill.Description != "" {
		fmt.Printf("Description: %s\n", skill.Description)
	}
	if skill.URL != "" {
		fmt.Printf("URL: %s\n", skill.URL)
	}
	fmt.Printf("\nInstall with: sm add %s --skill %s\n", skill.Source, skill.Name)

	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Print("\nInstall now? [y/N] ")
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(answer) == "y" || strings.ToLower(answer) == "yes" {
			return runAddFromBrowse(skill.Source, skill.Name)
		}
	}
	return nil
}

func browseTable(skills []browseSkill) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "#\tSKILL\tSOURCE\tINSTALLS\tDESCRIPTION")
	fmt.Fprintln(w, "--\t-----\t------\t--------\t-----------")
	for i, s := range skills {
		desc := s.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		installs := ""
		if s.Installs > 0 {
			installs = formatInstalls(s.Installs)
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", i+1, s.Name, s.Source, installs, desc)
	}
	w.Flush()
	fmt.Printf("\n%d skill(s) found on skills.sh\n", len(skills))
	fmt.Println("Install with: sm install <source> --skill <name> --agent <agent>")
	return nil
}

func formatInstalls(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func runAddFromBrowse(source, skillName string) error {
	installCmd.SetArgs([]string{source, "--skill", skillName, "--yes"})
	return installCmd.Execute()
}
