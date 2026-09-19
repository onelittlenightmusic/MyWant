package commands

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// A cell, as somebody says one.
//
// "(5, 0)", "5,0", "5 0" — the board prints the first, a person types the
// second, and anything relaying a question passes back whatever it read. They
// all name the same cell, so all of them are accepted rather than one of them
// being correct.
//
// Given as one argument on purpose: a flag taking "x,y" survives a negative
// coordinate (--at -3,0) where two positional numbers do not, since a parser
// reading flags anywhere sees -3 as a bundle of short flags.
func parseCell(text string) (x, y int, given bool, err error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, 0, false, nil
	}
	cleaned := strings.NewReplacer("(", "", ")", "", "（", "", "）", "", "、", ",", "，", ",").Replace(trimmed)
	fields := strings.FieldsFunc(cleaned, func(r rune) bool { return r == ',' || r == ' ' || r == '　' })
	if len(fields) != 2 {
		return 0, 0, false, fmt.Errorf("a cell is two numbers, like 5,0 — got %q", trimmed)
	}
	x, err = strconv.Atoi(strings.TrimSpace(fields[0]))
	if err != nil {
		return 0, 0, false, fmt.Errorf("%q is not a cell: %q is not a whole number", trimmed, fields[0])
	}
	y, err = strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil {
		return 0, 0, false, fmt.Errorf("%q is not a cell: %q is not a whole number", trimmed, fields[1])
	}
	return x, y, true, nil
}

// parseParamFlags reads --param key=value into the map a want is created with.
//
// Values arrive as text and a want's parameters are not all text — a timeout is
// a number, a switch is a boolean — so each one is read as what it looks like.
// Only unambiguous forms convert: "5" is a number, "true" is a boolean, and
// "Nakano" is a name. Anything quoted stays a string, which is how you say that
// the value "2" really is the two-character town.
func parseParamFlags(cmd *cobra.Command) (map[string]any, error) {
	pairs, _ := cmd.Flags().GetStringArray("param")
	if len(pairs) == 0 {
		return nil, nil
	}
	params := make(map[string]any, len(pairs))
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, fmt.Errorf("--param takes key=value, got %q", pair)
		}
		params[key] = parseParamValue(strings.TrimSpace(value))
	}
	return params, nil
}

// parseParamValue reads one value as the kind of thing it looks like.
func parseParamValue(value string) any {
	if quoted, err := strconv.Unquote(value); err == nil {
		return quoted
	}
	switch strings.ToLower(value) {
	case "true":
		return true
	case "false":
		return false
	}
	if n, err := strconv.Atoi(value); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f
	}
	return value
}
