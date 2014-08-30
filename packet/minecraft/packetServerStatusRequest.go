package minecraft

import (
	"io"
	"github.com/LilyPad/GoLilyPad/packet"
)

type PacketServerStatusRequest struct {

}

func NewPacketServerStatusRequest() (this *PacketServerStatusRequest) {
	this = new(PacketServerStatusRequest)
	return
}

func (this *PacketServerStatusRequest) Id() int {
	return PACKET_SERVER_STATUS_REQUEST
}

type packetServerStatusRequestCodec struct {

}

func (this *packetServerStatusRequestCodec) Decode(reader io.Reader, util []byte) (decode packet.Packet, err error) {
	decode = new(PacketServerStatusRequest)
	return
}

func (this *packetServerStatusRequestCodec) Encode(writer io.Writer, util []byte, encode packet.Packet) (err error) {
	return
}
