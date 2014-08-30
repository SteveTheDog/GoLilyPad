package minecraft

import (
	"io"
	"github.com/LilyPad/GoLilyPad/packet"
)

type PacketServerLoginStart struct {
	Name string
}

func NewPacketServerLoginStart(name string) (this *PacketServerLoginStart) {
	this = new(PacketServerLoginStart)
	this.Name = name
	return
}

func (this *PacketServerLoginStart) Id() int {
	return PACKET_SERVER_LOGIN_START
}

type packetServerLoginStartCodec struct {

}

func (this *packetServerLoginStartCodec) Decode(reader io.Reader, util []byte) (decode packet.Packet, err error) {
	packetServerLoginStart := new(PacketServerLoginStart)
	packetServerLoginStart.Name, err = packet.ReadString(reader, util)
	if err != nil {
		return
	}
	decode = packetServerLoginStart
	return
}

func (this *packetServerLoginStartCodec) Encode(writer io.Writer, util []byte, encode packet.Packet) (err error) {
	packetServerLoginStart := encode.(*PacketServerLoginStart)
	err = packet.WriteString(writer, util, packetServerLoginStart.Name)
	return
}
