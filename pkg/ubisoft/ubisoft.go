package ubisoft

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/eliassebastian/r6index-auth/pkg/rabbitmq"
)

// basic auth header for ubisoft session api rpc
func basicToken() string {
	username := os.Getenv("UBISOFT_USERNAME")
	pw := os.Getenv("UBISOFT_PASS")

	bs := []byte(fmt.Sprintf("%s:%s", username, pw))

	return fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString(bs))
}

type UbisoftRepository struct {
	//http client
	client *client.Client
	ctx    context.Context
	cancel context.CancelFunc
	//Basic Auth Token
	token string
}

// Create new Ubisoft Repository that holds auth details for main api microservice
func New(c *client.Client) *UbisoftRepository {
	return &UbisoftRepository{
		client: c,
		token:  basicToken(),
	}
}

type response struct {
	SessionId  string `json:"sessionId"`
	Ticket     string `json:"ticket"`
	Expiration string `json:"expiration"`
}

// Connect to Ubisoft Auth Servers
func (u *UbisoftRepository) connect(ctx context.Context, new string) (*response, error) {
	var userAgent string

	authScheme := "Basic"
	authValue := u.token
	appId := UBISOFT_APPID

	if new != "" {
		authScheme = "Ubi_v1"
		authValue = "t=" + new
		appId = UBISOFT_NEWAPPID
		userAgent = UBISOFT_USERAGENT
	}

	req := protocol.AcquireRequest()
	res := protocol.AcquireResponse()
	defer protocol.ReleaseResponse(res)

	req.SetMethod(consts.MethodPost)
	req.Header.Set("Connection", "keep-alive")
	req.Header.SetContentTypeBytes([]byte("application/json"))
	req.SetRequestURI(UBISOFT_URL)
	req.SetAuthSchemeToken(authScheme, authValue)
	req.Header.Add("Ubi-AppId", appId)
	if userAgent != "" {
		req.Header.Add("User-Agent", UBISOFT_USERAGENT)
	}

	err := u.client.DoRedirects(ctx, req, res, 1)
	if err != nil {
		return nil, err
	}

	var ubiResponse response
	err = json.NewDecoder(res.BodyStream()).Decode(&ubiResponse)
	if err != nil {
		return nil, err
	}

	return &ubiResponse, nil
}

type packet struct {
	ticket     string
	sessionId  string
	expiration string
	//ubisoft ranked 2.0 header values
	ticketNew     string
	sessionIdNew  string
	expirationNew string
}

func (u *UbisoftRepository) Send(ctx context.Context, rq *rabbitmq.RabbitMQConfig) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	u.ctx = ctx
	u.cancel = cancel

	fr, err := u.connect(ctx, "")
	if err != nil {
		return err
	}

	sr, err := u.connect(ctx, fr.Ticket)
	if err != nil {
		return err
	}

	packet := packet{
		ticket:        fr.Ticket,
		sessionId:     fr.SessionId,
		expiration:    fr.Expiration,
		ticketNew:     sr.Ticket,
		sessionIdNew:  sr.SessionId,
		expirationNew: sr.Expiration,
	}

	var network bytes.Buffer
	e := gob.NewEncoder(&network)
	err = e.Encode(packet)
	if err != nil {
		return err
	}

	b := network.Bytes()
	err = rq.Produce(ctx, &b)
	if err != nil {
		return err
	}

	return nil
}

func (u *UbisoftRepository) Close() {
	u.cancel()
}
