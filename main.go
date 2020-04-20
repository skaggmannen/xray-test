package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime/pprof"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-xray-sdk-go/xray"
	"github.com/aws/aws-xray-sdk-go/xraylog"
)

func main() {
	xray.SetLogger(xraylog.NewDefaultLogger(os.Stderr, xraylog.LogLevelDebug))

	sess := session.Must(session.NewSession())
	xray.AWSSession(sess)

	db := dynamodb.New(sess)

	var handler http.HandlerFunc = func(rsp http.ResponseWriter, req *http.Request) {
		log.Println("Listing tables...")
		output, err := db.ListTablesWithContext(req.Context(), &dynamodb.ListTablesInput{})
		if err != nil {
			panic(err)
		}

		rsp.WriteHeader(http.StatusOK)
		err = json.NewEncoder(rsp).Encode(output.TableNames)
		if err != nil {
			panic(err)
		}

		log.Println("Done!")
	}

	wrapped := xray.Handler(xray.NewFixedSegmentNamer("xray-test-app"), handler)

	for i := 0; i < 100; i++ {
		rsp := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "https://dummy/url", &bytes.Buffer{})
		if err != nil {
			panic(err)
		}
		signal := make(chan struct{})

		go func() {
			wrapped.ServeHTTP(rsp, req)
			if rsp.Code != http.StatusOK {
				panic("expected ok")
			}
			close(signal)
		}()
		select {
		case <-signal:
		case <-time.After(10 * time.Second):
			_ = pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
			panic("timed out!")
		}
	}
}
