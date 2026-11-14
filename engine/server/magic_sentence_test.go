package server

import "testing"

func TestCompileMagic(t *testing.T) {
	cases := []struct {
		tmpl, q  string
		ok       bool
		from, to string
	}{
		{"<from:station>から<to:station>", "荻窪から中野坂上", true, "荻窪", "中野坂上"},
		{"<from:station>から<to:station>", "  荻窪 から 中野坂上 ", true, "荻窪", "中野坂上"},
		{"<from:station>から<to:station>まで", "荻窪から中野坂上まで", true, "荻窪", "中野坂上"},
		{"<from:station>から<to:station>まで", "荻窪から中野坂上", false, "", ""},
		{"<from:station>から<to:station>", "中野の天気", false, "", ""},
	}
	for _, c := range cases {
		re, slots := compileMagic(c.tmpl)
		if re == nil || len(slots) != 2 || slots[0].param != "from" || slots[0].subtype != "station" {
			t.Fatalf("%q: compiled %v %v", c.tmpl, re, slots)
		}
		m := re.FindStringSubmatch(c.q)
		if (m != nil) != c.ok {
			t.Fatalf("%q on %q: match=%v want %v", c.tmpl, c.q, m != nil, c.ok)
		}
		if c.ok && (m[1] != c.from || m[2] != c.to) {
			t.Fatalf("%q on %q: got %q %q", c.tmpl, c.q, m[1], m[2])
		}
	}
	if re, _ := compileMagic("ただの文"); re != nil {
		t.Fatal("a sentence with no slots should not compile")
	}
}
