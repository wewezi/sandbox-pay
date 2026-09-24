package money

import "testing"

// Port of server/src/lib/money.spec.ts
func TestMoneyHelpers(t *testing.T) {
	t.Run("yuanStringToCents/converts yuan string to cents", func(t *testing.T) {
		cases := []struct {
			in   string
			want int64
		}{
			{"10.00", 1000},
			{"0.01", 1},
			{"99.99", 9999},
		}
		for _, c := range cases {
			if got := YuanStringToCents(c.in); got != c.want {
				t.Errorf("YuanStringToCents(%q) = %d, want %d", c.in, got, c.want)
			}
		}
	})

	t.Run("yuanStringToCents/converts yuan number to cents", func(t *testing.T) {
		// TS 里 yuanStringToCents 同时收 string 和 number；Go 拆成两个入口：
		// 字符串走 YuanStringToCents，数字走 YuanToCents。
		if got := YuanStringToCents("10"); got != 1000 {
			t.Errorf("YuanStringToCents(\"10\") = %d, want 1000", got)
		}
		if got := YuanToCents(10); got != 1000 {
			t.Errorf("YuanToCents(10) = %d, want 1000", got)
		}
		if got := YuanToCents(0.01); got != 1 {
			t.Errorf("YuanToCents(0.01) = %d, want 1", got)
		}
	})

	t.Run("yuanStringToCents/handles null/undefined/empty", func(t *testing.T) {
		// Go 里没有 null/undefined —— 零值就是空串，一并覆盖纯空白串。
		if got := YuanStringToCents(""); got != 0 {
			t.Errorf("YuanStringToCents(\"\") = %d, want 0", got)
		}
		if got := YuanStringToCents("   "); got != 0 {
			t.Errorf("YuanStringToCents(\"   \") = %d, want 0", got)
		}
	})

	t.Run("yuanStringToCents/rounds correctly to avoid floating-point drift", func(t *testing.T) {
		cases := []struct {
			in   string
			want int64
		}{
			{"0.10", 10},
			{"0.29", 29},
			// JS: Math.round(100.5) = 100 —— 1.005*100 在 float64 里是
			// 100.49999…，Go 的 math.Round 同样得到 100。
			{"1.005", 100},
		}
		for _, c := range cases {
			if got := YuanStringToCents(c.in); got != c.want {
				t.Errorf("YuanStringToCents(%q) = %d, want %d", c.in, got, c.want)
			}
		}
	})

	t.Run("toYuan/converts cents to yuan", func(t *testing.T) {
		cases := []struct {
			in   int64
			want float64
		}{
			{1000, 10},
			{1, 0.01},
			{9999, 99.99},
		}
		for _, c := range cases {
			if got := ToYuan(c.in); got != c.want {
				t.Errorf("ToYuan(%d) = %v, want %v", c.in, got, c.want)
			}
		}
	})

	t.Run("toYuan/handles null/undefined", func(t *testing.T) {
		// TS 的 null/undefined → 0 对应 Go 的零值 0。
		if got := ToYuan(0); got != 0 {
			t.Errorf("ToYuan(0) = %v, want 0", got)
		}
	})

	t.Run("toYuanString/returns fixed-2 string", func(t *testing.T) {
		cases := []struct {
			in   int64
			want string
		}{
			{1000, "10.00"},
			{1, "0.01"},
			{0, "0.00"},
		}
		for _, c := range cases {
			if got := ToYuanString(c.in); got != c.want {
				t.Errorf("ToYuanString(%d) = %q, want %q", c.in, got, c.want)
			}
		}
	})

	t.Run("toYuanString/handles null", func(t *testing.T) {
		// TS 的 null → "0.00" 对应 Go 的零值 0。
		if got := ToYuanString(0); got != "0.00" {
			t.Errorf("ToYuanString(0) = %q, want \"0.00\"", got)
		}
	})
}
