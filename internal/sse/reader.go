package sse

import (
	"bufio"
	"io"
	"strings"
)

type Event struct {
	Name string
	Data string
}

type Reader struct {
	scanner *bufio.Scanner
}

func NewReader(r io.Reader) *Reader {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &Reader{scanner: scanner}
}

func (r *Reader) Next() (Event, error) {
	var event Event
	var data []string
	seen := false

	for r.scanner.Scan() {
		line := strings.TrimSuffix(r.scanner.Text(), "\r")
		if line == "" {
			if seen {
				event.Data = strings.Join(data, "\n")
				return event, nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}

		seen = true
		field, value, ok := strings.Cut(line, ":")
		if ok {
			value = strings.TrimPrefix(value, " ")
		} else {
			value = ""
		}

		switch field {
		case "event":
			event.Name = value
		case "data":
			data = append(data, value)
		}
	}

	if err := r.scanner.Err(); err != nil {
		return Event{}, err
	}
	if seen {
		event.Data = strings.Join(data, "\n")
		return event, nil
	}
	return Event{}, io.EOF
}
