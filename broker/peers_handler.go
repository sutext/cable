package broker

import (
	"encoding/json"

	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xerr"
)

func (b *broker) handleSendMessage(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
	dec := coder.NewDecoder(p.Content)
	flag, err := dec.ReadUInt8()
	if err != nil {
		return nil, err
	}
	target, err := dec.ReadString()
	if err != nil {
		return nil, err
	}
	msg := &packet.Message{}
	msg.ReadFrom(dec)
	var total, success uint64
	switch flag {
	case 0:
		total, success = b.sendToAll(msg)
	case 1:
		total, success = b.sendToUser(target, msg)
	case 2:
		total, success = b.sendToChannel(target, msg)
	default:
		return nil, xerr.InvalidPeerMessageFlag
	}
	enc := coder.NewEncoder()
	enc.WriteVarint(total)
	enc.WriteVarint(success)
	return packet.NewResponse(p.ID, enc.Bytes()), nil
}
func (b *broker) handleIsOnline(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
	online := b.isOnline(string(p.Content))
	var r byte
	if online {
		r = 1
	}
	return packet.NewResponse(p.ID, []byte{r}), nil
}
func (b *broker) handleKickConn(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
	b.kickConn(string(p.Content))
	return packet.NewResponse(p.ID), nil
}
func (b *broker) handleKickUser(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
	b.kickUser(string(p.Content))
	return packet.NewResponse(p.ID), nil
}
func (b *broker) handlePeerInspect(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
	isp := b.inspect()
	data, err := json.Marshal(isp)
	if err != nil {
		return nil, err
	}
	return packet.NewResponse(p.ID, data), nil
}
func (b *broker) handleJoinChannel(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
	decoder := coder.NewDecoder(p.Content)
	uid, err := decoder.ReadString()
	if err != nil {
		return nil, err
	}
	chs, err := decoder.ReadStrings()
	if err != nil {
		return nil, err
	}
	count := b.joinChannel(uid, chs)
	encoder := coder.NewEncoder()
	encoder.WriteVarint(count)
	return packet.NewResponse(p.ID, encoder.Bytes()), nil
}
func (b *broker) handleLeaveChannel(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
	decoder := coder.NewDecoder(p.Content)
	uid, err := decoder.ReadString()
	if err != nil {
		return nil, err
	}
	chs, err := decoder.ReadStrings()
	if err != nil {
		return nil, err
	}
	count := b.leaveChannel(uid, chs)
	encoder := coder.NewEncoder()
	encoder.WriteVarint(count)
	return packet.NewResponse(p.ID, encoder.Bytes()), nil
}
