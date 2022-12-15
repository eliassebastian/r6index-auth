package ubisoft

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/eliassebastian/r6index-auth/internal/rabbitmq"
)

type UbisoftConfig struct {
	client  *http.Client
	ctx     context.Context
	cancel  context.CancelFunc
	session []byte
}

func NewUbisoftConnection() *UbisoftConfig {
	return &UbisoftConfig{
		client: &http.Client{
			Timeout: time.Second * 10,
		},
	}
}

func basicToken() string {
	username := os.Getenv("UBISOFT_USERNAME")
	pw := os.Getenv("UBISOFT_PASS")

	bs := []byte(fmt.Sprintf("%s:%s", username, pw))

	return fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString(bs))
}

func createSessionURL(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, errors.New("error creating session url")
	}

	appId := os.Getenv("UBISOFT_APPID")
	token := basicToken()

	req.Header = http.Header{
		"Content-Type":  []string{"application/json"},
		"Ubi-AppId":     []string{appId},
		"Authorization": []string{token},
		"Connection":    []string{"keep-alive"},
	}

	return req, nil
}

func fetchSessionData(ctx context.Context, client *http.Client, r *http.Request) []byte {
	//TODO: overhaul backoff and retry logic

	//backoff
	bs := []time.Duration{
		5 * time.Second,
		10 * time.Second,
		15 * time.Second,
		30 * time.Second,
	}

	for i, b := range bs {
		select {
		case <-ctx.Done():
			log.Println("Session Fetch Loop Context Done")
			return nil
		default:
			log.Println("Running Client Fetch Iteration:", i)

			res, err := client.Do(r)
			if err != nil {
				return nil
			}

			if res.StatusCode == 200 {
				bs, err := io.ReadAll(res.Body)
				if err != nil {
					log.Println("error reading response body", err)
					res.Body.Close()
					return nil
				}
				res.Body.Close()
				return bs
			}

			log.Println("Retrying Session:", i+1)
			res.Body.Close()
			time.Sleep(b)
		}
	}

	return nil
}

func (c *UbisoftConfig) Connect(ctx context.Context, p *rabbitmq.RabbitMQConfig) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c.ctx = ctx
	c.cancel = cancel
	url := os.Getenv("UBISOFT_URL")

	req, err := createSessionURL(ctx, url)
	if err != nil {
		return err
	}

	sd := fetchSessionData(ctx, c.client, req)
	if sd == nil {
		return errors.New("session fetched returned nil")
	}

	c.session = sd
	if ke := p.Produce(ctx, &sd); ke != nil {
		return ke
	}

	return nil
}

func (c *UbisoftConfig) Stop() {
	c.cancel()
}
