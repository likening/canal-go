package client

import (
	"bytes"
	protocol "canal-go/protocol"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"

	// "bytes"
	// "encoding/binary"

	"github.com/golang/protobuf/proto"
)

type SimpleCanalConnector struct {
	Address           string
	Port              int
	UserName          string
	PassWord          string
	SoTime            int32
	IdleTimeOut       int32
	ClientIdentity    protocol.ClientIdentity
	Connected         bool
	Running           bool
	Filter            string
	RollbackOnConnect bool
	LazyParseEntry    bool
}

var (
	sampleConnector SimpleCanalConnector
	conn            net.Conn
)

func NewSimpleCanalConnector(address string, port int, username string, password string, destination string, soTimeOut int32, idleTimeOut int32) *SimpleCanalConnector {
	sampleCanalConnector := &SimpleCanalConnector{}
	sampleCanalConnector.Address = address
	sampleCanalConnector.Port = port
	sampleCanalConnector.UserName = username
	sampleCanalConnector.PassWord = password
	sampleCanalConnector.ClientIdentity = protocol.ClientIdentity{Destination: destination, ClientId: 1001}
	sampleCanalConnector.SoTime = soTimeOut
	sampleCanalConnector.IdleTimeOut = idleTimeOut
	sampleCanalConnector.RollbackOnConnect = true
	sampleConnector = *sampleCanalConnector

	return sampleCanalConnector

}

func (c SimpleCanalConnector) Connect() {
	if c.Connected {
		return
	}

	if c.Running {
		return
	}

	c.doConnect()
	if c.Filter != "" {
		// Subscribe(samplecanal.Filter)
	}

	if c.RollbackOnConnect {

	}

	c.Connected = true

}

func (c SimpleCanalConnector) doConnect() {
	address := c.Address + ":" + fmt.Sprintf("%d", c.Port)
	con, err := net.Dial("tcp", address)
	conn = con
	defer conn.Close()
	checkError(err)

	p := new(protocol.Packet)
	data := readNextPacket()
	err = proto.Unmarshal(data, p)
	checkError(err)
	if p != nil {
		if p.GetVersion() != 1 {
			panic("unsupported version at this client.")
		}

		if p.GetType() != protocol.PacketType_HANDSHAKE {
			panic("expect handshake but found other type.")
		}

		handshake := &protocol.Handshake{}
		err = proto.Unmarshal(p.GetBody(), handshake)
		checkError(err)
		pas := []byte(c.PassWord)
		ca := &protocol.ClientAuth{
			Username:               c.UserName,
			Password:               pas,
			NetReadTimeoutPresent:  &protocol.ClientAuth_NetReadTimeout{NetReadTimeout: c.IdleTimeOut},
			NetWriteTimeoutPresent: &protocol.ClientAuth_NetWriteTimeout{NetWriteTimeout: c.IdleTimeOut},
		}
		caByteArray, _ := proto.Marshal(ca)
		packet := &protocol.Packet{
			Type: protocol.PacketType_CLIENTAUTHENTICATION,
			Body: caByteArray,
		}

		packArray, _ := proto.Marshal(packet)

		WriteWithHeader(packArray)

		pp := readNextPacket()
		pk := &protocol.Packet{}

		err = proto.Unmarshal(pp, pk)
		checkError(err)

		if pk.Type != protocol.PacketType_ACK {
			panic("unexpected packet type when ack is expected")
		}

		ackBody := &protocol.Ack{}
		err = proto.Unmarshal(pk.GetBody(), ackBody)

		if ackBody.GetErrorCode() > 0 {

			panic(errors.New(fmt.Sprintf("something goes wrong when doing authentication:%s", ackBody.GetErrorMessage())))
		}

		c.Connected = true

	}

}

