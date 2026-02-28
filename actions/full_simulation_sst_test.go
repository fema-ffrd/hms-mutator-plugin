package actions

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func Test_RecordSet(t *testing.T) {
	datapath := "/workspaces/hms-mutator/exampledata/trinity/storms.csv"
	data, err := os.ReadFile(datapath)
	if err != nil {
		t.Fail()
	}
	datastring := string(data)
	datalines := strings.Split(datastring, "\n")
	records := FullSimulationResult{}
	//skip header
	for i, r := range datalines {
		if i != 0 {
			elements := strings.Split(r, ",")
			en, err := strconv.Atoi(elements[0])
			if err != nil {
				t.Fail()
			}
			x, err := strconv.ParseFloat(elements[2], 64)
			if err != nil {
				t.Fail()
			}
			y, err := strconv.ParseFloat(elements[3], 64)
			if err != nil {
				t.Fail()
			}
			er := EventResult{
				EventNumber: int64(en),
				StormPath:   elements[1],
				X:           x,
				Y:           y,
				StormType:   elements[4],
				StormDate:   elements[5],
				BasinPath:   elements[6],
			}
			records = append(records, er)
		}
	}
	//test passes if we successfully parsed the CSV
}
