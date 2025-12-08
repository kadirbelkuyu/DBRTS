package explorer

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestSplitMongoCommand(t *testing.T) {
	cmd, payload := splitMongoCommand("insert {\"name\":\"ok\"}")
	if cmd != "insert" {
		t.Fatalf("expected insert command, got %s", cmd)
	}
	if payload == "" {
		t.Fatalf("expected payload to be preserved")
	}

	cmd, payload = splitMongoCommand("find")
	if cmd != "find" || payload != "" {
		t.Fatalf("expected find command without payload, got %s %s", cmd, payload)
	}
}

func TestDecodeMongoDocument(t *testing.T) {
	doc, err := decodeMongoDocument("{\"name\":\"alpha\"}")
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if doc["name"] != "alpha" {
		t.Fatalf("expected field to decode correctly")
	}

	doc, err = decodeMongoDocument("")
	if err != nil || len(doc) != 0 {
		t.Fatalf("expected empty payload to yield empty doc without error")
	}
}

func TestIsEmptyFilter(t *testing.T) {
	if !isEmptyFilter(nil) {
		t.Fatalf("nil should be considered empty filter")
	}
	if !isEmptyFilter(bson.M{}) || !isEmptyFilter(bson.D{}) {
		t.Fatalf("empty bson structures should be empty")
	}
	if isEmptyFilter(bson.M{"a": 1}) {
		t.Fatalf("non-empty filter should not be empty")
	}
}
