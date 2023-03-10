package eew

import (
	"os"
	"testing"
)

func TestParseXml(t *testing.T) {
	type data struct {
		File    string
		Message string
	}
	tests := []data{
		{
			File:    "samples/77_01_01_110311_VXSE45.xml",
			Message: "**緊急地震速報（予報）** 第23報\n11日14時46分ごろ、地震がありました。\n震源地は三陸沖（北緯38.1度、東経142.9度）で震源の深さは約10km、地震の規模（マグニチュード）は8.4、この地震による最大震度は震度6強と推定されます。\nhttps://earthquake.tenki.jp/bousai/earthquake/detail/2011/03/11/2011-03-11-14-46-40.html",
		},
		{
			File:    "samples/77_01_02_110311_VXSE45.xml",
			Message: "**緊急地震速報（予報）** 第23報\n不明ごろ、地震がありました。\n震源地は不明（経緯不明）で震源の深さは不明、地震の規模（マグニチュード）は不明、この地震による最大震度は不明と推定されます。\nhttps://earthquake.tenki.jp/bousai/earthquake/detail/2011/03/11/2011-03-11-14-46-40.html",
		},
	}
	for _, d := range tests {
		f, err := os.Open(d.File)
		if err != nil {
			t.Errorf("failed to open sample xml: %v", d.File)
		}
		content, err := NewContent(f)
		f.Close()
		if err != nil {
			t.Errorf("failed to parseXml: %v", err)
		}
		got := content.String()
		want := d.Message
		if got != want {
			t.Errorf("diff got:%s want:%s", got, want)
		}
	}
}
