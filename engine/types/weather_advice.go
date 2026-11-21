package types

import (
	"fmt"
	"regexp"
	"strconv"

	. "mywant/engine/core"
)

// The weather's advice on its answer (core/want_advice.go): what the weather
// it reports means for whoever goes out in it — an umbrella in rain or snow,
// a coat when it is cold, staying in in a storm.

// coldBelow is the lowest temperature (°C) under which a coat is advised.
const coldBelow = 8

var lowestTemp = regexp.MustCompile(`最低\s*(-?\d+)\s*°`)

func init() { RegisterAdvisor("weather", weatherAdvice) }

func weatherAdvice(in AdviceInput) []Advice {
	cond, _ := in.Fields["weather_condition"].(string)
	turned := ""
	for _, c := range in.Change {
		if c.Field == "weather_condition" && c.Kind == ChangeChanged {
			cond, _ = c.To.(string)
			if from, ok := c.From.(string); ok && from != "" {
				turned = conditionWords[from] + "から"
			}
		}
	}
	text, _ := in.Result.(string)
	var out []Advice
	switch cond {
	case "rain":
		out = append(out, Advice{Kind: AdviceBring, Item: "傘", Why: turned + "雨になりそう"})
	case "snow":
		out = append(out,
			Advice{Kind: AdviceBring, Item: "傘", Why: turned + "雪になりそう"},
			Advice{Kind: AdviceWear, Item: "滑りにくい靴", Why: "雪で足元が滑りやすい"})
	case "storm":
		out = append(out,
			Advice{Kind: AdviceBring, Item: "傘", Why: turned + "荒れた天気になりそう"},
			Advice{Kind: AdviceAvoid, Item: "急がない外出", Why: "嵐の予報"})
	}
	if m := lowestTemp.FindStringSubmatch(text); m != nil {
		if t, err := strconv.Atoi(m[1]); err == nil && t <= coldBelow {
			out = append(out, Advice{Kind: AdviceWear, Item: "上着", Why: fmt.Sprintf("最低 %d°", t)})
		}
	}
	return out
}

// conditionWords are the conditions in words, for why.
var conditionWords = map[string]string{
	"sunny": "晴れ", "cloudy": "曇り", "rain": "雨", "snow": "雪", "storm": "嵐", "fog": "霧", "default": "",
}
