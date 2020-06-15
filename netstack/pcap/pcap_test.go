package pcap

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestPcap(t *testing.T) {
	reader, err := NewReader("../testdata/test_tcp_client_close.pcap")
	if err != nil {
		t.Error(err)
		return
	}
	defer reader.Close()
	writer, err := NewWriter("/tmp/test_out.pcap")
	if err != nil {
		t.Error(err)
		return
	}
	defer writer.Close()
	fileHeadr, _ := json.Marshal(reader.Header)
	fmt.Printf("Header:%x,%v\n", reader.Header.MagicNumber, string(fileHeadr))
	buf := make([]byte, 10240)
	for {
		n, err := reader.Read(buf)
		if err != nil {
			return
		}
		_, err = writer.Write(buf[0:n])
		if err != nil {
			return
		}
	}
}
