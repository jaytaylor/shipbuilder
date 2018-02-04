package core

import (
	"testing"
)

func TestNodeStatusParse(t *testing.T) {
	testCases := []struct {
		input         string
		numContainers int
	}{
		{
			input:         "9638",
			numContainers: 0,
		},
		{
			input:         "9638 fancypie-v5-web-10001 fancypie-v5-web-10002 fancypie-v5-web-10006",
			numContainers: 3,
		},
	}

	for i, testCase := range testCases {
		ns := &NodeStatus{}
		ns.Parse(testCase.input, nil)
		if ns.Err != nil {
			t.Errorf("[i=%v] Unexpected error parsing input=%q: %s", i, testCase.input, ns.Err)
			continue
		}
		if expected, actual := testCase.numContainers, len(ns.Containers); actual != expected {
			t.Errorf("[i=%v] Expected numContainers=%v but actual=%v", i, expected, actual)
		}
	}
}
