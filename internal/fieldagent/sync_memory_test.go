package fieldagent

import "testing"

func TestParseMicroservice_MemoryLimitMiBToBytes(t *testing.T) {
	ms, err := parseMicroservice(map[string]any{
		"uuid":        "ms-mem",
		"imageId":     "alpine:3.19",
		"memoryLimit": float64(1024),
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ms.MemoryLimit == nil {
		t.Fatal("expected memory limit to be set")
	}
	want := int64(1024 * 1024 * 1024)
	if *ms.MemoryLimit != want {
		t.Fatalf("MemoryLimit=%d want %d", *ms.MemoryLimit, want)
	}
}
