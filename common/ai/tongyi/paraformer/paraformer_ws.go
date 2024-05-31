package paraformer

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
	httpclient "github.com/yaklang/yaklang/common/ai/tongyi/httpclient"
)

// real-time voice recognition

func ConnRecognitionClient(request *Request, token string) (*httpclient.WsClient, error) {
	// Initialize the client with the necessary parameters.
	header := http.Header{}
	header.Add("Authorization", token)

	client := httpclient.NewWsClient(ParaformerWSURL, header)

	if err := client.ConnClient(request); err != nil {
		return nil, err
	}

	return client, nil
}

func CloseRecognitionClient(cli *httpclient.WsClient) {
	if err := cli.CloseClient(); err != nil {
		log.Printf("close client error: %v", err)
	}
}

func SendRadioData(cli *httpclient.WsClient, bytesData []byte) {
	cli.SendBinaryDates(bytesData)
}

type ResultWriter interface {
	WriteResult(str string) error
}

func HandleRecognitionResult(ctx context.Context, cli *httpclient.WsClient, fn StreamingFunc) {
	outputChan, errChan := cli.ResultChans()

	// TODO: handle errors.
BREAK_FOR:
	for {
		select {
		case output, ok := <-outputChan:
			if !ok {
				log.Println("outputChan is closed")
				break BREAK_FOR
			}

			// streaming callback func
			if err := fn(ctx, output.Data); err != nil {
				log.Println("error: ", err)
				break BREAK_FOR
			}

		case err := <-errChan:
			if err != nil {
				log.Println("error: ", err)
				break BREAK_FOR
			}
		case <-ctx.Done():
			log.Println("Done")
			break BREAK_FOR
		}
	}

	log.Println("get recognition result...over")
}

// task_id length 32.
func GenerateTaskID() string {
	u, err := uuid.NewUUID()
	if err != nil {
		panic(err)
	}
	uuid := strings.ReplaceAll(u.String(), "-", "")

	return uuid
}
