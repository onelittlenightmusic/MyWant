package server

import (
	"testing"
	"time"
)

func workReading(id, q string, now time.Time) (string, bool) {
	for _, r := range timeReadings {
		if r.ID == id {
			return r.Work(readTimeNumbers(q), now)
		}
	}
	return "", false
}

func TestTimeReadingsWorkOnTheRequest(t *testing.T) {
	now := time.Date(2026, 11, 24, 19, 33, 0, 0, time.Local)
	cases := []struct {
		id, q, want string
		ok          bool
	}{
		{"after_min", "10分後に国分寺駅から中野坂上", "19:43", true},
		{"after_min", "十分後に国分寺から中野坂上", "19:43", true},
		{"after_min", "１５分後に出る", "19:48", true},
		{"after_hour", "2時間後に新宿から横浜", "21:33", true},
		{"after_hour", "1時間半後に出発", "21:03", true},
		{"after_min", "1時間30分後", "", false}, // a length, not minutes alone
		{"at_today", "21時に新宿を出る", "21:00", true},
		{"at_today", "9時40分までに戸塚", "", false}, // gone today
		{"at_tomorrow", "明日の朝9時40分までに戸塚", "09:40", true},
		{"at_today", "午後8時半に着きたい", "20:30", true},
		{"at_today", "20:15発で", "20:15", true},
		{"this_hour_min", "50分に出る", "19:50", true},
		{"this_hour_min", "10分に出る", "", false}, // gone this hour
		{"none", "国分寺から中野坂上", "", true},
	}
	for _, c := range cases {
		got, ok := workReading(c.id, c.q, now)
		if ok != c.ok || got != c.want {
			t.Errorf("%s 「%s」 = %q, %v; want %q, %v", c.id, c.q, got, ok, c.want, c.ok)
		}
	}
}

func TestTimeReadingQuestionsStayTheSame(t *testing.T) {
	// The labels Laya is trained on: a change is a new question with no head.
	q := timeReadingQuestions(true)
	if len(q["time_reading"].Criteria) != len(timeReadings) || len(q["time_side"].Criteria) != 2 {
		t.Fatalf("questions: %+v", q)
	}
	if _, ok := timeReadingQuestions(false)["time_side"]; ok {
		t.Fatal("no side asked when the type has none")
	}
}

func TestMagicCutTime(t *testing.T) {
	for q, want := range map[string]string{
		"10分後に国分寺駅から中野坂上":   "国分寺駅から中野坂上",
		"十分後に国分寺から中野坂上":     "国分寺から中野坂上",
		"国分寺から中野坂上 20:15発で": "国分寺から中野坂上",
		"明日の朝9時40分までに戸塚へ":   "戸塚へ",
		"2時間半後に新宿から横浜":      "新宿から横浜",
		"九段下から三鷹":           "九段下から三鷹", // a station's own numerals stay
		"荻窪から中野坂上まで":        "荻窪から中野坂上まで",
	} {
		if got := magicCutTime(q); got != want {
			t.Errorf("magicCutTime(%q) = %q, want %q", q, got, want)
		}
	}
}

func TestMagicParamsReadKeepsOnlyWhatWasSaid(t *testing.T) {
	slots := []magicSlotDef{{"from", "station"}, {"to", "station"}}
	q := "10分後に国分寺駅から中野坂上"
	got := magicParamsRead("from=国分寺駅\nto=「中野坂上」\n", q, slots, nil)
	if got["from"] != "国分寺駅" || got["to"] != "中野坂上" {
		t.Fatalf("read %v", got)
	}
	// Words of its own, or nothing: the sentence's reading stays.
	got = magicParamsRead("from=国分寺\n- to = 新宿\nnote=x", "国分寺駅から中野坂上", slots, nil)
	if got["from"] != "国分寺" || got["to"] != "" {
		t.Fatalf("read %v", got)
	}
}

