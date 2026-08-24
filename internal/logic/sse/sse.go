package sse

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/container/gmap"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
)

type Client struct {
	Id          string
	Request     *ghttp.Request
	messageChan chan string
}

type Service struct {
	clients *gmap.StrAnyMap
}

func New() *Service {
	return &Service{clients: gmap.NewStrAnyMap(true)}
}

func (s *Service) Create(_ context.Context, r *ghttp.Request) (*Client, error) {
	r.Response.Header().Set("Content-Type", "text/event-stream")
	r.Response.Header().Set("Cache-Control", "no-cache")
	r.Response.Header().Set("Connection", "keep-alive")
	r.Response.Header().Set("Access-Control-Allow-Origin", "*")

	clientID := r.Get("client_id", guid.S()).String()
	client := &Client{
		Id:          clientID,
		Request:     r,
		messageChan: make(chan string, 100),
	}
	r.Response.Writefln("id: %s", clientID)
	r.Response.Writefln("event: connected")
	r.Response.Writefln("data: {\"status\": \"connected\", \"client_id\": \"%s\"}\n", clientID)
	r.Response.Flush()
	return client, nil
}

func (c *Client) SendToClient(eventType, data string) bool {
	c.Request.Response.Write(formatEvent(eventType, data))
	c.Request.Response.Flush()
	return true
}

func formatEvent(eventType, data string) string {
	return fmt.Sprintf(
		"id: %d\nevent: %s\ndata: %s\n\n",
		time.Now().UnixNano(), eventType, data,
	)
}
