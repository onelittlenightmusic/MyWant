package types

import (
	"context"
	"fmt"
	"strings"
)

// SuggestElementName asks `claude --print` for a short (<=10 rune) name
// describing the given HTML snippet. Fails open: callers should treat a
// non-nil error (or empty name) as "no suggestion" and keep whatever
// heuristic name the caller already has — never surface this as a
// user-facing error.
func SuggestElementName(ctx context.Context, html string) (string, error) {
	prompt := buildSuggestNamePrompt(html)
	out, err := runClaudeDirect(ctx, prompt)
	if err != nil {
		return "", err
	}
	name := firstLine(out)
	name = strings.Trim(name, " \t\n\"'「」『』")
	return truncateRunes(name, 10), nil
}

func buildSuggestNamePrompt(html string) string {
	return fmt.Sprintf(`以下はWebページの要素のHTMLスニペットです。この要素を表す、日本語UIのラベルとして自然な非常に短い名前を1つだけ考えてください。
制約:
- 出力は名前の文字列のみ。説明・記号・引用符・改行は含めないこと。
- 10文字以内(日本語の場合は10文字、英数字でも10文字以内)。
- 要素のrole/属性/表示テキストから最も妥当な意味を推測すること。

HTML:
%s`, html)
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
