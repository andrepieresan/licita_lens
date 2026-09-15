package ingestion

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/segmentio/kafka-go"
)

type S3Archive struct {
	client *minio.Client
	bucket string
}

func NewS3Archive(ctx context.Context, endpoint, accessKey, secretKey, bucket string, secure bool) (*S3Archive, error) {
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: secure})
	if err != nil {
		return nil, err
	}
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}
	return &S3Archive{client: client, bucket: bucket}, nil
}

func (a *S3Archive) Put(ctx context.Context, key string, body []byte) error {
	_, err := a.client.PutObject(ctx, a.bucket, key, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{ContentType: "application/json"})
	return err
}

type KafkaPublisher struct{ writer *kafka.Writer }

func NewKafkaPublisher(brokers string) *KafkaPublisher {
	return &KafkaPublisher{writer: &kafka.Writer{Addr: kafka.TCP(strings.Split(brokers, ",")...), Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll, Async: false}}
}
func (p *KafkaPublisher) Publish(ctx context.Context, topic, key string, body []byte) error {
	return p.writer.WriteMessages(ctx, kafka.Message{Topic: topic, Key: []byte(key), Value: body, Time: time.Now().UTC()})
}
func (p *KafkaPublisher) Close() error { return p.writer.Close() }
