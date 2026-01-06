package broker

import "sutext.github.io/cable/coder"

type optype uint8

// 定义状态机操作类型
const (
	optypeUserOpened optype = iota
	optypeUserClosed
	optypeJoinChannel
	optypeLeaveChannel
)

type opdata interface {
	coder.Codable
	opt() optype
}

type userOpenedOp struct {
	uid      string
	cid      string
	net      string
	brokerID uint64
}

func (op *userOpenedOp) opt() optype {
	return optypeUserOpened
}
func (op *userOpenedOp) WriteTo(e coder.Encoder) error {
	e.WriteString(op.uid)
	e.WriteString(op.cid)
	e.WriteString(op.net)
	e.WriteUInt64(op.brokerID)
	return nil
}
func (op *userOpenedOp) ReadFrom(d coder.Decoder) error {
	var err error
	op.uid, err = d.ReadString()
	if err != nil {
		return err
	}
	op.cid, err = d.ReadString()
	if err != nil {
		return err
	}
	op.net, err = d.ReadString()
	if err != nil {
		return err
	}
	op.brokerID, err = d.ReadUInt64()
	return err
}

type userClosedOp struct {
	uid string
	cid string
}

func (op *userClosedOp) opt() optype {
	return optypeUserClosed
}
func (op *userClosedOp) WriteTo(e coder.Encoder) error {
	e.WriteString(op.uid)
	e.WriteString(op.cid)
	return nil
}
func (op *userClosedOp) ReadFrom(d coder.Decoder) error {
	var err error
	op.uid, err = d.ReadString()
	if err != nil {
		return err
	}
	op.cid, err = d.ReadString()
	return err
}

type joinChannelOp struct {
	uid      string
	channels []string
}

func (op *joinChannelOp) opt() optype {
	return optypeJoinChannel
}
func (op *joinChannelOp) WriteTo(e coder.Encoder) error {
	e.WriteString(op.uid)
	e.WriteStrings(op.channels)
	return nil
}
func (op *joinChannelOp) ReadFrom(d coder.Decoder) error {
	var err error
	op.uid, err = d.ReadString()
	if err != nil {
		return err
	}
	op.channels, err = d.ReadStrings()
	return err
}

type leaveChannelOp struct {
	uid      string
	channels []string
}

func (op *leaveChannelOp) opt() optype {
	return optypeLeaveChannel
}
func (op *leaveChannelOp) WriteTo(e coder.Encoder) error {
	e.WriteString(op.uid)
	e.WriteStrings(op.channels)
	return nil
}
func (op *leaveChannelOp) ReadFrom(d coder.Decoder) error {
	var err error
	op.uid, err = d.ReadString()
	if err != nil {
		return err
	}
	op.channels, err = d.ReadStrings()
	return err
}
