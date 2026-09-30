// Command lambda is a runnable demo of the Go Presto Pay SDK's webhook
// verifier deployed as an AWS Lambda function behind API Gateway. A
// verifier needs no private key, so this function holds no signing
// material — only Presto's public key and the accepted merchant IDs.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http/httptest"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/prestoconnect/presto-pay-sdk-go/prestopay"
)

var verifier *prestopay.WebhookVerifier

func init() {
	mid := os.Getenv("PRESTOPAY_MID")
	if mid == "" {
		log.Fatal("PRESTOPAY_MID environment variable is required")
	}
	publicKey, err := readKey("PRESTOPAY_PUBLIC_KEY", "PRESTOPAY_PUBLIC_KEY_FILE")
	if err != nil {
		log.Fatalf("reading Presto public key: %v", err)
	}

	verifier, err = prestopay.NewWebhookVerifier(prestopay.WebhookConfig{
		MerchantIDs:      []string{mid},
		PrestoPublicKeys: [][]byte{publicKey},
	})
	if err != nil {
		log.Fatalf("NewWebhookVerifier: %v", err)
	}
}

func handleRequest(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	event, err := verifier.Verify([]byte(req.Body))
	if err != nil {
		log.Printf("webhook verification failed: %v", err)
		return ackResponse(prestopay.AckForError(err)), nil
	}

	// A real function would look up event.EventRefNum in a table with a
	// uniqueness constraint here, and only fulfil on the first sighting —
	// Presto redelivers an unacknowledged webhook up to five times.
	// A webhook says what happened, not the payment's resulting status: a
	// function that needs the status calls Payments.Query here, and answers
	// AckForError(err) if that fails so Presto delivers the event again.
	log.Printf("webhook: prestoMrn=%s paymentRefNum=%s eventCode=%s success=%t",
		event.PrestoMRN, event.PaymentRefNum, event.EventCode, event.Success)

	return ackResponse(prestopay.AckOK), nil
}

// ackResponse reuses WriteAck, which writes to an http.ResponseWriter, by
// recording it into an httptest.ResponseRecorder, so the notify-ack body
// and status stay defined in one place regardless of which SDK example
// calls it.
func ackResponse(ack prestopay.Ack) events.APIGatewayProxyResponse {
	rec := httptest.NewRecorder()
	prestopay.WriteAck(rec, ack)
	return events.APIGatewayProxyResponse{
		StatusCode: rec.Code,
		Headers:    map[string]string{"Content-Type": rec.Header().Get("Content-Type")},
		Body:       rec.Body.String(),
	}
}

func readKey(envVar, fileVar string) ([]byte, error) {
	if v := os.Getenv(envVar); v != "" {
		return []byte(v), nil
	}
	path := os.Getenv(fileVar)
	if path == "" {
		return nil, fmt.Errorf("%s or %s is required", envVar, fileVar)
	}
	return os.ReadFile(path)
}

func main() {
	lambda.Start(handleRequest)
}
