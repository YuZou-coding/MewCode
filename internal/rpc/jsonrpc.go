package rpc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const Version = "2.0"

type ID struct {
	value string
}

func StringID(value string) ID {
	return ID{value: value}
}

func (id ID) String() string {
	return id.value
}

func (id ID) MarshalJSON() ([]byte, error) {
	if id.value == "" {
		return []byte("null"), nil
	}
	return json.Marshal(id.value)
}

func (id *ID) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if bytes.Equal(raw, []byte("null")) || len(raw) == 0 {
		id.value = ""
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		id.value = text
		return nil
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return err
	}
	id.value = number.String()
	return nil
}

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      ID              `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      ID              `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func NewRequest(id ID, method string, params any) (Request, error) {
	raw, err := marshalParams(params)
	if err != nil {
		return Request{}, err
	}
	return Request{JSONRPC: Version, ID: id, Method: method, Params: raw}, nil
}

func NewNotification(method string, params any) (Notification, error) {
	raw, err := marshalParams(params)
	if err != nil {
		return Notification{}, err
	}
	return Notification{JSONRPC: Version, Method: method, Params: raw}, nil
}

func Encode(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func Decode(raw []byte) (Request, *Response, *Notification, error) {
	var envelope struct {
		JSONRPC string           `json:"jsonrpc"`
		ID      *json.RawMessage `json:"id"`
		Method  string           `json:"method"`
		Result  *json.RawMessage `json:"result"`
		Error   *Error           `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(raw), &envelope); err != nil {
		return Request{}, nil, nil, err
	}
	if envelope.JSONRPC != Version {
		if envelope.JSONRPC == "" {
			return Request{}, nil, nil, fmt.Errorf("jsonrpc is required")
		}
		return Request{}, nil, nil, fmt.Errorf("unsupported jsonrpc version: %s", envelope.JSONRPC)
	}
	if envelope.Method != "" && envelope.ID != nil {
		var req Request
		if err := json.Unmarshal(raw, &req); err != nil {
			return Request{}, nil, nil, err
		}
		return req, nil, nil, nil
	}
	if envelope.Method != "" {
		var note Notification
		if err := json.Unmarshal(raw, &note); err != nil {
			return Request{}, nil, nil, err
		}
		return Request{}, nil, &note, nil
	}
	if envelope.ID != nil && (envelope.Result != nil || envelope.Error != nil) {
		var resp Response
		if err := json.Unmarshal(raw, &resp); err != nil {
			return Request{}, nil, nil, err
		}
		return Request{}, &resp, nil, nil
	}
	return Request{}, nil, nil, fmt.Errorf("unknown jsonrpc message")
}

func ReadMessage(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return ReadMessage(r)
	}
	return line, nil
}

func ReadAllMessages(r io.Reader) ([][]byte, error) {
	reader := bufio.NewReader(r)
	var messages [][]byte
	for {
		msg, err := ReadMessage(reader)
		if err != nil {
			if err == io.EOF {
				return messages, nil
			}
			return messages, err
		}
		messages = append(messages, msg)
	}
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	if raw, ok := params.(json.RawMessage); ok {
		return raw, nil
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return raw, nil
}
