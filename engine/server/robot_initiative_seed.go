package server

// The situations a decision's heads start from, before the person's own
// reactions have labelled enough of them (robot_initiative_learn.go).
//
// Made here, in the words the robot says a situation in (initiativeState),
// so every device that trains (train-decider: the iPhone app, the Mac's) is
// handed the same rows with the labels — the device holds the model, this
// server the data. What they teach: tell the person a want's new answer when
// it is news worth hearing (a slot opened, rain, a delay, a booking confirmed)
// and the robot has not just spoken; a count ("1件") or "no change" is not.

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
)

// initiativeRecentWeight: a reply within the window counts this many times —
// it is what the person said lately, and there are few of them.
const initiativeRecentWeight = 5

// initiativeSeedVariants are the made events in the many words real ones come
// in — stores and times, lines and minutes, counts — so a head is judged
// (and taught) on words it has not seen, not on the same eighteen sentences.
func initiativeSeedVariants() (news, routine [][2]string) {
	stores := []string{"中野郵便局前店", "北新宿店", "西新宿店", "高円寺店", "新宿御苑前店"}
	times := []string{"9:00", "11:00", "12:30", "14:00", "15:30", "18:00", "20:30"}
	for i, st := range stores {
		for j, t := range times {
			if (i+j)%2 == 0 {
				news = append(news, [2]string{"smartgolf-available", fmt.Sprintf("%sの%sが空いてるよ", st, t)})
			} else {
				news = append(news, [2]string{"smartgolf-available", fmt.Sprintf("%sの%sに空きが出たよ", st, t)})
			}
		}
		routine = append(routine, [2]string{"smartgolf-available", st + "の空き状況に変化なし"})
	}
	lines := []string{"中央線", "山手線", "丸ノ内線", "総武線", "京王線"}
	for i, l := range lines {
		news = append(news,
			[2]string{"新宿→国分寺", fmt.Sprintf("%sが遅延中、%d分遅れ", l, 5+i*5)},
			[2]string{"伊東→中野坂上", l + "が運転見合わせ、振替輸送あり"})
		routine = append(routine, [2]string{"戸塚", l + "は平常通り運転"})
	}
	for n := 1; n <= 6; n++ {
		routine = append(routine,
			[2]string{"transit_search-2118", fmt.Sprintf("%d件", n)},
			[2]string{"新宿→国分寺", fmt.Sprintf("%d件", n)})
	}
	for _, w := range []string{"午後から雨、傘を持っていって", "夕方に雷雨の予報です", "強風注意報が出ています", "明日の朝は雪の予報", "気温が急に下がります、上着を"} {
		news = append(news, [2]string{"NakanoのWeather", w})
	}
	for _, w := range []string{"昨日と同じ晴れ", "変化なし", "いつもと同じ曇り", "いい天気！ 15°で過ごしやすい日だよ", "晴れ、22°"} {
		routine = append(routine, [2]string{"NakanoのWeather", w})
	}
	news = append(news,
		[2]string{"smartgolf-check-reserved-instance", "明日10:00の予約が確定したよ"},
		[2]string{"smartgolf-check-reserved-instance", "予約がキャンセルされました"},
		[2]string{"backup-to-google-instance", "バックアップに失敗しました"},
		[2]string{"checklist-instance", "期限が今日のタスクが2つあります"})
	routine = append(routine,
		[2]string{"smartgolf-check-reserved-instance", "予約の状態に変わりなし"},
		[2]string{"backup-to-google-instance", "バックアップ完了（いつも通り）"},
		[2]string{"checklist-instance", "変更なし"})
	return news, routine
}

// initiativeSeedRows are the made events with their labels — words only,
// as the news question reads them (initiativeEventText) — each once.
func initiativeSeedRows() []map[string]any {
	news, routine := initiativeSeedVariants()
	rows := make([]map[string]any, 0, len(news)+len(routine))
	add := func(p [2]string, gold string) {
		rows = append(rows, map[string]any{
			"state": fmt.Sprintf("出来事: wantの新しい結果。want: %s。結果: %s。", p[0], p[1]),
			"gold":  map[string]string{newsQuestionID: gold}})
	}
	for _, p := range news {
		add(p, "news")
	}
	for _, p := range routine {
		add(p, "routine")
	}
	return rows
}

// initiativeTrainingSet is what a training is handed, oldest knowledge to
// newest: the made rows (the oldest prior, compressed to what
// they say, initiativeFold), the older replies compressed (initiativeCompress), and the
// replies within the window, weighted. Shuffled, so a worker holding out every
// fifth row holds out some of each.
func initiativeTrainingSet(recent, older, offerSeeds []map[string]any) []map[string]any {
	rows := initiativeFold(append(initiativeSeedRows(), offerSeeds...))
	rows = append(rows, older...)
	for range initiativeRecentWeight {
		rows = append(rows, recent...)
	}
	rng := rand.New(rand.NewSource(11))
	rng.Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] })
	return rows
}

// initiativeFold compresses repeated rows as old replies are compressed: each
// (state, label) as many times as log2(1 + how often it came) — what was said
// often still weighs more, but no longer as much more as it was repeated.
func initiativeFold(rows []map[string]any) []map[string]any {
	counts := map[string]int{}
	first := map[string]map[string]any{}
	var order []string
	for _, r := range rows {
		k, _ := json.Marshal(r)
		key := string(k)
		if counts[key] == 0 {
			first[key] = r
			order = append(order, key)
		}
		counts[key]++
	}
	var out []map[string]any
	for _, key := range order {
		for range max(1, int(math.Round(math.Log2(1+float64(counts[key]))))) {
			out = append(out, first[key])
		}
	}
	return out
}
