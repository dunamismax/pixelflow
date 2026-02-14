package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

type Client struct {
	client *asynq.Client
	queue  string
}

func NewClient(redisOpt asynq.RedisClientOpt, queueName string) *Client {
	return &Client{
		client: asynq.NewClient(redisOpt),
		queue:  queueName,
	}
}

func (c *Client) EnqueueProcessImage(ctx context.Context, payload ProcessImagePayload) (*asynq.TaskInfo, error) {
	task, err := NewProcessImageTask(payload)
	if err != nil {
		return nil, err
	}
	return c.client.EnqueueContext(
		ctx,
		task,
		asynq.Queue(c.queue),
		asynq.MaxRetry(5),
		asynq.Timeout(3*time.Minute),
	)
}

func (c *Client) Close() error {
	return c.client.Close()
}
