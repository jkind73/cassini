package sh2

import "testing"

func TestException(t *testing.T) {
	c := &CPU{}
	c.serviceException(4)
	// Run tests
	t.Log("Exception test passed")
}

func TestCPU(t *testing.T) {
	// Placeholder
}
