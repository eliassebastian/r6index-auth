package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/app/client/retry"
	"github.com/cloudwego/hertz/pkg/network/standard"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/eliassebastian/r6index-auth/pkg/rabbitmq"
	"github.com/eliassebastian/r6index-auth/pkg/ubisoft"
	"github.com/go-co-op/gocron"
	"github.com/joho/godotenv"
)

func main() {
	log.Println("::::::::: PRE R6 INDEX AUTH STARTING")
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file", err)
	}

	errC, err := run()
	if err != nil {
		log.Fatalf("Couldn't run: %s", err)
	}

	if err := <-errC; err != nil {
		log.Fatalf("Error while running: %s", err)
	}

	log.Println(os.Getenv("UBISOFT_URL"))
}

type serverConfig struct {
	rabbitmq  *rabbitmq.RabbitMQConfig
	scheduler *gocron.Scheduler
	ubisoft   *ubisoft.UbisoftRepository
	doneC     chan struct{}
}

func run() (<-chan error, error) {

	//http client
	c, err := client.NewClient(
		client.WithResponseBodyStream(true),
		client.WithDialTimeout(1*time.Second),
		client.WithDialer(standard.NewDialer()),
		client.WithRetryConfig(
			retry.WithInitDelay(1*time.Second),
			retry.WithMaxAttemptTimes(5),
			retry.WithDelayPolicy(retry.BackOffDelayPolicy),
		),
	)

	if err != nil {
		log.Println(err.Error())
		return nil, err
	}

	c.SetRetryIfFunc(func(req *protocol.Request, resp *protocol.Response, err error) bool {
		return resp.StatusCode() != 200 || err != nil
	})

	//fetch initial ubisoft auth details
	ubi := ubisoft.New(c)

	//start rabbitmq connection as producer
	rq, err := rabbitmq.New()
	if err != nil {
		return nil, err
	}

	//start cron scheduler
	s := gocron.NewScheduler(time.UTC)

	srv := &serverConfig{
		rabbitmq:  rq,
		ubisoft:   ubi,
		scheduler: s,
	}

	errC := make(chan error, 1)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		<-ctx.Done()
		log.Println("::::::::: Shutdown signal received")
		ctxTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		defer func() {
			err := rq.Close()
			if err != nil {
				log.Println("Failed to close RabbitMQ Connection")
			}

			ubi.Close()
			s.Stop()

			stop()
			cancel()
			close(errC)
		}()

		if err := srv.shutdown(ctxTimeout); err != nil {
			errC <- err
		}

		log.Println("::::::::: Shutdown Finished")
	}()

	go func() {
		if err := srv.listenAndServe(); err != nil {
			errC <- err
		}
	}()

	return errC, nil
}

func (s *serverConfig) listenAndServe() error {
	log.Println(":::::: ListenAndServer")
	//TODO initiate cron job every 2hr45min
	//s.scheduler.Every("2h45m").Do()
	job, err := s.scheduler.Every("10m").Do(func(con *rabbitmq.RabbitMQConfig) {
		err := s.ubisoft.Send(context.Background(), con)
		if err != nil {
			log.Println("Job Error", err)
		}

		log.Println("SUCCESS UBI")
	}, s.rabbitmq)

	log.Println(job, err)
	s.scheduler.StartBlocking()

	log.Println("Scheduler Stopped")
	return nil
}

func (s *serverConfig) shutdown(ctx context.Context) error {
	log.Println(":::::: Shutting Down Server")
	for {
		select {
		case <-ctx.Done():
			return errors.New("Context.Done Error")
		case <-s.doneC:
			return nil
		}
	}
}
