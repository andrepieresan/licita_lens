package ingestion

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDeadLetterEventKeepsReplayMetadata(t *testing.T) {
	event := deadLetterEvent{
		ID: "procurement.discovered.v1:2:42", Type: "procurement.dead-letter.v1", OccurredAt: time.Now().UTC(),
		Reason: "invalid_json", Error: "unexpected end", OriginalTopic: "procurement.discovered.v1",
		OriginalKey: []byte("source-id"), OriginalValue: []byte("{"), OriginalPartition: 2, OriginalOffset: 42,
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded deadLetterEvent
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID != event.ID || decoded.OriginalPartition != 2 || decoded.OriginalOffset != 42 || string(decoded.OriginalValue) != "{" {
		t.Fatalf("dead letter lost replay metadata: %#v", decoded)
	}
}
