package sse

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gogf/gf/v2/container/gmap"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
)

var ErrUnavailable = errors.New("sse client is unavailable")

const defaultBufferSize = 100

type Client struct {
	Id          string
	Request     *ghttp.Request
	messageChan chan string
	ctx         context.Context
	cancel      context.CancelFunc
	finishOnce  sync.Once
	finished    chan struct{}
}

type Service struct {
	clients    *gmap.StrAnyMap
	bufferSize int
}

func New() *Service {
	return NewWithBuffer(defaultBufferSize)
}

func NewWithBuffer(bufferSize int) *Service {
	if bufferSize <= 0 {
		bufferSize = defaultBufferSize
	}
	return &Service{clients: gmap.NewStrAnyMap(true), bufferSize: bufferSize}
}

func (s *Service) Create(ctx context.Context, r *ghttp.Request) (*Client, error) {
	if r == nil {
		return nil, ErrUnavailable
	}
	if ctx == nil {
		ctx = r.GetCtx()
	}
	clientCtx, cancel := context.WithCancel(ctx)
	r.Response.Header().Set("Content-Type", "text/event-stream")
	r.Response.Header().Set("Cache-Control", "no-cache")
	r.Response.Header().Set("Connection", "keep-alive")
	r.Response.Header().Set("Access-Control-Allow-Origin", "*")

	clientID := r.Get("client_id", guid.S()).String()
	client := &Client{
		Id:          clientID,
		Request:     r,
		messageChan: make(chan string, s.bufferSize),
		ctx:         clientCtx,
		cancel:      cancel,
		finished:    make(chan struct{}),
	}
	r.Response.Writefln("id: %s", clientID)
	r.Response.Writefln("event: connected")
	r.Response.Writefln("data: {\"status\": \"connected\", \"client_id\": \"%s\"}\n", clientID)
	r.Response.Flush()
	go client.writeLoop()
	return client, nil
}

func (c *Client) SendToClient(eventType, data string) bool {
	if c == nil || c.Request == nil {
		return false
	}
	payload := formatEvent(eventType, data)
	select {
	case <-c.ctx.Done():
		return false
	case c.messageChan <- payload:
		return true
	default:
		c.cancel()
		return false
	}
}

// Finish drains all queued events and stops the writer. It is used after the
// terminal event has been queued so a successful response cannot be truncated.
func (c *Client) Finish() {
	if c == nil {
		return
	}
	c.finishOnce.Do(func() {
		close(c.messageChan)
	})
	<-c.finished
}

func (c *Client) writeLoop() {
	defer close(c.finished)
	for {
		select {
		case <-c.ctx.Done():
			return
		case payload, ok := <-c.messageChan:
			if !ok {
				return
			}
			c.Request.Response.Write(payload)
			c.Request.Response.Flush()
		}
	}
}

func formatEvent(eventType, data string) string {
	return fmt.Sprintf(
		"id: %d\nevent: %s\ndata: %s\n\n",
		time.Now().UnixNano(), eventType, data,
	)
}