func (c *SimpleCanalConnector) GetWithoutAck(batchSize int32, timeOut *int64, units *int32) *protocol.Message {
	c.waitClientRunning()
	if c.Running {
		return nil
	}
	var size int32

	if batchSize < 0 {
		size = 1000
	} else {
		size = batchSize
	}
	var time *int64
	if timeOut == nil {
		time = timeOut
	} else {
		time = timeOut
	}
	var i int32
	i = -1
	if units == nil {
		units = &i
	}
	get := new(protocol.Get)
	get.AutoAckPresent = &protocol.Get_AutoAck{AutoAck: false}
	get.Destination = c.ClientIdentity.Destination
	get.ClientId = strconv.Itoa(c.ClientIdentity.ClientId)
	get.FetchSize = size
	get.TimeoutPresent = &protocol.Get_Timeout{Timeout: *time}
	get.UnitPresent = &protocol.Get_Unit{Unit: *units}

	getBody, err := proto.Marshal(get)
	checkError(err)
	packet := new(protocol.Packet)
	packet.Type = protocol.PacketType_GET
	packet.Body = getBody
	pa, err := proto.Marshal(packet)
	checkError(err)
	WriteWithHeader(pa)

	return nil
}

func (c *SimpleCanalConnector) receiveMessages() *protocol.Message {
	data := readNextPacket()
	p := new(protocol.Packet)
	err := proto.Unmarshal(data, p)
	checkError(err)
	messages := new(protocol.Messages)
	message := new(protocol.Message)
	entry := &protocol.Entry{}
	length := len(messages.Messages)
	message.Entries = make([]protocol.Entry, length)
	ack := new(protocol.Ack)
	switch p.Type {
	case protocol.PacketType_MESSAGES:
		if !(p.GetCompression() == protocol.Compression_NONE) {
			panic("compression is not supported in this connector")
			err := proto.Unmarshal(p.Body, messages)
			checkError(err)
			if c.LazyParseEntry {
				message.RawEntries = messages.Messages
			} else {

				for index, value := range messages.Messages {
					err := proto.Unmarshal(value, entry)
					checkError(err)
					fmt.Print(message.Entries)
					fmt.Print(index)
					// append(*message.Entries, entry)
				}
			}
		}
		return message

	case protocol.PacketType_ACK:
		err := proto.Unmarshal(p.Body, ack)
		checkError(err)
		panic(errors.New(fmt.Sprintf("something goes wrong with reason:%s", ack.GetErrorMessage())))

	default:
		panic(errors.New(fmt.Sprintf("unexpected packet type:%s", p.Type)))

	}
}

func readHeaderLength() int {
	buf := make([]byte, 4)
	conn.Read(buf)
	bytesBuffer := bytes.NewBuffer(buf)
	var x int32
	binary.Read(bytesBuffer, binary.BigEndian, &x)
	return int(x)
}

func readNextPacket() []byte {
	readLen := readHeaderLength()
	receiveData := make([]byte, readLen)
	conn.Read(receiveData)
	return receiveData
}

func WriteWithHeader(body []byte) {
	lenth := len(body)
	bytes := getWriteHeaderBytes(lenth)
	conn.Write(bytes)
	conn.Write(body)
}

func getWriteHeaderBytes(lenth int) []byte {
	x := int32(lenth)
	bytesBuffer := bytes.NewBuffer([]byte{})
	binary.Write(bytesBuffer, binary.BigEndian, x)
	return bytesBuffer.Bytes()
}

func (c *SimpleCanalConnector) Subscribe(filter string) {
	c.waitClientRunning()
	if !c.Running {
		return
	}
	body, _ := proto.Marshal(&protocol.Sub{Destination: c.ClientIdentity.Destination, ClientId: strconv.Itoa(c.ClientIdentity.ClientId), Filter: c.Filter})
	pack := new(protocol.Packet)
	pack.Type = protocol.PacketType_SUBSCRIPTION
	pack.Body = body

	packet, _ := proto.Marshal(pack)
	WriteWithHeader(packet)

	p := new(protocol.Packet)

	paBytes := readNextPacket()
	err := proto.Unmarshal(paBytes, p)
	checkError(err)
	ack := new(protocol.Ack)
	err = proto.Unmarshal(p.Body, ack)
	checkError(err)

	if ack.GetErrorCode() > 0 {

		panic(errors.New(fmt.Sprintf("failed to subscribe with reason::%s", ack.GetErrorMessage())))
	}

	c.Filter = filter

}

// func (c *SimpleCanalConnector) Subscribe() {
// 	c.Subscribe("")
// }

func Rollback() {

}

func (c *SimpleCanalConnector) waitClientRunning() {
	c.Running = true
}

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}
