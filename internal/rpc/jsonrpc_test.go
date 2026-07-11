package rpc

import (
	"bufio"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONRPCMessages(t *testing.T) {
	req, err := NewRequest(StringID("1"), "tools/list", map[string]any{"cursor": ""})
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	raw, err := Encode(req)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"jsonrpc":"2.0"`) || !strings.Contains(text, `"id":"1"`) || !strings.Contains(text, `"method":"tools/list"`) || !strings.Contains(text, `"params"`) {
		t.Fatalf("request = %s", text)
	}

	note, err := NewNotification("notifications/ready", nil)
	if err != nil {
		t.Fatalf("NewNotification returned error: %v", err)
	}
	raw, err = Encode(note)
	if err != nil {
		t.Fatalf("Encode notification returned error: %v", err)
	}
	if strings.Contains(string(raw), `"id"`) {
		t.Fatalf("notification has id: %s", raw)
	}

	_, resp, _, err := Decode([]byte(`{"jsonrpc":"2.0","id":2,"result":{"ok":true}}`))
	if err != nil {
		t.Fatalf("Decode response returned error: %v", err)
	}
	if resp.ID.String() != "2" || !strings.Contains(string(resp.Result), `"ok":true`) {
		t.Fatalf("response = %#v", resp)
	}
	_, resp, _, err = Decode([]byte(`{"jsonrpc":"2.0","id":"abc","error":{"code":-32601,"message":"missing"}}`))
	if err != nil {
		t.Fatalf("Decode error response returned error: %v", err)
	}
	if resp.ID.String() != "abc" || resp.Error.Code != -32601 || resp.Error.Message != "missing" {
		t.Fatalf("error response = %#v", resp)
	}
	if _, _, _, err := Decode([]byte(`{"id":"1","result":{}}`)); err == nil {
		t.Fatalf("missing jsonrpc returned nil error")
	}
	if _, _, _, err := Decode([]byte(`{"jsonrpc":"1.0","id":"1","result":{}}`)); err == nil {
		t.Fatalf("bad version returned nil error")
	}
}

func TestReadConsecutiveMessages(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{}}\n"))
	first, err := ReadMessage(reader)
	if err != nil {
		t.Fatalf("ReadMessage first: %v", err)
	}
	second, err := ReadMessage(reader)
	if err != nil {
		t.Fatalf("ReadMessage second: %v", err)
	}
	var a Response
	var b Response
	if err := json.Unmarshal(first, &a); err != nil {
		t.Fatalf("unmarshal first: %v", err)
	}
	if err := json.Unmarshal(second, &b); err != nil {
		t.Fatalf("unmarshal second: %v", err)
	}
	if a.ID.String() != "1" || b.ID.String() != "2" {
		t.Fatalf("ids = %s %s", a.ID.String(), b.ID.String())
	}
}
