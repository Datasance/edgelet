package gps

import (
	"bufio"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReadLineWithTimeout_TimesOutWithoutData(t *testing.T) {
	readerEnd, writerEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer readerEnd.Close()
	defer writerEnd.Close()

	d := &DeviceHandler{}
	reader := bufio.NewReader(readerEnd)
	start := time.Now()
	_, err = d.readLineWithTimeout(readerEnd, reader, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatalf("read returned too quickly, expected timeout wait")
	}
}