func TestMagicParamsReadByDescription(t *testing.T) {
	slots := []magicSlotDef{{"from", "station"}, {"to", "station"}}
	desc := map[string]string{"from": "Departure station", "to": "Arrival station"}
	got := magicParamsRead("Departure station: 国分寺駅  \nArrival station：中野坂上", "10分後に国分寺駅から中野坂上", slots, desc)
	if got["from"] != "国分寺駅" || got["to"] != "中野坂上" {
		t.Fatalf("read %v", got)
	}
}

func TestMagicSentenceWords(t *testing.T) {
	got := magicSentenceWords(magicSource{typ: "transit_search",
		tmpls:   "<from:station>から<to:station>|<from:station>から<to:station>まで|<alias> <from:station> <to:station>",
		aliases: []string{"乗り換え", "transit"}})
	want := "乗り換え・transit: （station）から（station） / （station）から（station）まで"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExpandAlias(t *testing.T) {
	got := expandAlias("中野の<alias>", []string{"天気", "weather"})
	if len(got) != 2 || got[0] != "中野の天気" || got[1] != "中野のweather" {
		t.Fatalf("got %v", got)
	}
	if got := expandAlias("横浜の天気が知りたい", []string{"天気"}); len(got) != 1 || got[0] != "横浜の天気が知りたい" {
		t.Fatalf("words with no <alias> are themselves: %v", got)
	}
	if got := expandAlias("<alias>", nil); len(got) != 0 {
		t.Fatalf("no aliases, none: %v", got)
	}
	if got := cleanAliases([]string{" 天気", "", "天気", "weather"}); len(got) != 2 {
		t.Fatalf("clean: %v", got)
	}
}

func TestMeets(t *testing.T) {
	if got := meets([]string{"reschedule"}, []string{"reschedule", "notify"}); len(got) != 1 || got[0] != "reschedule" {
		t.Fatalf("got %v", got)
	}
	if got := meets([]string{"rebook"}, []string{"reschedule", "notify"}); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestSayShape(t *testing.T) {
	route := sayShape("15分後に中野から渋谷に行きたい", []string{"中野", "渋谷"})
	if route != "○分後に○から○に行きたい" {
		t.Fatalf("shape %q", route)
	}
	transit := sayShape("1時間半後に渋谷から品川に行きたい", []string{"渋谷", "品川"})
	weather := sayShape("札幌の天気が知りたい", []string{"札幌"})
	if shapeSimilarity(route, transit) <= shapeSimilarity(route, weather) {
		t.Fatalf("a route is liker a route: %.2f vs %.2f", shapeSimilarity(route, transit), shapeSimilarity(route, weather))
	}
}

func TestKeywordCues(t *testing.T) {
	byKey := map[string]magicSource{
		"ある場所の天気を調べる":     {typ: "weather", aliases: []string{"weather", "天気"}},
		"盤面の場所やものの位置を見せる": {tool: "point", aliases: []string{"場所・どこ"}},
		"メモを貼る":           {typ: "note", aliases: []string{"メモ", "x"}},
	}
	got := keywordCues("札幌の天気", byKey)
	if got["ある場所の天気を調べる"] != 1 || len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	if got := keywordCues("国分寺はどこ", byKey); got["盤面の場所やものの位置を見せる"] != 1 {
		t.Fatalf("a tool's words: %v", got)
	}
	if got := keywordCues("Weather Tokyo", byKey); got["ある場所の天気を調べる"] != 1 {
		t.Fatalf("any case: %v", got)
	}
}

func TestPlaceFit(t *testing.T) {
	station := map[string]bool{"station": true}
	both := map[string]bool{"station": true, "city": true}
	two := []namedThing{{"中野坂上", both}, {"大森", station}}
	if f := placeFit([]string{"station", "station"}, two); f != 1 {
		t.Fatalf("a route of two stations: %v", f)
	}
	if f := placeFit([]string{"city"}, two); f != 0.5 {
		t.Fatalf("a weather of one of two: %v", f)
	}
	if f := placeFit([]string{"station"}, []namedThing{{"大森", station}}); f != 1 {
		t.Fatalf("大森まで, the to alone: %v", f)
	}
}
