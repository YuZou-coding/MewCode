package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

type Transport interface {
	Send(ctx context.Context, data []byte) error
	Receive(ctx context.Context) ([]byte, error)
	Close() error
}

type Session struct {
	transport Transport
	nextID    atomic.Int64
	pending   map[string]chan Response
	mu        sync.Mutex
	closed    chan struct{}
	closeOnce sync.Once
	err       error
}

func NewSession(transport Transport) *Session {
	s := &Session{
		transport: transport,
		pending:   map[string]chan Response{},
		closed:    make(chan struct{}),
	}
	go s.readLoop()
	return s
}

func (s *Session) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := StringID(fmt.Sprintf("%d", s.nextID.Add(1)))
	req, err := NewRequest(id, method, params)
	if err != nil {
		return nil, err
	}
	raw, err := Encode(req)
	if err != nil {
		return nil, err
	}
	ch := make(chan Response, 1)
	s.mu.Lock()
	if s.err != nil {
		err := s.err
		s.mu.Unlock()
		return nil, err
	}
	s.pending[id.String()] = ch
	s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		s.removePending(id.String())
		return nil, err
	}
	if err := s.transport.Send(ctx, raw); err != nil {
		s.removePending(id.String())
		return nil, err
	}
	select {
	case resp, ok := <-ch:
		if !ok {
			s.mu.Lock()
			err := s.err
			s.mu.Unlock()
			if err == nil {
				err = fmt.Errorf("jsonrpc session closed")
			}
			return nil, err
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("jsonrpc error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		s.removePending(id.String())
		return nil, ctx.Err()
	case <-s.closed:
		s.removePending(id.String())
		s.mu.Lock()
		err := s.err
		s.mu.Unlock()
		if err == nil {
			err = fmt.Errorf("jsonrpc session closed")
		}
		return nil, err
	}
}

func (s *Session) Close() error {
	s.closeWithError(fmt.Errorf("jsonrpc session closed"))
	return s.transport.Close()
}

func (s *Session) PendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending)
}

func (s *Session) readLoop() {
	for {
		raw, err := s.transport.Receive(context.Background())
		if err != nil {
			s.closeWithError(err)
			return
		}
		_, resp, _, err := Decode(raw)
		if err != nil || resp == nil {
			continue
		}
		s.mu.Lock()
		ch := s.pending[resp.ID.String()]
		if ch != nil {
			delete(s.pending, resp.ID.String())
		}
		s.mu.Unlock()
		if ch != nil {
			ch <- *resp
		}
	}
}

func (s *Session) closeWithError(err error) {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.err = err
		for id, ch := range s.pending {
			close(ch)
			delete(s.pending, id)
		}
		s.mu.Unlock()
		close(s.closed)
	})
}

func (s *Session) removePending(id string) {
	s.mu.Lock()
	delete(s.pending, id)
	s.mu.Unlock()
}
