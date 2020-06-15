package tcpip

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/netstack/tcpip/header"
)

func TestKeepAlive(t *testing.T) {
	data, _ := hex.DecodeString("45000028bf2d400040060f91ac1200020ed7b126b9a201bbcd2757cc27e526f6501004d90fbd0000")
	pk, _ := Wrap(data, false)
	opts := pk.TCP.ParsedOptions()
	info, _ := json.Marshal(opts)
	fmt.Printf("%v\n", len(pk.TCP.Options()))
	fmt.Println(pk.TCP.Flags(), header.TCPFlagAck)
	fmt.Println(string(info))
}
