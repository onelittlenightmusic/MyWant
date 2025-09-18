package main

import (
	"fmt"
)

type m struct {
	lowend  string
	highend string
	pole    *string
}
type area struct {
	low_end  int
	high_end int
}

func load() {
	//fileからの読み込みを実装

}
func write() {
	//fileへの書き込みを実装

}
func mece(ideas []area, target area) []area {
	//input: ideas 同列のアイディアの配列、target 目的
	//軸を見つける
	//モレを見つける
	//全体範囲を目的から抽出
	//a:=上限と下限を持つstructのarray
	a := make([]area, 10)
	i := 0
	//ideaを下限でソート
	/*		if target.low_end < ideas[0].low_end {
				//aに、最低の下限値でカバーできてない範囲を追加
				a[i] = area{target.low_end, ideas[0].low_end};
				i++;
			}
	*/
	//		a[0] = target;
	max_high := target.low_end
	for _, idea := range ideas {
		if max_high < idea.low_end {
			//モレ発見
			a[i] = area{max_high, idea.low_end}
			i++
		}
		if max_high < idea.high_end {
			max_high = idea.high_end
		}
	}
	if max_high < target.high_end {
		//aに、最大の上限値でカバーできてない範囲を追加
		a[i] = area{max_high, target.high_end}
		i++
	}

	//ダブりを見つける
	return a
}
func main() {
	poles := make([]string, 10)
	a := m{"", "", &poles[0]}
	n := make([]area, 10)
	n[0] = area{10, 20}
	n[1] = area{20, 30}
	x := mece(n, area{0, 100})
	poles[0] = "new"
	fmt.Printf("%s\n", poles[0])
	fmt.Printf("%T\n", poles)
	fmt.Printf("%s\n", *(a.pole))
	for i, x1 := range x {
		fmt.Printf("[%d](%d, %d)\n", i, x1.low_end, x1.high_end)
	}
}
