package cmd

import "testing"

func TestSplitQueries(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"k3", "5.3-flash"}, []string{"k3", "5.3-flash"}},
		{[]string{"5.3 flash"}, []string{"5.3 flash"}},
		{[]string{"k3, 5.3-flash"}, []string{"k3", "5.3-flash"}},
		{[]string{"", " , "}, nil},
	}
	for _, tc := range cases {
		got := splitQueries(tc.args)
		if len(got) != len(tc.want) {
			t.Fatalf("splitQueries(%q) = %q, want %q", tc.args, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("query[%d] = %q, want %q", i, got[i], tc.want[i])
			}
		}
	}
}

func TestPageSlice(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6, 7}
	if got := pageSlice(items, 1, 3); len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Errorf("page 1: got %v", got)
	}
	if got := pageSlice(items, 3, 3); len(got) != 1 || got[0] != 7 {
		t.Errorf("last page: got %v", got)
	}
	if got := pageSlice(items, 4, 3); len(got) != 0 {
		t.Errorf("beyond range: got %v", got)
	}
	if got := pageSlice(items, 2, 0); len(got) != 7 {
		t.Errorf("limit 0 = all: got %v", got)
	}
}

func TestPageCount(t *testing.T) {
	cases := []struct{ n, limit, want int }{
		{0, 50, 1}, {7, 3, 3}, {9, 3, 3}, {10, 3, 4}, {5, 0, 1},
	}
	for _, tc := range cases {
		if got := pageCount(tc.n, tc.limit); got != tc.want {
			t.Errorf("pageCount(%d, %d) = %d, want %d", tc.n, tc.limit, got, tc.want)
		}
	}
}

func TestShown(t *testing.T) {
	if got := shown(137, 50, 1); got != 50 {
		t.Errorf("page 1: got %d", got)
	}
	if got := shown(137, 50, 3); got != 37 {
		t.Errorf("last page: got %d", got)
	}
	if got := shown(137, 50, 9); got != 0 {
		t.Errorf("beyond range: got %d", got)
	}
	if got := shown(137, 0, 2); got != 137 {
		t.Errorf("limit 0 = all: got %d", got)
	}
}
