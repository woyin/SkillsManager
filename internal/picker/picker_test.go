package picker

import (
	"testing"
)

func TestFilterItems(t *testing.T) {
	items := []Item{
		{Label: "tdd", Detail: "test driven"},
		{Label: "grilling", Detail: "stress test ideas"},
		{Label: "code-review", Detail: "review changes"},
	}
	cases := []struct {
		query string
		want  int
	}{
		{"", 3},
		{"tdd", 1},                       // 命中 label
		{"ideas", 1},                     // 命中 detail
		{"tdd ideas", 0},                 // AND 语义：跨字段需全命中，无项同时含两者
		{"review changes", 1},            // 两个词都在 code-review 的 detail
		{"zzz", 0},                       // 无命中
	}
	for _, c := range cases {
		got := filterItems(items, c.query)
		if len(got) != c.want {
			t.Errorf("filterItems(query=%q) = %d items, want %d", c.query, len(got), c.want)
		}
	}
}

func TestFilterItemsEmptyQueryReturnsCopy(t *testing.T) {
	items := []Item{{Label: "a"}, {Label: "b"}}
	got := filterItems(items, "")
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	// 修改返回值不影响原切片（确认为拷贝）。
	got[0].Label = "mutated"
	if items[0].Label != "a" {
		t.Errorf("filterItems should return a copy; original was mutated")
	}
}

func TestTruncateDetail(t *testing.T) {
	// truncateDetail: len > 40 时截为前 37 + "..."，否则原样返回。
	cases := []struct {
		in, want string
	}{
		{"short", "short"},
		{"1234567890123456789012345678901234567890", "1234567890123456789012345678901234567890"},                 // 正好 40，不截断
		{"12345678901234567890123456789012345678901", "1234567890123456789012345678901234567..."},                // 41，截断
		{"12345678901234567890123456789012345678901234567890", "1234567890123456789012345678901234567..."},       // 50，截断
	}
	for _, c := range cases {
		if got := truncateDetail(c.in); got != c.want {
			t.Errorf("truncateDetail(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
