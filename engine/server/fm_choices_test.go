package server

import (
	"reflect"
	"testing"
)

func TestFMReplyChoices(t *testing.T) {
	for _, c := range []struct {
		answer string
		want   []string
	}{
		{"スコアを確認しますか？", fmYesNo},
		{"アーカイブした「新宿御苑前→銀座」があります。復元しますか？", fmYesNo},
		{"Shall I check it?", fmYesNo},
		{"どこに置きますか？", nil},
		{"何を確認しますか？", nil},
		{"荻窪に来ました", nil},
		{"70（-2）です。", nil},
	} {
		if got := fmReplyChoices(c.answer); !reflect.DeepEqual(got, c.want) {
			t.Errorf("fmReplyChoices(%q) = %v, want %v", c.answer, got, c.want)
		}
	}
}
