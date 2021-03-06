package kv

import (
	"bytes"
	"log"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/coyove/iis/dal/kv/cache"
	//sync "github.com/sasha-s/go-deadlock"
)

var dyTable = "iis"

type DynamoKV struct {
	cache *cache.GlobalCache
	db    *dynamodb.DynamoDB
}

func NewDynamoKV(region, accessKey, secretKey string) *DynamoKV {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
		HTTPClient: &http.Client{
			Timeout: time.Second,
			Transport: &http.Transport{
				MaxConnsPerHost: 200,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	db := dynamodb.New(sess)
	_, err = db.DescribeEndpoints(&dynamodb.DescribeEndpointsInput{})
	if err != nil {
		panic(err)
	}
	r := &DynamoKV{
		db: db,
	}
	return r
}

func (m *DynamoKV) SetGlobalCache(c *cache.GlobalCache) {
	m.cache = c
}

func (m *DynamoKV) Get(key string) ([]byte, error) {
	nocache := false

	v, ok := m.cache.Get(key)
	if bytes.Equal(v, locker) {
		v = nil
		nocache = true
		// continue fetching value from dynamodb
	} else if ok {
		return v, nil
	}

	in := &dynamodb.GetItemInput{
		TableName: &dyTable,
		Key: map[string]*dynamodb.AttributeValue{
			"id": &dynamodb.AttributeValue{
				S: &key,
			},
		},
	}

	out, err := m.db.GetItem(in)
	if err != nil {
		return nil, err
	}

	if vi := out.Item["value"]; vi != nil && vi.S != nil {
		v = []byte(*vi.S)
	}

	if !nocache {
		if err := m.cache.Add(key, v); err != nil {
			log.Println("KV add:", err)
		}
	}

	return v, err
}

func (m *DynamoKV) Set(key string, value []byte) error {
	if err := m.cache.Add(key, locker); err != nil {
		return err
	}

	in := &dynamodb.UpdateItemInput{
		TableName: &dyTable,
		Key: map[string]*dynamodb.AttributeValue{
			"id": &dynamodb.AttributeValue{
				S: &key,
			},
		},
		UpdateExpression: aws.String("set #xyzvalue = :value"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":value": &dynamodb.AttributeValue{
				S: aws.String(string(value)),
			},
		},
		ExpressionAttributeNames: map[string]*string{
			"#xyzvalue": aws.String("value"),
		},
	}

	_, err := m.db.UpdateItem(in)
	if err == nil {
		m.cache.Add(key, value)
	}
	return err
}

// func (m *DynamoKV) Delete(key string) error {
// 	m.cache.Remove(key)
//
// 	in := &dynamodb.DeleteItemInput{
// 		TableName: &dyTable,
// 		Key: map[string]*dynamodb.AttributeValue{
// 			"id": &dynamodb.AttributeValue{
// 				S: &key,
// 			},
// 		},
// 	}
// 	_, err := m.db.DeleteItem(in)
// 	return err
// }
