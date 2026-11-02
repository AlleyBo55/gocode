package mcpclient

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"testing"
)

// pipeConn builds a ServerConn whose stdin/stdout are the two ends of an
// in-memory pipe pair, and returns the "server side" of that pipe so a test
// can script the server's replies without spawning a real subprocess.
type fakePipes struct {
	clientStdinW *io.PipeWriter // client writes requests here
	serverRead   *bufio.Reader  // server reads them
	serverWriteW *io.PipeWriter // server writes responses here
	conn         *ServerConn
}

func newPipeConn(t *testing.T, framing string) *fakePipes {
	t.Helper()
	reqR, reqW := io.Pipe()   // client -> server
	respR, respW := io.Pipe() // server -> client

	sc := &ServerConn{
		def:    ServerDef{Name: "fake", Framing: framing},
		stdin:  reqW,
		stdout: bufio.NewReader(respR),
		alive:  true,
	}
	// Close the request writer when the test ends so any server goroutine
	// blocked in ReadString('\n') gets EOF instead of hanging the suite.
	t.Cleanup(func() { reqW.Close() })
	return &fakePipes{
		clientStdinW: reqW,
		serverRead:   bufio.NewReader(reqR),
		serverWriteW: respW,
		conn:         sc,
	}
}

// serveOnce reads one client frame and writes back the given response body,
// using the same framing the client used. Run in a goroutine.
func (f *fakePipes) serveOnce(t *testing.T, framing, responseBody string) {
	t.Helper()
	if _, err := readServerFrame(f.serverRead, framing); err != nil {
		t.Errorf("server read request: %v", err)
		return
	}
	if err := writeServerFrame(f.serverWriteW, framing, responseBody); err != nil {
		t.Errorf("server write response: %v", err)
	}
}

func readServerFrame(in *bufio.Reader, framing string) (string, error) {
	if framing == "ndjson" {
		line, err := in.ReadString('\n')
		return strings.TrimSpace(line), err
	}
	contentLen := 0
	for {
		line, err := in.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			if _, err := fmt.Sscanf(strings.TrimPrefix(line, "Content-Length:"), "%d", &contentLen); err != nil {
				return "", err
			}
		}
	}
	buf := make([]byte, contentLen)
	_, err := io.ReadFull(in, buf)
	return string(buf), err
}

func writeServerFrame(out *io.PipeWriter, framing, body string) error {
	if framing == "ndjson" {
		_, err := out.Write([]byte(body + "\n"))
		return err
	}
	if _, err := fmt.Fprintf(out, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err := out.Write([]byte(body))
	return err
}

// roundTrip drives one send() against the fake server and returns the raw result.
func roundTrip(t *testing.T, framing string) string {
	t.Helper()
	f := newPipeConn(t, framing)
	resp := `{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`
	go f.serveOnce(t, framing, resp)

	r, err := f.conn.send("initialize", nil)
	if err != nil {
		t.Fatalf("send(%s): %v", framing, err)
	}
	return string(r.Result)
}

// TestWriteReadContentLength proves the opt-in LSP Content-Length path still
// round-trips (regression guard for servers configured "content-length").
func TestWriteReadContentLength(t *testing.T) {
	got := roundTrip(t, "content-length")
	if !strings.Contains(got, `"ok":true`) {
		t.Fatalf("result = %s", got)
	}
}

// TestWriteReadNdjson proves the new newline-delimited path (used by REA)
// round-trips through the same send() code.
func TestWriteReadNdjson(t *testing.T) {
	got := roundTrip(t, "ndjson")
	if !strings.Contains(got, `"ok":true`) {
		t.Fatalf("result = %s", got)
	}
}

// TestNdjsonWriteTerminatesWithNewline asserts the ndjson writer actually ends
// the frame with '\n' — the exact bug this feature fixes (gocode previously
// wrote only Content-Length frames, which an ndjson server never answers).
func TestNdjsonWriteTerminatesWithNewline(t *testing.T) {
	f := newPipeConn(t, "ndjson")
	done := make(chan string, 1)
	go func() {
		line, _ := f.serverRead.ReadString('\n')
		done <- line
	}()
	if err := f.conn.writeMessage([]byte(`{"jsonrpc":"2.0","id":1}`)); err != nil {
		t.Fatalf("writeMessage: %v", err)
	}
	got := <-done
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("ndjson frame not newline-terminated: %q", got)
	}
	if !strings.Contains(got, `"id":1`) {
		t.Fatalf("ndjson frame missing body: %q", got)
	}
}

// TestContentLengthWriteHasHeader asserts the Content-Length writer emits a
// proper header before the body (regression guard for the opt-in path). It
// reads the header line AND drains the body so the client's body write does
// not block on a full pipe.
func TestContentLengthWriteHasHeader(t *testing.T) {
	f := newPipeConn(t, "content-length")
	done := make(chan string, 2)
	go func() {
		header, _ := f.serverRead.ReadString('\n') // "Content-Length: 8\r\n"
		blank, _ := f.serverRead.ReadString('\n')  // "\r\n"
		done <- header
		done <- blank
		body := make([]byte, 8)
		io.ReadFull(f.serverRead, body) // drain the body so writeMessage returns
	}()
	if err := f.conn.writeMessage([]byte(`{"id":1}`)); err != nil {
		t.Fatalf("writeMessage: %v", err)
	}
	header := <-done
	blank := <-done
	if !strings.HasPrefix(header, "Content-Length: 8") {
		t.Fatalf("Content-Length header wrong: %q", header)
	}
	if strings.TrimSpace(blank) != "" {
		t.Fatalf("expected blank separator line, got %q", blank)
	}
}
