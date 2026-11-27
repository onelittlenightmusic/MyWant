package server

import "testing"

func TestReadErrandJudgement(t *testing.T) {
	v, why, ok := readErrandJudgement("判定: 違う\n理由：始発しかない")
	if !ok || v != errandDifferent || why != "始発しかない" {
		t.Fatalf("%q %q %v", v, why, ok)
	}
	if v, _, ok := readErrandJudgement("- 判定: 同じ\n- 理由: 時刻がずれただけ"); !ok || v != errandSame {
		t.Fatalf("%q %v", v, ok)
	}
	// "わからない" is not read as 違う or 同じ for the words inside it.
	if v, _, _ := readErrandJudgement("判定: わからない"); v != errandUnknown {
		t.Fatalf("%q", v)
	}
	if _, _, ok := readErrandJudgement("たぶん違います"); ok {
		t.Fatal("read a verdict that was not given")
	}
}

func TestErrandReportSaysWhatWasHeard(t *testing.T) {
	r := errandReport(errandDifferent, "Heroさん、", "中野坂上→戸塚", "00:20発", "05:11発")
	if r != "Heroさん、やっぱり違いました。中野坂上→戸塚は、前は「00:20発」でしたが、今は「05:11発」です。" {
		t.Fatal(r)
	}
}

func TestRobotRunsAgain(t *testing.T) {
	if !robotRunsAgain("deploy_want", nil, `Ran "中野坂上→戸塚" (transit_search) again`) {
		t.Fatal("a route searched again is an errand")
	}
	if robotRunsAgain("deploy_want", nil, `Deployed "新宿→横浜"`) {
		t.Fatal("a want made for the first time has nothing to compare with")
	}
	if !robotRunsAgain("use_capability", map[string]string{"capability": "restart"}, "Ran it again.") {
		t.Fatal("restart is an errand")
	}
	if robotRunsAgain("look", nil, "anything") {
		t.Fatal("looking is not running it again")
	}
}
