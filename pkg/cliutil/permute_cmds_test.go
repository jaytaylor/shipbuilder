package cliutil

import (
	"reflect"
	"testing"
)

func TestPermuteCmds(t *testing.T) {
	testCases := []struct {
		prefixes       []string
		suffixes       []string
		suffixOptional bool
		funcName       string
		expected       []string
	}{
		{
			expected: []string{},
		},
		{
			suffixOptional: true,
			expected:       []string{},
		},
		{
			funcName:       "Foo",
			suffixOptional: false,
			expected:       []string{"Foo"},
		},
		{
			prefixes:       []string{},
			suffixes:       []string{},
			suffixOptional: true,
			funcName:       "LoadBalancer_Sync",
			expected:       []string{"LoadBalancer_Sync"},
		},
		{
			prefixes:       []string{"lb", "lbs"},
			suffixes:       []string{},
			suffixOptional: true,
			funcName:       "LoadBalancer_Sync",
			expected:       []string{"lb", "lbs", "LoadBalancer_Sync"},
		},
		{
			prefixes:       []string{"lb", "lbs"},
			suffixes:       []string{},
			suffixOptional: false,
			funcName:       "LoadBalancer_Sync",
			expected:       []string{"LoadBalancer_Sync"},
		},
		{
			prefixes:       []string{"lb", "lbs"},
			suffixes:       []string{"list", "ls"},
			suffixOptional: true,
			funcName:       "LoadBalancer_Sync",
			expected:       []string{"lb", "lbs", "lb:list", "lbs:list", "lb:ls", "lbs:ls", "LoadBalancer_Sync"},
		},
		{
			prefixes:       []string{"lb", "lbs"},
			suffixes:       []string{"remove", "rm", "delete", "del", "-"},
			suffixOptional: false,
			funcName:       "LoadBalancer_Sync",
			expected:       []string{"lb:remove", "lbs:remove", "lb:rm", "lbs:rm", "lb:delete", "lbs:delete", "lb:del", "lbs:del", "lb:-", "lbs:-", "LoadBalancer_Sync"},
		},
	}

	for i, testCase := range testCases {
		actual := PermuteCmds(testCase.prefixes, testCase.suffixes, testCase.suffixOptional, testCase.funcName)

		if !reflect.DeepEqual(testCase.expected, actual) {
			t.Errorf("[i=%v] Expected=%+v but actual=%+v", i, testCase.expected, actual)
		}
	}
}
