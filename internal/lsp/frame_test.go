package lsp

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestReadMessage_ParsesContentLengthFrame(t *testing.T) {
	raw := "Content-Length: 17\r\n\r\n" + `{"jsonrpc":"2.0"}`
	body, err := ReadMessage(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(body) != `{"jsonrpc":"2.0"}` {
		t.Errorf("body: got %q", body)
	}
}

func TestWriteMessage_EmitsFramedMessage(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteMessage(&buf, []byte(`{"a":1}`)); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	want := "Content-Length: 7\r\n\r\n" + `{"a":1}`
	if buf.String() != want {
		t.Errorf("frame: got %q want %q", buf.String(), want)
	}
}

func TestReadMessage_Roundtrip(t *testing.T) {
	var buf bytes.Buffer
	msg := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if err := WriteMessage(&buf, msg); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	got, err := ReadMessage(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Errorf("roundtrip: got %q want %q", got, msg)
	}
}
