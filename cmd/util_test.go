package cmd

import "testing"

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 60, "short"},                         // 未超长，原样
		{"", 60, ""},                                   // 空
		{"1234567890123456789012345678901234567890123456789012345678901234567890", 60, "123456789012345678901234567890123456789012345678901234567" + "..."}, // ASCII 超长：保留前 57
		{"中文描述测试", 60, "中文描述测试"},               // 中文短串原样
		{"一二三四五六七八九十一二三四五六七八九十一二三四五六七八九十一二三四五六七八九十一", 20, "一二三四五六七八九十一二三四五六七" + "..."}, // 中文超长：rune 计数保留前 17
		{"abc", 2, "abc"},                              // n<=3 不截断（省略号放不下）
		{"abcdef", 6, "abcdef"},                        // 恰好等于 n，不截断
		{"abcdefg", 6, "abc..."},                       // 超 1 字符
	}
	for _, c := range cases {
		if got := truncate(c.in, c.n); got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}
