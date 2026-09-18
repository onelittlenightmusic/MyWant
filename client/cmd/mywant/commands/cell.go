package commands

import (
	"fmt"
	"strconv"
	"strings"
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
