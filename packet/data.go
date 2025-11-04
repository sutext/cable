package packet

import (
	"bytes"
	"fmt"

	"sutext.github.io/cable/internal/buffer"
)

type DataPacket struct {
	Channel string
	Catgory uint8
	Payload []byte
}

func NewData(payload []byte) *DataPacket {
	return &DataPacket{
		Payload: payload,
	}
}

func (p *DataPacket) String() string {
	return fmt.Sprintf("DATA(%d bytes)", len(p.Payload))
}
func (p *DataPacket) Type() PacketType {
	return DATA
}
func (p *DataPacket) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if p.Type() != other.Type() {
		return false
	}
	otherData := other.(*DataPacket)
	return p.Channel == otherData.Channel && bytes.Equal(p.Payload, otherData.Payload)
}

func (p *DataPacket) WriteTo(buf *buffer.Buffer) error {
	buf.WriteString(p.Channel)
	buf.WriteUInt8(p.Catgory)
	buf.WriteBytes(p.Payload)
	return nil
}

func (p *DataPacket) ReadFrom(buf *buffer.Buffer) error {
	channel, err := buf.ReadString()
	if err != nil {
		return err
	}
	catgory, err := buf.ReadUInt8()
	if err != nil {
		return err
	}
	payload, err := buf.ReadAll()
	if err != nil {
		return err
	}
	p.Catgory = catgory
	p.Channel = channel
	p.Payload = payload
	return nil
}
