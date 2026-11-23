package httpx

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSSEFrameSurvivesOrdinaryWriteTimeout(t *testing.T) {
	resume := make(chan struct{})
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if err := WriteSSEFrame(w, "data: first\n\n"); err != nil {
			return
		}
		<-resume
		WriteSSEFrame(w, "data: second\n\n")
	}))
	server.Config.WriteTimeout = 30 * time.Millisecond
	server.Start()
	defer server.Close()
	response, err := server.Client().Get(server.URL)
	if err != nil {
		close(resume)
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	first, err := reader.ReadString('\n')
	time.Sleep(2 * server.Config.WriteTimeout)
	close(resume)
	if err != nil || first != "data: first\n" {
		t.Fatalf("首帧内容无效 %q %v", first, err)
	}
	tail, err := io.ReadAll(reader)
	if err != nil || string(tail) != "\ndata: second\n\n" {
		t.Fatalf("续帧内容无效 %q %v", tail, err)
	}
}
