package server

import (
	"reflect"
	"strings"
	"testing"
)

func TestFMReplyChoices(t *testing.T) {
	for _, c := range []struct {
		answer string
		want   []string
	}{
		{"スコアを確認しますか？", robotYesNo},
		{"アーカイブした「新宿御苑前→銀座」があります。復元しますか？", robotYesNo},
		{"Shall I check it?", robotYesNo},
		{"どこに置きますか？", nil},
		{"何を確認しますか？", nil},
		{"荻窪に来ました", nil},
		{"70（-2）です。", nil},
	} {
		if got := robotReplyChoices(c.answer); !reflect.DeepEqual(got, c.want) {
			t.Errorf("robotReplyChoices(%q) = %v, want %v", c.answer, got, c.want)
		}
	}
}

func TestFMLocaleText(t *testing.T) {
	if got := robotLocaleText(""); got != "" {
		t.Errorf("no locale: %q", got)
	}
	if got := robotLocaleText("ja_JP"); !strings.Contains(got, "answer in Japanese") || !strings.Contains(got, "ja_JP") {
		t.Errorf("ja_JP: %q", got)
	}
	if got := robotLocaleText("en-US"); !strings.Contains(got, "answer in English") {
		t.Errorf("en-US: %q", got)
	}
	if got := robotLocaleText("xx"); !strings.Contains(got, "locale xx") {
		t.Errorf("xx: %q", got)
	}
}
