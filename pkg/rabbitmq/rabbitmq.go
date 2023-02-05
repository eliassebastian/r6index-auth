package rabbitmq

import (
	"context"
	"errors"
	"log"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitMQConfig struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	queue      *amqp.Queue
}

func New() (*RabbitMQConfig, error) {
	url := os.Getenv("RABBITMQ_URL")

	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, errors.New("error starting new rabbitmq connection")
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}

	if err := ch.ExchangeDeclare(
		"r6index", // name
		"fanout",  // type
		true,      // durable
		false,     // auto-deleted
		false,     // internal
		false,     // noWait
		nil,       // arguments
	); err != nil {
		return nil, err
	}

	q, err := ch.QueueDeclare(
		"auth",                        // name
		true,                          // durable
		false,                         // delete when unused
		false,                         // exclusive
		false,                         // no-wait
		amqp.Table{"x-max-length": 1}, // arguments
	)

	if err != nil {
		return nil, err
	}

	err = ch.QueueBind(
		q.Name,    // queue name
		"",        // routing key
		"r6index", // exchange
		false,
		nil,
	)

	if err != nil {
		return nil, err
	}

	return &RabbitMQConfig{
		connection: conn,
		channel:    ch,
		queue:      &q,
	}, nil
}

func (p *RabbitMQConfig) Close() error {
	err := p.channel.Close()
	if err != nil {
		log.Println("error trying to close rabbit channel")
	}

	err = p.connection.Close()
	if err != nil {
		log.Println("error trying to close rabbit connection")
	}

	return nil
}

func (p *RabbitMQConfig) Produce(ctx context.Context, b *[]byte) error {
	err := p.channel.PublishWithContext(
		ctx,
		"r6index", // exchange
		"",        // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        *b,
			//DeliveryMode: 2,
		})

	return err
}
