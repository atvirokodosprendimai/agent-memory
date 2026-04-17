package ipfs

import (
	"testing"
	"time"
)

func TestClientTimeout(t *testing.T) {
	c := NewClient("http://localhost:5001")
	defer c.Close()

	got := c.client.Timeout
	want := 30 * time.Second
	if got != want {
		t.Errorf("client timeout = %v, want %v", got, want)
	}
}
