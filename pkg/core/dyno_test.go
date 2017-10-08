package core

import (
	"testing"
)

func TestDynoPortAllocation(t *testing.T) {
	server := &Server{}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
}
